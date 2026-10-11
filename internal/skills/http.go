package skills

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"github.com/butler/butler/internal/owneridentity"
	"io"
	"net/http"
	"net/url"
	"time"
)

type ControlRequest struct {
	Operation    string          `json:"operation"`
	Digest       string          `json:"digest"`
	Generation   int64           `json:"generation"`
	ManifestText string          `json:"manifest_text"`
	Grant        CorrectionGrant `json:"grant"`
}
type ControlResponse struct {
	Items      []Record       `json:"items,omitempty"`
	Item       *Record        `json:"item,omitempty"`
	Validation *Validation    `json:"validation,omitempty"`
	Usages     []Usage        `json:"usages,omitempty"`
	Quality    *QualityReport `json:"quality,omitempty"`
	Error      string         `json:"error,omitempty"`
}
type request struct {
	Actor   *owneridentity.Actor `json:"actor,omitempty"`
	Control *ControlRequest      `json:"control,omitempty"`
	RunID   string               `json:"run_id,omitempty"`
	Query   string               `json:"query,omitempty"`
	Pinned  bool                 `json:"pinned,omitempty"`
}
type response struct {
	Control *ControlResponse `json:"control,omitempty"`
	Index   []Descriptor     `json:"index,omitempty"`
	Pinned  []Package        `json:"pinned,omitempty"`
}

func (s *Store) Control(ctx context.Context, r ControlRequest) (ControlResponse, error) {
	result := ControlResponse{}
	var err error
	var item Record
	if _, err = s.actor(ctx); err != nil {
		return result, err
	}
	switch r.Operation {
	case "list":
		result.Items, err = s.List(ctx)
	case "stage":
		var p Package
		p, err = Parse([]byte(r.ManifestText))
		if err == nil {
			item, err = s.Stage(ctx, p)
		}
	case "test":
		var v Validation
		v, err = s.Test(ctx, r.Digest)
		result.Validation = &v
	case "activate", "rollback":
		item, err = s.Activate(ctx, r.Digest, r.Generation)
	case "disable", "revoke":
		item, err = s.Disable(ctx, r.Digest, r.Generation, r.Operation == "revoke")
	case "grant_correction":
		err = s.GrantCorrection(ctx, r.Grant)
	case "revoke_correction":
		err = s.RevokeCorrection(ctx, r.Grant.ID)
	case "usages":
		result.Usages, err = s.Usages(ctx, r.Digest)
	case "quality":
		var q QualityReport
		q, err = s.Quality(ctx, r.Digest)
		result.Quality = &q
	default:
		err = ErrManifest
	}
	if item.ID != "" {
		result.Item = &item
	}
	if err != nil {
		result.Error = safeError(err)
	}
	return result, err
}
func safeError(err error) string {
	switch err {
	case ErrManifest, ErrAccess, ErrConflict, ErrEvidence, ErrUnavailable:
		return err.Error()
	default:
		return "reviewed skill storage or verification unavailable"
	}
}
func privateRequest(w http.ResponseWriter, r *http.Request, token string, target any) bool {
	if len(token) < 32 || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
		http.Error(w, "private skill authentication required", http.StatusUnauthorized)
		return false
	}
	if r.Method != "POST" || r.URL.RawQuery != "" {
		http.Error(w, "fixed skill POST required", http.StatusMethodNotAllowed)
		return false
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxBytes))
	if err != nil || Decode(body, target) != nil {
		http.Error(w, "invalid skill request", http.StatusBadRequest)
		return false
	}
	return true
}
func NewHandler(token string, s *Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req request
		if !privateRequest(w, r, token, &req) {
			return
		}
		if s == nil {
			http.Error(w, "skills unavailable", http.StatusServiceUnavailable)
			return
		}
		result := response{}
		var err error
		if req.Control != nil && req.Actor != nil && req.RunID == "" && !req.Pinned && req.Query == "" {
			ctx := owneridentity.WithActor(r.Context(), *req.Actor)
			var c ControlResponse
			c, err = s.Control(ctx, *req.Control)
			result.Control = &c
		} else if req.RunID != "" && req.Actor == nil && req.Control == nil {
			if req.Pinned {
				result.Pinned, err = s.Pinned(r.Context(), req.RunID)
			} else {
				result.Index, err = s.Index(r.Context(), req.RunID, req.Query, 12)
			}
		} else {
			err = ErrAccess
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		if err != nil {
			w.WriteHeader(http.StatusConflict)
		}
		_ = json.NewEncoder(w).Encode(result)
	})
}

