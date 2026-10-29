package app

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/butler/butler/internal/extensions/projectfixture"
)

// Test-only recipient dataset/fault wrapper for the existing reviewed HTTP ABI.
// It never applies a patch or resolves a request field as a filesystem path.
// No lesson/control oracle is accepted here: only current files and fixed faults.
type experienceRecipientConfig struct {
	Version string            `json:"version"`
	Ready   string            `json:"accepted_ready_value"`
	Files   map[string]string `json:"initial_files"`
	Outage  bool              `json:"receipt_outage"`
}

type experienceRecipientEntry struct {
	Key, Fingerprint, Resource, State, Artifact string
	Proposal                                    projectfixture.Proposal
}

type experienceRecipientState struct {
	Config     experienceRecipientConfig           `json:"config"`
	Entries    map[string]experienceRecipientEntry `json:"entries"`
	Dispatches int                                 `json:"dispatches"`
}

type experienceRecipient struct {
	mu           sync.Mutex
	root         *os.Root
	state        experienceRecipientState
	httpStatuses []experienceRecipientHTTPStatus
}

// Independent private HTTP evidence only. It supplies no effect state, runtime
// result, model input or lesson source and is not a replacement for Reconcile.
type experienceRecipientHTTPStatus struct {
	Method   string `json:"method"`
	Resource string `json:"resource"`
	Status   int    `json:"status"`
}
type experienceRecipientStatusWriter struct {
	http.ResponseWriter
	observe func(int)
}

func (w *experienceRecipientStatusWriter) WriteHeader(code int) {
	w.observe(code)
	w.ResponseWriter.WriteHeader(code)
}
func (w *experienceRecipientStatusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("recipient connection cannot be hijacked")
	}
	return h.Hijack()
}
func (r *experienceRecipient) httpStatusReceipts() []experienceRecipientHTTPStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]experienceRecipientHTTPStatus(nil), r.httpStatuses...)
}

func experienceDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func experienceEnvironment(c experienceRecipientConfig) string {
	// Instance file bytes/canaries/resources are exact revision proof, not the
	// compatibility version. Both v1 alpha and beta share the same environment.
	raw, _ := json.Marshal(struct {
		Version string
		Ready   string
		Outage  bool
	}{c.Version, c.Ready, c.Outage})
	return experienceDigest("reviewed-http-project-experience-v1\x00" + string(raw))
}

func newExperienceRecipient(directory string, config experienceRecipientConfig) (*experienceRecipient, error) {
	if config.Version != "v1" && config.Version != "v2" || config.Ready != "true" && config.Ready != "verified" || config.Version == "v1" && config.Ready != "true" || config.Version == "v2" && config.Ready != "verified" || len(config.Files) < 1 || len(config.Files) > 2 {
		return nil, errors.New("invalid bounded experience recipient config")
	}
	for resource, file := range config.Files {
		if resource != "alpha" && resource != "beta" || len(file) > 2048 || strings.Count(file, "ready=") != 1 || !strings.Contains(file, "ready=false\n") && !strings.Contains(file, "ready=pending\n") {
			return nil, errors.New("invalid current recipient file")
		}
	}
	files := make(map[string]string, len(config.Files))
	for resource, file := range config.Files {
		files[resource] = file
	}
	config.Files = files
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	r := &experienceRecipient{root: root, state: experienceRecipientState{Config: config, Entries: map[string]experienceRecipientEntry{}}}
	file, err := root.Open("recipient.json")
	if errors.Is(err, os.ErrNotExist) {
		return r, nil
	}
	if err != nil {
		root.Close()
		return nil, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, 4*1024*1024+1))
	expected, _ := json.Marshal(config)
	if err != nil || len(raw) > 4*1024*1024 || json.Unmarshal(raw, &r.state) != nil {
		root.Close()
		return nil, errors.New("recipient state unavailable")
	}
	actual, _ := json.Marshal(r.state.Config)
	if string(expected) != string(actual) || len(r.state.Entries) > 4096 || r.state.Dispatches < len(r.state.Entries) {
		root.Close()
		return nil, errors.New("recipient configuration or audit changed")
	}
	for key, entry := range r.state.Entries {
		if key != entry.Key || !experienceIdentity(entry.Key, entry.Fingerprint) || config.Files[entry.Resource] == "" || entry.State != "effect_confirmed" && entry.State != "effect_absent" || entry.State == "effect_confirmed" && (entry.Artifact != experienceDigest(entry.Key+"\x00"+entry.Fingerprint) || !r.validProposal(entry.Resource, entry.Proposal)) || entry.State == "effect_absent" && entry.Artifact != "" {
			root.Close()
			return nil, errors.New("recipient immutable artifact audit invalid")
		}
	}
	return r, nil
}

