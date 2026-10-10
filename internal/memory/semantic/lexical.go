package semantic

import (
	"context"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/butler/butler/internal/memory/sanitize"
	"github.com/butler/butler/internal/security/content"
)

const maxCandidates = 16

type LexicalHits struct {
	Exact, FullText []Hit
}

// technicalTerms preserves spelling of bounded identifiers, paths and versions.
// Natural words do not become exact-match authority or relevance thresholds.
func technicalTerms(query string) []string {
	parts := strings.FieldsFunc(query, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune("_./:#@-", r)
	})
	var output []string
	seen := map[string]bool{}
	for _, term := range parts {
		term = strings.Trim(term, ".:#@-")
		if len(term) < 2 || len(term) > 256 || seen[term] {
			continue
		}
		technical, lower, upper, nonASCII := strings.ContainsAny(term, "_./:#@-"), false, false, false
		for _, r := range term {
			if r > unicode.MaxASCII {
				nonASCII = true
			}
			technical = technical || unicode.IsDigit(r)
			lower = lower || unicode.IsLower(r)
			upper = upper || unicode.IsUpper(r)
		}
		// A generic acronym in a sentence (for example POST/TLS) is not a
		// specific function/error identity. A bare acronym is still exact lookup.
		bareAcronym := !nonASCII && upper && !lower && strings.Trim(strings.TrimSpace(query), "`\"'()?!") == term
		if technical || !nonASCII && lower && upper || bareAcronym {
			output = append(output, term)
			seen[term] = true
			if len(output) == 16 {
				break
			}
		}
	}
	return output
}

func exactOnlyQuery(query string, terms []string) bool {
	return len(terms) == 1 && strings.Trim(strings.TrimSpace(query), "`\"'()?!") == terms[0]
}

const lexicalDocument = `coalesce(title,'')||' '||coalesce(content,'')||' '||coalesce(source_id,'')||' '||coalesce(provenance->'locator'->>'path','')||' '||coalesce(provenance->'locator'->>'repository_revision','')`
const lexicalBounds = `octet_length(title)<=1024 AND octet_length(content)<=32768 AND octet_length(summary)<=32768 AND octet_length(source_id)<=2048 AND octet_length(provenance::text)<=16384`

// Lexical uses current chunks rather than stale index text. The guard validates
// every candidate before any model can receive it; nil authority always denies.
func (s *Store) Lexical(ctx context.Context, scope Scope, query string) (LexicalHits, error) {
	var output LexicalHits
	if s == nil || s.pool == nil {
		return output, ErrUnavailable
	}
	if s.Guard == nil {
		return output, ErrAccess
	}
	if !scope.valid() || strings.TrimSpace(query) == "" || len(query) > 2048 {
		return output, ErrInvalid
	}
	if sanitize.Text(query) != query || operationOrigin(ctx).Sensitivity == content.Secret {
		return output, content.ErrSecret
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	terms := technicalTerms(query)
	if len(terms) > 0 {
		// Literal-quoted, bounded regex with identifier boundaries: no query SQL or
		// regex syntax is executable. A shorter function name can match its method.
		patterns := make([]string, len(terms))
		for i, term := range terms {
			patterns[i] = `(^|[^[:alnum:]_])` + regexp.QuoteMeta(term) + `($|[^[:alnum:]_])`
		}
		sql := `SELECT ` + sourceColumns + ` FROM memory_chunks WHERE scope_type=$1 AND scope_id=$2 AND ` + liveSource + ` AND ` + lexicalBounds + ` AND (` + lexicalDocument + `) ~ ANY($3::text[]) ORDER BY updated_at DESC,id LIMIT $4`
		hits, err := s.lexicalRows(ctx, sql, scope.Type, scope.ID, patterns, maxCandidates)
		if err != nil {
			return output, err
		}
		output.Exact = hits
	}
	vector := `(to_tsvector('simple'::regconfig,` + lexicalDocument + `)||to_tsvector('russian'::regconfig,` + lexicalDocument + `))`
	search := `(plainto_tsquery('simple'::regconfig,$3)||plainto_tsquery('russian'::regconfig,$3))`
	sql := `SELECT ` + sourceColumns + ` FROM memory_chunks WHERE scope_type=$1 AND scope_id=$2 AND ` + liveSource + ` AND ` + lexicalBounds + ` AND ` + vector + ` @@ ` + search + ` ORDER BY ts_rank_cd(` + vector + `,` + search + `) DESC,updated_at DESC,id LIMIT $4`
	hits, err := s.lexicalRows(ctx, sql, scope.Type, scope.ID, query, maxCandidates)
	if err != nil {
		return output, err
	}
	output.FullText = hits
	return output, nil
}

func (s *Store) lexicalRows(ctx context.Context, sql string, args ...any) ([]Hit, error) {
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hits []Hit
	var sources []Source
	for rows.Next() {
		src, err := scanSource(rows)
		if err != nil {
			return nil, err
		}
		if src.MemoryOrigin().Sensitivity == content.Secret || sanitize.Text(src.Title) != src.Title || sanitize.Text(src.Content) != src.Content || sanitize.Text(src.Summary) != src.Summary {
			continue
		}
		hits = append(hits, Hit{Source: src})
		sources = append(sources, src)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if err := s.check(ctx, sources); err != nil {
		return nil, err
	}
	return hits, nil
}
