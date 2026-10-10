// Package projectfixture is a disposable reviewed recipient for extension
// acceptance. It prepares an immutable patch for one synthetic file; it never
// applies a patch, runs code or resolves an argument as a filesystem path.
package projectfixture

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
)

const ProjectRef = "extension-project-fixture"
const Original = "Butler reviewed fixture\nready=false\n"
const Prepared = "Butler reviewed fixture\nready=true\n"

type Change struct {
	Path   string `json:"path"`
	Before string `json:"before"`
	After  string `json:"after"`
}
type Proposal struct {
	ProjectRevision string   `json:"project_revision"`
	Changes         []Change `json:"changes"`
}
type Patch struct {
	Key         string   `json:"key"`
	Fingerprint string   `json:"fingerprint"`
	ArtifactID  string   `json:"artifact_id"`
	Proposal    Proposal `json:"proposal"`
}
type Server struct {
	mu      sync.Mutex
	root    *os.Root
	patches map[string]Patch
}

func digest(raw string) string { s := sha256.Sum256([]byte(raw)); return hex.EncodeToString(s[:]) }
func Revision() string         { return digest(ProjectRef + "\x00" + Original) }
func checkFields(passed, produced bool) map[string]any {
	status, errorKind := "failed", "ready_flag_false"
	if passed {
		status, errorKind = "passed", ""
	}
	return map[string]any{"observation_format": "project_check_v1", "project_ref": ProjectRef, "revision_sha256": Revision(), "project_revision": Revision(), "checks_passed": passed, "error_kind": errorKind, "check_id": digest(ProjectRef + "\x00" + Revision() + "\x00ready_flag\x00" + status), "produced_by_butler": produced, "status": status}
}
func New(directory string) (*Server, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	s := &Server{root: root, patches: map[string]Patch{}}
	f, err := root.Open("patches.json")
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		root.Close()
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 4*1024*1024+1))
	if err != nil || len(data) > 4*1024*1024 || json.Unmarshal(data, &s.patches) != nil {
		root.Close()
		return nil, errors.New("fixture state invalid")
	}
	for key, p := range s.patches {
		if key != p.Key || !validPatch(p) {
			root.Close()
			return nil, errors.New("fixture state invalid")
		}
	}
	return s, nil
}
func (s *Server) Close() error { return s.root.Close() }
func validPatch(p Patch) bool {
	return len(p.Key) > 0 && len(p.Key) <= 160 && len(p.Fingerprint) == 64 && p.ArtifactID == digest(p.Key+"\x00"+p.Fingerprint) && p.Proposal.ProjectRevision == Revision() && len(p.Proposal.Changes) == 1 && p.Proposal.Changes[0].Path == "README.md" && p.Proposal.Changes[0].Before == Original && p.Proposal.Changes[0].After == Prepared
}
func (s *Server) persist(p Patch) error {
	copy := map[string]Patch{}
	for key, value := range s.patches {
		copy[key] = value
	}
	copy[p.Key] = p
	data, err := json.Marshal(copy)
	if err != nil || len(data) > 4*1024*1024 {
		return errors.New("fixture storage bound exceeded")
	}
	f, err := s.root.OpenFile("patches.pending", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = s.root.Rename("patches.pending", "patches.json"); err != nil {
		return err
	}
	d, err := s.root.Open(".")
	if err != nil {
		return err
	}
	err = d.Sync()
	d.Close()
	if err != nil {
		return err
	}
	s.patches = copy
	return nil
}
func reply(w http.ResponseWriter, code int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(value)
}
func receipt(p Patch) map[string]any {
	return map[string]any{"artifact_id": p.ArtifactID, "butler_effect_receipt": map[string]string{"idempotency_key": p.Key, "action_fingerprint": p.Fingerprint, "effect_state": "effect_confirmed", "artifact_id": p.ArtifactID}}
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" || r.URL.RawPath != "" {
		reply(w, 400, map[string]string{"error": "fixed fixture paths required"})
		return
	}
	if r.Method == "GET" && r.URL.Path == "/project/health" {
		reply(w, 200, map[string]bool{"ready": true})
		return
	}
	if r.Method == "GET" && r.URL.Path == "/project" {
		fields := checkFields(false, false)
		fields["files"] = []map[string]string{{"path": "README.md", "content": Original}}
		reply(w, 200, fields)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.Method == "POST" && r.URL.Path == "/project/patches" {
		var proposal Proposal
		d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		d.DisallowUnknownFields()
		if d.Decode(&proposal) != nil || d.Decode(new(any)) != io.EOF {
			reply(w, 400, map[string]string{"error": "bounded fixture proposal required"})
			return
		}
		p := Patch{Key: r.Header.Get("Idempotency-Key"), Fingerprint: r.Header.Get("Butler-Action-Fingerprint"), Proposal: proposal}
		p.ArtifactID = digest(p.Key + "\x00" + p.Fingerprint)
		if !validPatch(p) {
			reply(w, 400, map[string]string{"error": "exact synthetic file/revision/change required"})
			return
		}
		if old, exists := s.patches[p.Key]; exists {
			a, _ := json.Marshal(old)
			b, _ := json.Marshal(p)
			if string(a) != string(b) {
				reply(w, 409, map[string]string{"error": "immutable recipient identity changed"})
				return
			}
			reply(w, 200, receipt(old))
			return
		}
		if err := s.persist(p); err != nil {
			reply(w, 503, map[string]string{"error": "fixture storage unavailable"})
			return
		}
		reply(w, 201, receipt(p))
		return
	}
	if r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/project/effects/") {
		if p, ok := s.patches[strings.TrimPrefix(r.URL.Path, "/project/effects/")]; ok {
			reply(w, 200, receipt(p))
			return
		}
	}
	if r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/project/patches/") && strings.HasSuffix(r.URL.Path, "/checks") {
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/project/patches/"), "/checks")
		for _, p := range s.patches {
			if p.ArtifactID == id {
				fields := checkFields(true, true)
				fields["artifact_id"], fields["applied"] = id, false
				fields["checks"] = []map[string]string{{"name": "ready_flag", "status": "passed"}}
				reply(w, 200, fields)
				return
			}
		}
	}
	reply(w, 404, map[string]string{"error": "fixture resource absent"})
}