func experienceIdentity(key, fingerprint string) bool {
	_, err := hex.DecodeString(fingerprint)
	return len(key) > 0 && len(key) <= 160 && len(fingerprint) == 64 && err == nil
}

func (r *experienceRecipient) close() error { return r.root.Close() }

// Hidden evaluator audit, never a model/tool/lesson observation. Copy under the
// same mutex as persistence so tests do not race HTTP handler completion.
func (r *experienceRecipient) snapshot() experienceRecipientState {
	r.mu.Lock()
	defer r.mu.Unlock()
	raw, _ := json.Marshal(r.state)
	var snapshot experienceRecipientState
	_ = json.Unmarshal(raw, &snapshot)
	return snapshot
}

func (r *experienceRecipient) revision(resource string) string {
	return experienceDigest(resource + "\x00" + r.state.Config.Files[resource])
}
func (r *experienceRecipient) validProposal(resource string, p projectfixture.Proposal) bool {
	original := r.state.Config.Files[resource]
	if original == "" || p.ProjectRevision != r.revision(resource) || len(p.Changes) != 1 || p.Changes[0].Path != "README.md" || p.Changes[0].Before != original {
		return false
	}
	ready := strings.ReplaceAll(original, "ready=false\n", "ready="+r.state.Config.Ready+"\n")
	ready = strings.ReplaceAll(ready, "ready=pending\n", "ready="+r.state.Config.Ready+"\n")
	return p.Changes[0].After == ready
}

