package extensions

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	"github.com/butler/butler/internal/owneridentity"
)

type ControlRequest struct {
	Operation      string                              `json:"operation"`
	ExtensionID    string                              `json:"extension_id"`
	ManifestHash   string                              `json:"manifest_hash"`
	Generation     int64                               `json:"generation"`
	ManifestJSON   json.RawMessage                     `json:"manifest_json,omitempty"`
	ManifestText   string                              `json:"manifest_text,omitempty"`
	Configuration  map[string]string                   `json:"configuration"`
	CredentialRefs map[string]actionauth.CredentialRef `json:"credential_refs"`
	Tools          []string                            `json:"tools"`
	Access         TaskAccess                          `json:"access"`
}
type controlEnvelope struct {
	Actor   owneridentity.Actor `json:"actor"`
	Request ControlRequest      `json:"request"`
}
type ControlItem struct {
	Record   Record   `json:"state"`
	Manifest Manifest `json:"manifest"`
}
type ControlResponse struct {
	Items      []ControlItem     `json:"items,omitempty"`
	Item       *ControlItem      `json:"item,omitempty"`
	Versions   []ManifestVersion `json:"versions,omitempty"`
	StagedHash string            `json:"staged_hash,omitempty"`
	Access     *TaskAccess       `json:"access,omitempty"`
	Error      string            `json:"error,omitempty"`
}
type ManifestVersion struct {
	Manifest Manifest `json:"manifest"`
	Digest   string   `json:"manifest_hash"`
}
type Prober interface {
	ProbeExtension(context.Context, string, *runtimev1.ProbeExtensionRequest) (*runtimev1.ProbeExtensionResponse, error)
}

