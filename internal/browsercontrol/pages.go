package browsercontrol

import (
	"context"
	"github.com/butler/butler/internal/browserpage"
	"math"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"
)

const MaxLivePages = 4
const MaxLifetimePages = 32

// PageContext is trusted Broker metadata, never model-supplied authority.
// Page APIs below only maintain executor observations: they grant no tool,
// credential, network, resource lease, approval or effect-reconciliation right.
// Action Service and the final executor still have to verify current authority.
type PageContext struct {
	SessionID, RunID, OwnerID, TaskID string
	Epoch                             int64
}

// PageRef must match the exact page and observation, not a title or active tab.
type PageRef = browserpage.Ref

type PageState struct {
	PageRef
	OpenerID string `json:"opener_page_id,omitempty"`
	URL      string `json:"observed_url"`
	Epoch    int64  `json:"observed_epoch"`
	State    string `json:"state"`
}

func pageID(id string) bool { return validID(id) && id == strings.ToLower(id) }
func validPageRef(ref PageRef) bool {
	return pageID(ref.ID) && ref.Version >= 0 && ref.Version < math.MaxInt64 &&
		(ref.Version == 0 && ref.DocumentID == "" || ref.Version > 0 && pageID(ref.DocumentID))
}

// pageURL strips callback query/fragment BEFORE any PostgreSQL write. The
// executor must already mask secrets in path segments. This is not an egress
// check or a grant to release the returned URL to a model.
func pageURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 8192 || u.Opaque != "" || u.User != nil || u.Hostname() == "" || u.Scheme != "https" && u.Scheme != "http" {
		return "", ErrDenied
	}
	u.RawQuery, u.Fragment, u.RawFragment, u.ForceQuery = "", "", "", false
	clean := u.String()
	if len(clean) > 2048 || strings.ContainsAny(clean, "\r\n\t") {
		return "", ErrDenied
	}
	return clean, nil
}

func (s *Store) pageTx(ctx context.Context, p PageContext) (pgx.Tx, error) {
	if s == nil || s.Pool == nil || !pageID(p.SessionID) || p.RunID == "" || p.OwnerID == "" || p.Epoch < 0 {
		return nil, ErrDenied
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, ErrDenied
	}
	var found bool
	err = tx.QueryRow(ctx, `SELECT TRUE FROM server_browser_sessions
 WHERE browser_session_id=$1 AND run_id=$2 AND owner_id=$3 AND task_id=$4
 AND epoch=$5 AND mode='agent' AND expires_at>clock_timestamp() FOR UPDATE`,
		p.SessionID, p.RunID, p.OwnerID, p.TaskID, p.Epoch).Scan(&found)
	if err != nil || !found {
		_ = tx.Rollback(ctx)
		return nil, ErrDenied
	}
	return tx, nil
}

func commitPageTx(ctx context.Context, tx pgx.Tx, p PageContext) error {
	var live bool
	if tx.QueryRow(ctx, `SELECT expires_at>clock_timestamp() FROM server_browser_sessions WHERE browser_session_id=$1`, p.SessionID).Scan(&live) != nil || !live || tx.Commit(ctx) != nil {
		return ErrDenied
	}
	return nil
}

func readPage(ctx context.Context, tx pgx.Tx, session string, ref PageRef) (PageState, error) {
	var state PageState
	err := tx.QueryRow(ctx, `SELECT page_id,document_id,observation_version,COALESCE(opener_page_id,''),observed_url,observed_epoch,state
 FROM server_browser_pages WHERE browser_session_id=$1 AND page_id=$2 AND document_id=$3 AND observation_version=$4`,
		session, ref.ID, ref.DocumentID, ref.Version).Scan(&state.ID, &state.DocumentID, &state.Version, &state.OpenerID, &state.URL, &state.Epoch, &state.State)
	if err != nil || state.State != "pending" && state.State != "ready" {
		return PageState{}, ErrDenied
	}
	return state, nil
}

