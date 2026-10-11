package resourceexec

import (
	"context"
	"encoding/json"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/resources"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SingleTabSession is private trusted metadata returned by the owning module
// after locking its current approved selection in the caller transaction.
// Neither an opaque ID nor a raw Chrome tab number grants rights.
type SingleTabSession struct {
	ID              string    `json:"id"`
	BrowserInstance string    `json:"browser_instance"`
	TabRef          string    `json:"tab_ref"`
	Origin          string    `json:"origin"`
	ExpiresAt       time.Time `json:"expires_at"`
}
type SingleTabGuard func(context.Context, pgx.Tx, resources.Binding, string) (SingleTabSession, error)

// SingleTabContractRevision requires the observation-aware fixed contract.
// Pinning the legacy hash does not activate its implicit-session semantics.
const SingleTabContractRevision = "guarded-1"

func NewSingleTab(pool *pgxpool.Pool, r *resources.Store, b *budget.Controller, source SourceGuard, guard SingleTabGuard, p Profile) (*Store, error) {
	if pool == nil || r == nil || b == nil || source == nil || guard == nil || p.ID == "" || len(p.ID) > 128 || p.RuntimeTarget == "" || len(p.ContractHashes) == 0 {
		return nil, ErrDenied
	}
	hashes := map[string]string{}
	for name, h := range p.ContractHashes {
		if !singleTabTool(name) || len(h) != 64 || !isHex(h) {
			return nil, ErrDenied
		}
		hashes[name] = h
	}
	p.ContractHashes = hashes
	return &Store{pool: pool, resources: r, budgets: b, sourceGuard: source, profile: p, singleTab: guard}, nil
}
func singleTabTool(tool string) bool {
	return tool == "single_tab.status" || tool == "single_tab.click"
}

type singleTabSelection struct {
	Session     SingleTabSession `json:"session"`
	Observation string           `json:"observation"`
}

func (s *Store) singleSelection(call *toolbrokerv1.ToolCall, c *toolbrokerv1.ToolContract, g actionauth.Grant) (selection, error) {
	if s.singleTab == nil || c.GetContractRevision() != SingleTabContractRevision || !singleTabTool(call.GetToolName()) || call.GetToolName() != c.GetToolName() || call.GetRunId() != g.Binding.RunID || call.GetToolCallId() != g.Binding.ToolCallID || call.GetToolName() != g.Binding.ToolName || c.GetRuntimeTarget() != s.profile.RuntimeTarget {
		return selection{}, ErrDenied
	}
	ch, e := actionauth.ContractHash(c)
	if e != nil || ch != g.Binding.ContractHash || s.profile.ContractHashes[call.GetToolName()] != ch {
		return selection{}, ErrDenied
	}
	ah, e := actionauth.ArgumentsHash(call.GetArgsJson())
	if e != nil || ah != g.Binding.ArgsHash {
		return selection{}, ErrDenied
	}
	var args struct {
		SessionID   string `json:"session_id"`
		Observation string `json:"observation_id"`
	}
	if json.Unmarshal([]byte(call.GetArgsJson()), &args) != nil || args.SessionID == "" || len(args.SessionID) > 128 ||
		(call.GetToolName() == "single_tab.click" && !validObservation(args.Observation)) || (call.GetToolName() == "single_tab.status" && args.Observation != "") {
		return selection{}, ErrDenied
	}
	return selection{profile: s.profile.ID, target: s.profile.RuntimeTarget, tool: call.GetToolName(), contract: ch, args: ah, authorization: g.ID, single: &singleTabSelection{Session: SingleTabSession{ID: args.SessionID}, Observation: args.Observation}}, nil
}
func validObservation(v string) bool {
	return len(v) == 36 && v[8] == '-' && v[13] == '-' && v[18] == '-' && v[23] == '-' && isHex(v[:8]+v[9:13]+v[14:18]+v[19:23]+v[24:])
}
func singleResource(profile string, t SingleTabSession) (string, error) {
	n, e := strconv.ParseUint(t.TabRef, 10, 32)
	origin, originErr := SingleTabOrigin(t.Origin)
	if e != nil || n < 1 || strconv.FormatUint(n, 10) != t.TabRef || t.ID == "" || t.BrowserInstance == "" || len(t.BrowserInstance) > 128 || t.ExpiresAt.IsZero() || originErr != nil || origin != t.Origin {
		return "", ErrDenied
	}
	return "browser-tab:" + hash(profile+"\x00"+t.BrowserInstance+"\x00"+t.TabRef), nil
}

// SingleTabOrigin strips path/query/fragment from the trusted selected URL. It
// grants no rights; only the owning selection guard may establish this value.
func SingleTabOrigin(raw string) (string, error) {
	u, e := url.Parse(raw)
	if e != nil || u.User != nil || u.Opaque != "" || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || strings.Contains(u.Hostname(), "%") {
		return "", ErrDenied
	}
	host, port := strings.ToLower(u.Hostname()), u.Port()
	if port != "" {
		n, e := strconv.Atoi(port)
		if e != nil || n < 1 || n > 65535 {
			return "", ErrDenied
		}
	}
	if (u.Scheme == "http" && port == "80") || (u.Scheme == "https" && port == "443") {
		port = ""
	}
	if strings.Contains(host, ":") {
		if net.ParseIP(host) == nil {
			return "", ErrDenied
		}
		host = "[" + host + "]"
	}
	if port != "" {
		host += ":" + port
	}
	return u.Scheme + "://" + host, nil
}
func (s *Store) singleSessionTx(ctx context.Context, tx pgx.Tx, b resources.Binding, expected string) (time.Time, error) {
	sel, ok := ctx.Value(selectionKey{}).(selection)
	if !ok || sel.single == nil || s.singleTab == nil {
		return time.Time{}, ErrDenied
	}
	actual, e := s.singleTab(ctx, tx, b, sel.single.Session.ID)
	if e != nil {
		return time.Time{}, ErrDenied
	}
	resource, e := singleResource(s.profile.ID, actual)
	if e != nil || (expected != "" && resource != expected) {
		return time.Time{}, ErrDenied
	}
	old := sel.single.Session
	if old.TabRef != "" && (old.ID != actual.ID || old.TabRef != actual.TabRef || old.BrowserInstance != actual.BrowserInstance || old.Origin != actual.Origin || !old.ExpiresAt.Equal(actual.ExpiresAt)) {
		return time.Time{}, ErrDenied
	}
	return actual.ExpiresAt, nil
}
func (s *Store) dispatchContext(ctx context.Context, d dispatch) context.Context {
	if d.single == nil {
		return ctx
	}
	return context.WithValue(ctx, selectionKey{}, selection{single: d.single})
}

// SpecFromContext is available only after the authenticated runtime Begin CAS.
// It is never a model/tool argument and does not replace backend page checks.
type runtimeSpecKey struct{}

func SpecFromContext(ctx context.Context) (RuntimeSpec, bool) {
	v, ok := ctx.Value(runtimeSpecKey{}).(RuntimeSpec)
	return v, ok
}
