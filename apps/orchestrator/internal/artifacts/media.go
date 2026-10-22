package artifacts

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
)

const MaxMediaBytes = 10 * 1024 * 1024

var ErrMediaDenied = errors.New("retained media unavailable or outside policy")

// Media contains retained bytes only in the private channel layer. Model tools
// receive an artifact reference, never a path, credential or public download URL.
type Media struct {
	Ref    string `json:"-"`
	Name   string `json:"name"`
	MIME   string `json:"mime"`
	SHA256 string `json:"sha256"`
	Data   []byte `json:"data"`
}

func (m Media) Validate() error {
	if len(m.Data) == 0 || len(m.Data) > MaxMediaBytes || len(m.Name) == 0 || len(m.Name) > 120 || !utf8.ValidString(m.Name) || strings.ContainsAny(m.Name, "/\\\r\n\x00\"") {
		return ErrMediaDenied
	}
	for _, r := range m.Name {
		if r < 32 || r == 127 {
			return ErrMediaDenied
		}
	}
	h := sha256.Sum256(m.Data)
	if m.SHA256 != hex.EncodeToString(h[:]) {
		return ErrMediaDenied
	}
	switch m.MIME {
	case "image/png", "image/jpeg":
		cfg, _, err := image.DecodeConfig(bytes.NewReader(m.Data))
		if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width+cfg.Height > 10000 || cfg.Width > 20*cfg.Height || cfg.Height > 20*cfg.Width || http.DetectContentType(m.Data) != m.MIME {
			return ErrMediaDenied
		}
	case "text/plain", "application/pdf", "application/octet-stream":
	default:
		return ErrMediaDenied
	}
	return nil
}

// SaveMedia uses the existing artifact repository and its run ownership guard.
// Each save has a fresh immutable identity; no model-selected storage path exists.
func (s *Service) SaveMedia(ctx context.Context, runID, sessionKey, name, mime string, data []byte) (string, error) {
	token, ok := ownership.TokenFromContext(ctx)
	if !ok || token.RunID != runID || token.SessionKey != sessionKey {
		return "", ErrMediaDenied
	}
	h := sha256.Sum256(data)
	m := Media{Name: name, MIME: mime, SHA256: hex.EncodeToString(h[:]), Data: data}
	if m.Validate() != nil {
		return "", ErrMediaDenied
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return "", ErrMediaDenied
	}
	var identity [16]byte
	if _, err := rand.Read(identity[:]); err != nil {
		return "", ErrMediaDenied
	}
	id := "artifact-media-" + hex.EncodeToString(identity[:])
	_, err = s.repo.CreateArtifact(ctx, CreateParams{ArtifactID: id, RunID: runID, SessionKey: sessionKey, ArtifactType: "communication_media", Title: name, Summary: "Retained channel attachment", ContentJSON: string(raw), ContentFormat: "private_media_v1", SourceType: "run", SourceRef: runID, CreatedAt: time.Now().UTC()})
	if err != nil {
		return "", ErrMediaDenied
	}
	return "artifact:" + id, nil
}

// ResolveMediaTx must be composed with current Catalog/export authorization.
// Owner equality alone is insufficient. The row lock pins retained bytes through
// publication planning; send resolves and revalidates the digest again.
func ResolveMediaTx(ctx context.Context, tx pgx.Tx, runID, ref string, authorize func(context.Context, pgx.Tx, string, string) error) (Media, error) {
	if tx == nil || authorize == nil || !strings.HasPrefix(ref, "artifact:") || len(ref) > 512 {
		return Media{}, ErrMediaDenied
	}
	if authorize(ctx, tx, runID, ref) != nil {
		return Media{}, ErrMediaDenied
	}
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT a.content_json FROM artifacts a JOIN sessions sa ON sa.session_key=a.session_key JOIN runs r ON r.run_id=$2 JOIN sessions sr ON sr.session_key=r.session_key WHERE a.artifact_id=$1 AND a.artifact_type='communication_media' AND a.content_format='private_media_v1' AND sa.user_id=sr.user_id AND NOT COALESCE((to_jsonb(a)->>'privacy_redacted')::boolean,FALSE) FOR SHARE OF a,sa,sr`, strings.TrimPrefix(ref, "artifact:"), runID).Scan(&raw)
	if err != nil || len(raw) > base64.StdEncoding.EncodedLen(MaxMediaBytes)+1024 {
		return Media{}, ErrMediaDenied
	}
	var m Media
	if json.Unmarshal(raw, &m) != nil || m.Validate() != nil {
		return Media{}, ErrMediaDenied
	}
	if authorize(ctx, tx, runID, ref) != nil {
		return Media{}, ErrMediaDenied
	}
	m.Ref = ref
	return m, nil
}