// RegisterPage records one observed pending page. The first page has no opener;
// subsequent popup records require an exact, fresh opener. Network admission
// must happen independently BEFORE its first request; registration is no grant.
func (s *Store) RegisterPage(ctx context.Context, p PageContext, id string, opener *PageRef) (PageRef, error) {
	if !pageID(id) || opener != nil && !validPageRef(*opener) {
		return PageRef{}, ErrDenied
	}
	tx, err := s.pageTx(ctx, p)
	if err != nil {
		return PageRef{}, ErrDenied
	}
	defer tx.Rollback(ctx)
	var live, total int
	if tx.QueryRow(ctx, `SELECT count(*) FILTER(WHERE state IN ('pending','ready')),count(*) FROM server_browser_pages WHERE browser_session_id=$1`, p.SessionID).Scan(&live, &total) != nil || live >= MaxLivePages || total >= MaxLifetimePages {
		return PageRef{}, ErrDenied
	}
	var parent any
	if opener == nil {
		if total != 0 {
			return PageRef{}, ErrDenied
		}
	} else {
		state, e := readPage(ctx, tx, p.SessionID, *opener)
		if e != nil || state.State != "ready" || state.Epoch != p.Epoch {
			return PageRef{}, ErrDenied
		}
		parent = opener.ID
	}
	if _, err = tx.Exec(ctx, `INSERT INTO server_browser_pages(browser_session_id,page_id,opener_page_id) VALUES($1,$2,$3)`, p.SessionID, id, parent); err != nil || commitPageTx(ctx, tx, p) != nil {
		return PageRef{}, ErrDenied
	}
	return PageRef{ID: id}, nil
}

// ObservePage records a fresh executor observation using compare-and-swap.
// Even same-document reads advance the version, invalidating old preparations.
// A new control epoch requires a new observation for EACH page, not a context
// active-page fallback. No legacy fresh_needed flag is cleared here.
func (s *Store) ObservePage(ctx context.Context, p PageContext, expected PageRef, documentID, observedURL string) (PageState, error) {
	cleanURL, err := pageURL(observedURL)
	if err != nil || !validPageRef(expected) || !pageID(documentID) {
		return PageState{}, ErrDenied
	}
	tx, err := s.pageTx(ctx, p)
	if err != nil {
		return PageState{}, ErrDenied
	}
	defer tx.Rollback(ctx)
	state, err := readPage(ctx, tx, p.SessionID, expected)
	if err != nil {
		return PageState{}, ErrDenied
	}
	state.DocumentID, state.Version, state.URL, state.Epoch, state.State = documentID, state.Version+1, cleanURL, p.Epoch, "ready"
	_, err = tx.Exec(ctx, `UPDATE server_browser_pages SET document_id=$3,observation_version=$4,observed_url=$5,observed_epoch=$6,state='ready',updated_at=clock_timestamp()
 WHERE browser_session_id=$1 AND page_id=$2`, p.SessionID, state.ID, state.DocumentID, state.Version, state.URL, state.Epoch)
	if err != nil || commitPageTx(ctx, tx, p) != nil {
		return PageState{}, ErrDenied
	}
	return state, nil
}

// ResolvePage verifies exact metadata under the shared context fence. It must
// never be interpreted as dispatch/credential authorization by itself.
func (s *Store) ResolvePage(ctx context.Context, p PageContext, expected PageRef) (PageState, error) {
	if !validPageRef(expected) {
		return PageState{}, ErrDenied
	}
	tx, err := s.pageTx(ctx, p)
	if err != nil {
		return PageState{}, ErrDenied
	}
	defer tx.Rollback(ctx)
	state, err := readPage(ctx, tx, p.SessionID, expected)
	if err != nil || state.State != "ready" || state.Epoch != p.Epoch || commitPageTx(ctx, tx, p) != nil {
		return PageState{}, ErrDenied
	}
	return state, nil
}

// RetirePage records executor-confirmed closure/loss, not a command to close a
// page and not proof of absence of a previous external effect. Terminal IDs are
// retained and cannot be re-registered to evade fences or the lifetime limit.
func (s *Store) RetirePage(ctx context.Context, p PageContext, expected PageRef, terminal string) error {
	if !validPageRef(expected) || terminal != "closed" && terminal != "lost" {
		return ErrDenied
	}
	tx, err := s.pageTx(ctx, p)
	if err != nil {
		return ErrDenied
	}
	defer tx.Rollback(ctx)
	if _, err = readPage(ctx, tx, p.SessionID, expected); err != nil {
		return ErrDenied
	}
	_, err = tx.Exec(ctx, `UPDATE server_browser_pages SET state=$3,observation_version=observation_version+1,updated_at=clock_timestamp() WHERE browser_session_id=$1 AND page_id=$2`, p.SessionID, expected.ID, terminal)
	if err != nil || commitPageTx(ctx, tx, p) != nil {
		return ErrDenied
	}
	return nil
}