func (r *experienceRecipient) persist(next experienceRecipientState) error {
	raw, err := json.Marshal(next)
	if err != nil || len(raw) > 4*1024*1024 {
		return errors.New("recipient finite state exhausted")
	}
	file, err := r.root.OpenFile("recipient.pending", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	if _, err = file.Write(raw); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = r.root.Rename("recipient.pending", "recipient.json"); err != nil {
		return err
	}
	dir, err := r.root.Open(".")
	if err != nil {
		return err
	}
	err = dir.Sync()
	dir.Close()
	if err == nil {
		r.state = next
	}
	return err
}

func experienceReply(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}
func experienceReceipt(e experienceRecipientEntry) map[string]any {
	return map[string]any{"artifact_id": e.Artifact, "butler_effect_receipt": map[string]string{"idempotency_key": e.Key, "action_fingerprint": e.Fingerprint, "effect_state": e.State, "artifact_id": e.Artifact}}
}

func (r *experienceRecipient) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	w = &experienceRecipientStatusWriter{ResponseWriter: w, observe: func(code int) {
		if len(r.httpStatuses) < 4096 && len(req.URL.Path) <= 2048 && len(req.Method) <= 16 {
			r.httpStatuses = append(r.httpStatuses, experienceRecipientHTTPStatus{Method: req.Method, Resource: req.URL.Path, Status: code})
		}
	}}
	if req.URL.RawPath != "" || req.URL.RawQuery != "" || !strings.HasPrefix(req.URL.Path, "/project") {
		if req.Method == http.MethodGet && req.URL.RawPath == "" && req.URL.Path == "/project/alpha" && req.URL.RawQuery == "validate_ready=maybe" && r.state.Config.Files["alpha"] != "" {
			experienceReply(w, 200, map[string]any{"observation_format": "project_validation_v1", "project_ref": "alpha", "validated_ready": "maybe", "rejection_code": "unsupported_ready_value", "effect_absent": len(r.state.Entries) == 0, "applied": false, "checks_passed": false})
			return
		}
		experienceReply(w, 400, map[string]string{"error": "fixed recipient resource required"})
		return
	}
	if req.Method == http.MethodGet && req.URL.Path == "/project/health" {
		experienceReply(w, 200, map[string]bool{"ready": true})
		return
	}
	if req.Method == http.MethodGet && strings.HasPrefix(req.URL.Path, "/project/effects/") {
		entry, ok := r.state.Entries[strings.TrimPrefix(req.URL.Path, "/project/effects/")]
		if r.state.Config.Outage {
			experienceReply(w, 503, map[string]string{"error": "recipient verification unavailable"})
		} else if !ok {
			experienceReply(w, 404, map[string]string{"error": "unobserved recipient identity"})
		} else {
			experienceReply(w, 200, experienceReceipt(entry))
		}
		return
	}
	parts := strings.Split(strings.TrimPrefix(req.URL.Path, "/project/"), "/")
	resource := parts[0]
	if r.state.Config.Files[resource] == "" {
		experienceReply(w, 404, map[string]string{"error": "recipient resource absent"})
		return
	}
	if req.Method == http.MethodGet && len(parts) == 1 {
		experienceReply(w, 200, map[string]any{"observation_format": "project_check_v1", "project_ref": resource, "project_revision": r.revision(resource), "revision_sha256": r.revision(resource), "environment_sha256": experienceEnvironment(r.state.Config), "accepted_ready_value": r.state.Config.Ready, "checks_passed": false, "status": "failed", "error_kind": "ready_flag_false", "files": []map[string]string{{"path": "README.md", "content": r.state.Config.Files[resource]}}})
		return
	}
	if req.Method == http.MethodPost && len(parts) == 2 && parts[1] == "patches" {
		var proposal projectfixture.Proposal
		decoder := json.NewDecoder(http.MaxBytesReader(w, req.Body, 4096))
		decoder.DisallowUnknownFields()
		key, fingerprint := req.Header.Get("Idempotency-Key"), req.Header.Get("Butler-Action-Fingerprint")
		if decoder.Decode(&proposal) != nil || decoder.Decode(new(any)) != io.EOF || !experienceIdentity(key, fingerprint) {
			experienceReply(w, 400, map[string]string{"error": "bounded keyed proposal required"})
			return
		}
		entry := experienceRecipientEntry{Key: key, Fingerprint: fingerprint, Resource: resource, State: "effect_absent", Proposal: proposal}
		if r.validProposal(resource, proposal) {
			entry.State, entry.Artifact = "effect_confirmed", experienceDigest(key+"\x00"+fingerprint)
		}
		if prior, ok := r.state.Entries[key]; ok {
			a, _ := json.Marshal(prior)
			b, _ := json.Marshal(entry)
			if string(a) != string(b) {
				experienceReply(w, 409, map[string]string{"error": "immutable recipient identity changed"})
				return
			}
		} else if len(r.state.Entries) >= 4096 {
			experienceReply(w, 503, map[string]string{"error": "recipient finite state exhausted"})
			return
		}
		next := r.state
		next.Entries = make(map[string]experienceRecipientEntry, len(r.state.Entries)+1)
		for k, e := range r.state.Entries {
			next.Entries[k] = e
		}
		next.Entries[key], next.Dispatches = entry, next.Dispatches+1
		if r.persist(next) != nil {
			experienceReply(w, 503, map[string]string{"error": "recipient state unavailable"})
			return
		}
		if entry.State == "effect_absent" {
			// A mutation rejection is not itself typed absence. Only the separate
			// exact key/fingerprint lookup provides the contracted absence receipt.
			experienceReply(w, 400, map[string]string{"error": "exact current file/revision/ready value required"})
			return
		}
		if r.state.Config.Outage {
			conn, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				_ = conn.Close()
			}
			return
		}
		experienceReply(w, 201, experienceReceipt(entry))
		return
	}
	if req.Method == http.MethodGet && len(parts) == 4 && parts[1] == "patches" && parts[3] == "checks" {
		if r.state.Config.Outage {
			experienceReply(w, 503, map[string]string{"error": "recipient verification unavailable"})
			return
		}
		for _, entry := range r.state.Entries {
			if entry.Resource == resource && entry.Artifact == parts[2] && entry.State == "effect_confirmed" {
				experienceReply(w, 200, map[string]any{"observation_format": "project_check_v1", "project_ref": resource, "project_revision": r.revision(resource), "artifact_id": entry.Artifact, "checks_passed": true, "applied": false, "status": "passed", "prepared_content": entry.Proposal.Changes[0].After})
				return
			}
		}
	}
	experienceReply(w, 404, map[string]string{"error": "recipient resource absent"})
}