type sourceRequest struct {
	OwnerID string        `json:"owner_id"`
	Sources []EvidenceRef `json:"sources"`
	Usages  []Usage       `json:"usages,omitempty"`
}
type sourceResponse struct {
	Proofs   []SourceProof `json:"proofs"`
	Outcomes []Outcome     `json:"outcomes,omitempty"`
}

func NewEvidenceHandler(token string, v EvidenceVerifier) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req sourceRequest
		if !privateRequest(w, r, token, &req) {
			return
		}
		if v == nil || req.OwnerID == "" || len(req.Sources) > 8 || len(req.Usages) > 200 || (len(req.Sources) == 0) == (len(req.Usages) == 0) {
			http.Error(w, "source verification unavailable", http.StatusForbidden)
			return
		}
		result := sourceResponse{}
		var err error
		if len(req.Sources) > 0 {
			result.Proofs, err = v.VerifySkillSources(r.Context(), req.OwnerID, req.Sources)
		} else if reader, ok := v.(OutcomeReader); ok {
			result.Outcomes, err = reader.ReadSkillOutcomes(r.Context(), req.OwnerID, req.Usages)
		} else {
			err = ErrUnavailable
		}
		if err != nil {
			http.Error(w, "verified skill sources required", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(result)
	})
}

type Client struct {
	endpoint, token string
	client          *http.Client
}

func NewClient(endpoint, token string) *Client {
	return &Client{endpoint, token, &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

// SiblingEndpoint derives a fixed path on the operator's existing private
// backend endpoint; no source/model URL or arbitrary redirect is accepted.
func SiblingEndpoint(endpoint, path string) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.User != nil || u.Hostname() == "" || u.RawQuery != "" || u.Fragment != "" || u.Scheme != "http" && u.Scheme != "https" {
		return "", ErrUnavailable
	}
	u.Path = path
	u.RawPath = ""
	return u.String(), nil
}
func (c *Client) read(ctx context.Context, req any, target any) error {
	if c == nil || c.endpoint == "" || len(c.token) < 32 {
		return ErrUnavailable
	}
	body, err := json.Marshal(req)
	if err != nil || len(body) > MaxBytes {
		return ErrManifest
	}
	r, err := http.NewRequestWithContext(ctx, "POST", c.endpoint, bytes.NewReader(body))
	if err != nil {
		return ErrUnavailable
	}
	r.Header.Set("Authorization", "Bearer "+c.token)
	r.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(r)
	if err != nil {
		return ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return ErrUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024+1))
	if err != nil || len(raw) > 2*1024*1024 {
		return ErrUnavailable
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(target) != nil || d.Decode(new(any)) != io.EOF {
		return ErrUnavailable
	}
	return nil
}
func (c *Client) Control(ctx context.Context, control ControlRequest) (ControlResponse, error) {
	a, ok := owneridentity.FromContext(ctx)
	if !ok {
		return ControlResponse{}, ErrAccess
	}
	var r response
	if err := c.read(ctx, request{Actor: &a, Control: &control}, &r); err != nil {
		return ControlResponse{}, err
	}
	if r.Control == nil {
		return ControlResponse{}, ErrUnavailable
	}
	return *r.Control, nil
}
func (c *Client) Index(ctx context.Context, run string) ([]Descriptor, error) {
	var r response
	err := c.read(ctx, request{RunID: run}, &r)
	return r.Index, err
}
func (c *Client) Pinned(ctx context.Context, run string) ([]Package, error) {
	var r response
	err := c.read(ctx, request{RunID: run, Pinned: true}, &r)
	return r.Pinned, err
}
func (c *Client) VerifySkillSources(ctx context.Context, owner string, refs []EvidenceRef) ([]SourceProof, error) {
	var r sourceResponse
	err := c.read(ctx, sourceRequest{OwnerID: owner, Sources: refs}, &r)
	return r.Proofs, err
}

func (c *Client) ReadSkillOutcomes(ctx context.Context, owner string, usages []Usage) ([]Outcome, error) {
	var r sourceResponse
	err := c.read(ctx, sourceRequest{OwnerID: owner, Usages: usages}, &r)
	return r.Outcomes, err
}