func DecodeControlRequest(raw []byte, request *ControlRequest) error {
	if len(raw) > MaxManifestBytes+128*1024 || strictJSON(raw) != nil {
		return ErrManifest
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(request) != nil || d.Decode(new(any)) != io.EOF {
		return ErrManifest
	}
	return nil
}

func NewControlHandler(token string, store *Store, prober Prober) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(token) < 32 || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			http.Error(w, "private extension control authentication required", http.StatusUnauthorized)
			return
		}
		if r.Method != "POST" || r.URL.RawQuery != "" {
			http.Error(w, "fixed extension control POST required", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxManifestBytes+128*1024))
		if err != nil || strictJSON(body) != nil {
			http.Error(w, "invalid reviewed extension request", http.StatusBadRequest)
			return
		}
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.DisallowUnknownFields()
		var envelope controlEnvelope
		if decoder.Decode(&envelope) != nil || decoder.Decode(new(any)) != io.EOF || len(envelope.Actor.DeliveryID) > 512 {
			http.Error(w, "invalid owner control proof", http.StatusBadRequest)
			return
		}
		ctx := owneridentity.WithActor(r.Context(), envelope.Actor)
		if store == nil {
			http.Error(w, "reviewed extension control unavailable", http.StatusServiceUnavailable)
			return
		}
		if _, err = store.actor(ctx); err != nil {
			http.Error(w, "authenticated configured owner required", http.StatusForbidden)
			return
		}
		response, err := store.Control(ctx, envelope.Request, prober)
		if err != nil {
			response.Error = safeControlError(err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		if err != nil {
			w.WriteHeader(http.StatusConflict)
		}
		_ = json.NewEncoder(w).Encode(response)
	})
}
func safeControlError(err error) string {
	var exhausted *budget.ExhaustedError
	switch {
	case errors.Is(err, ErrManifest):
		return "manifest, schema, compatibility or configuration rejected"
	case errors.Is(err, ErrConflict):
		return "extension version or configuration changed"
	case errors.Is(err, ErrAccess):
		return "current configured owner or exact extension permission required"
	case errors.As(err, &exhausted):
		return exhausted.Error()
	default:
		return "reviewed extension readiness or storage unavailable"
	}
}
func (s *Store) Control(ctx context.Context, request ControlRequest, prober Prober) (ControlResponse, error) {
	result := ControlResponse{}
	var r Record
	var err error
	switch request.Operation {
	case "list":
		items, e := s.List(ctx)
		if e != nil {
			return result, e
		}
		for _, item := range items {
			result.Items = append(result.Items, ControlItem{item, item.Package.Manifest})
		}
		return result, nil
	case "stage":
		manifest := request.ManifestJSON
		if request.ManifestText != "" {
			if len(manifest) != 0 {
				return result, ErrManifest
			}
			manifest = []byte(request.ManifestText)
		}
		p, e := Parse(bytes.NewReader(manifest))
		if e != nil {
			return result, e
		}
		r, err = s.Stage(ctx, p)
		result.StagedHash = p.Digest
	case "review":
		err = s.Review(ctx, request.ExtensionID, request.ManifestHash)
	case "configure", "rollback":
		r, err = s.Configure(ctx, request.ExtensionID, request.ManifestHash, request.Generation, request.Configuration, request.CredentialRefs, request.Tools)
	case "enable", "disable":
		r, err = s.SetEnabled(ctx, request.ExtensionID, request.Generation, request.Operation == "enable")
	case "probe":
		if prober == nil {
			return result, ErrUnavailable
		}
		var p Probe
		p, r, err = s.BeginProbe(ctx, request.ExtensionID, request.Generation)
		if err != nil {
			break
		}
		descriptor, target, e := s.ProbeContract(r)
		if e != nil {
			return result, e
		}
		probeCtx, cancel := context.WithTimeout(ctx, time.Duration(descriptor.GetTimeoutMs())*time.Millisecond)
		defer cancel()
		observation, e := prober.ProbeExtension(probeCtx, target, &runtimev1.ProbeExtensionRequest{ProbeId: p.ID, Url: p.URL, Extension: descriptor})
		if e != nil || observation == nil {
			err = ErrUnavailable
			_ = s.markProbeUnknown(context.WithoutCancel(ctx), p)
		} else {
			err = s.FinishProbe(ctx, p, observation.GetReady(), observation.GetResponseDigest())
		}
	case "grant_task":
		err = s.GrantTask(ctx, request.ExtensionID, request.Access)
	case "task_access":
		a, e := s.TaskAccess(ctx, request.Access.TaskID, request.ExtensionID)
		if e != nil {
			return result, e
		}
		result.Access = &a
		return result, nil
	case "versions":
		versions, e := s.Versions(ctx, request.ExtensionID)
		if e != nil {
			return result, e
		}
		for _, v := range versions {
			result.Versions = append(result.Versions, ManifestVersion{v.Manifest, v.Digest})
		}
		return result, nil
	default:
		return result, ErrManifest
	}
	if r.ID == "" && request.ExtensionID != "" {
		r, _ = s.Get(ctx, request.ExtensionID)
	}
	if r.ID != "" {
		fresh, e := s.Get(ctx, r.ID)
		if e == nil {
			r = fresh
		}
		result.Item = &ControlItem{r, r.Package.Manifest}
	}
	return result, err
}
func (s *Store) markProbeUnknown(ctx context.Context, p Probe) error {
	if _, err := s.actor(ctx); err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = s.read(ctx, tx, p.ExtensionID, "FOR UPDATE OF i"); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE extension_probes SET state='unknown',finished_at=clock_timestamp() WHERE probe_id=$1 AND state='executing'`, p.ID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE extension_installations SET health='failed',health_reason='probe_unknown',healthy_until=NULL WHERE extension_id=$1 AND manifest_hash=$2 AND generation=$3`, p.ExtensionID, p.ManifestHash, p.Generation)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ControlClient is private backend RPC with typed owner proof. It never accepts
// URLs/tokens from the model or forwards channel authorization headers.
type ControlClient struct {
	endpoint, token string
	client          *http.Client
}

func NewControlClient(endpoint, token string) *ControlClient {
	return &ControlClient{endpoint, token, &http.Client{Timeout: 35 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return ErrAccess }}}
}
func (c *ControlClient) Control(ctx context.Context, request ControlRequest) (ControlResponse, error) {
	actor, ok := owneridentity.FromContext(ctx)
	if !ok || c == nil || c.endpoint == "" || len(c.token) < 32 {
		return ControlResponse{}, ErrUnavailable
	}
	body, err := json.Marshal(controlEnvelope{actor, request})
	if err != nil || len(body) > MaxManifestBytes+128*1024 {
		return ControlResponse{}, ErrManifest
	}
	r, err := http.NewRequestWithContext(ctx, "POST", c.endpoint, bytes.NewReader(body))
	if err != nil {
		return ControlResponse{}, ErrUnavailable
	}
	r.Header.Set("Authorization", "Bearer "+c.token)
	r.Header.Set("Content-Type", "application/json")
	response, err := c.client.Do(r)
	if err != nil {
		return ControlResponse{}, ErrUnavailable
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 2*1024*1024+1))
	if err != nil || len(data) > 2*1024*1024 {
		return ControlResponse{}, ErrUnavailable
	}
	var result ControlResponse
	if json.Unmarshal(data, &result) != nil {
		return result, ErrUnavailable
	}
	if response.StatusCode != 200 {
		return result, ErrUnavailable
	}
	return result, nil
}
