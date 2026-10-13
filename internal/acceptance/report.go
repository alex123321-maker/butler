package acceptance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type GroupProof struct {
	ID           string      `json:"id"`
	Runtime      string      `json:"runtime"`
	Status       string      `json:"status"`
	Reason       string      `json:"reason,omitempty"`
	Tests        []TestProof `json:"tests,omitempty"`
	EventsSHA256 string      `json:"events_sha256,omitempty"`
}

type ScenarioProof struct {
	Scenario
	Status string `json:"status"`
}

type Report struct {
	Version           string          `json:"version"`
	CreatedUTC        time.Time       `json:"created_utc"`
	CodeCommit        string          `json:"code_commit"`
	SourceTreeSHA256  string          `json:"source_tree_sha256"`
	SchemaSHA256      string          `json:"schema_sha256"`
	CorpusDigest      string          `json:"corpus_digest"`
	CriteriaDigest    string          `json:"criteria_sha256"`
	Hardware          json.RawMessage `json:"hardware"`
	Profile           string          `json:"profile"`
	SelectedPassed    bool            `json:"selected_passed"`
	Issue33Ready      bool            `json:"issue_33_ready"`
	Groups            []GroupProof    `json:"groups"`
	Scenarios         []ScenarioProof `json:"scenarios"`
	MeasurementLimits []string        `json:"measurement_limits"`
}

func FileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func TreeHash(root string, filter func(string) bool) (string, error) {
	paths := []string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == ".cache" || d.Name() == "node_modules" || d.Name() == "reports" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("source tree symlink denied: %s", d.Name())
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if filter == nil || filter(rel) {
			paths = append(paths, rel)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(paths)
	h := sha256.New()
	for _, path := range paths {
		sum, err := FileHash(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%s\x00%s\n", path, sum)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func BuildReport(root, events, profile, commit string, hardware json.RawMessage, c Corpus) (Report, error) {
	if !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(commit) || !json.Valid(hardware) {
		return Report{}, fmt.Errorf("exact commit and hardware manifest required")
	}
	if profile != "core" && profile != "full" {
		return Report{}, fmt.Errorf("profile must be core or full")
	}
	r := Report{Version: "butler-fault-matrix-v1", CreatedUTC: time.Now().UTC(), CodeCommit: commit, CorpusDigest: CorpusDigest(c), Hardware: hardware, Profile: profile, SelectedPassed: true, Issue33Ready: false}
	r.CriteriaDigest = CriteriaDigest()
	var err error
	r.SourceTreeSHA256, err = TreeHash(root, nil)
	if err != nil {
		return r, err
	}
	if _, err = os.Stat(filepath.Join(root, "migrations")); err != nil {
		return r, fmt.Errorf("migration source missing: %w", err)
	}
	r.SchemaSHA256, err = TreeHash(root, func(s string) bool { return strings.HasPrefix(s, "migrations/") })
	if err != nil {
		return r, err
	}
	states := map[string]string{}
	for _, g := range Groups() {
		p := GroupProof{ID: g.ID, Runtime: g.Runtime, Status: "PENDING"}
		selected := profile == "full" || g.Runtime == "go-race"
		unavailable := false
		if g.ID == "extensions" {
			if _, statErr := os.Stat(filepath.Join(root, g.Package)); statErr != nil {
				unavailable = true
				selected = false
			}
		}
		path := filepath.Join(events, g.ID+".jsonl")
		if unavailable {
			p.Reason = "installed-extension sources absent from this snapshot; replay integrated accepted #18 release required"
			states[p.ID] = p.Status
			r.Groups = append(r.Groups, p)
			continue
		}
		f, err := os.Open(path)
		if err != nil {
			if unavailable {
				p.Reason = "installed-extension sources absent from this snapshot; replay integrated accepted #18 release required"
			} else if selected {
				p.Status = "FAIL"
				p.Reason = "required event artifact missing"
			} else {
				p.Reason = "not selected by core profile"
			}
		} else {
			p.Tests, err = VerifyEvents(f, g)
			f.Close()
			p.EventsSHA256, _ = FileHash(path)
			p.Status = "PASS"
			if err != nil {
				p.Status = "FAIL"
				p.Reason = err.Error()
			}
			for _, t := range p.Tests {
				if t.Status != "PASS" {
					p.Status = "FAIL"
				}
			}
		}
		if selected && p.Status != "PASS" {
			r.SelectedPassed = false
		}
		states[p.ID] = p.Status
		r.Groups = append(r.Groups, p)
	}
	for _, s := range Scenarios() {
		p := ScenarioProof{Scenario: s, Status: "PASS"}
		for _, g := range s.Groups {
			if states[g] == "FAIL" {
				p.Status = "FAIL"
				break
			}
			if states[g] != "PASS" {
				p.Status = "PENDING"
			}
		}
		if s.Pending != "" && p.Status != "FAIL" {
			p.Status = "PENDING"
		}
		r.Scenarios = append(r.Scenarios, p)
	}
	r.MeasurementLimits = []string{"Go/Chromium tests use synthetic credentials and fixture effects; passing tests assert contracts, not broad model task success.", "Controlled worker OOM is not measured operating-system OOM of deployed weights.", "Model accuracy, missed-event/false-initiative rates and single/multi-agent full cost are unmeasured until independently approved pinned observation runs exist.", "Issue #33 cannot pass while release-chain/delegation/skills/repair dependencies remain pending. No skipped required test counts as proof."}
	return r, nil
}

// RunCore launches only fixed packages/tests. No model-controlled command or
// environment is accepted. Docker/network/cache isolation is owned by fault-lab.sh.
func RunCore(ctx context.Context, root, events string) error {
	return runGroups(ctx, root, events, Groups())
}

func RunFullFaultCore(ctx context.Context, root, events string) error {
	return runGroups(ctx, root, events, FullFaultGroups())
}

func runGroups(ctx context.Context, root, events string, groups []Group) error {
	if err := os.MkdirAll(events, 0700); err != nil {
		return err
	}
	var failures []string
	for _, g := range groups {
		if g.Runtime != "go-race" {
			continue
		}
		if g.ID == "extensions" {
			if _, err := os.Stat(filepath.Join(root, g.Package)); err != nil {
				fmt.Println("acceptance group=extensions status=PENDING source_absent")
				continue
			}
		}
		for _, name := range g.Environment {
			if os.Getenv(name) == "" {
				return fmt.Errorf("required synthetic test environment missing: %s", name)
			}
		}
		pattern := []string{}
		for _, name := range g.Tests {
			pattern = append(pattern, regexp.QuoteMeta(name))
		}
		groupCtx, cancel := context.WithTimeout(ctx, 180*time.Second)
		cmd := exec.CommandContext(groupCtx, "go", "test", "-json", "-race", "-p", "1", "-count=1", "-timeout=150s", "-run", "^("+strings.Join(pattern, "|")+")$", "./"+g.Package)
		cmd.Dir = root
		cmd.Env = os.Environ()
		// Never share fixture tables between groups, even when a fixture resets its
		// public schema. DSNs point solely to the disposable harness server.
		for _, name := range []string{"BUTLER_TEST_POSTGRES_URL", "BUTLER_TEST_GOALS_POSTGRES_URL", "BUTLER_TEST_ACTIONAUTH_POSTGRES_URL", "BUTLER_TEST_COMPLETION_POSTGRES_URL", "BUTLER_TEST_MEMORY_POSTGRES_URL", "BUTLER_TEST_SEMANTIC_POSTGRES_URL"} {
			raw := os.Getenv(name)
			u, err := url.Parse(raw)
			if err != nil || u.Host != "acceptance-postgres:5432" || u.Path != "/butler_acceptance_template_test" || u.Scheme != "postgres" {
				cancel()
				return fmt.Errorf("disposable acceptance database required: %s", name)
			}
			cmd.Env = append(cmd.Env, name+"="+strings.ReplaceAll(raw, "butler_acceptance_template_test", "butler_acceptance_"+strings.ReplaceAll(g.ID, "-", "_")+"_test"))
		}
		f, err := os.OpenFile(filepath.Join(events, g.ID+".jsonl"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			cancel()
			return err
		}
		bounded := &limitedWriter{w: f, left: 32 << 20}
		cmd.Stdout = bounded
		cmd.Stderr = io.Discard
		fmt.Printf("acceptance group=%s runtime=go-race\n", g.ID)
		err = cmd.Run()
		f.Close()
		cancel()
		if err != nil {
			failures = append(failures, g.ID)
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("failed groups: %s", strings.Join(failures, ", "))
	}
	return nil
}

type limitedWriter struct {
	w    io.Writer
	left int
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if len(p) > w.left {
		return 0, fmt.Errorf("bounded test artifact exceeded")
	}
	n, err := w.w.Write(p)
	w.left -= n
	return n, err
}

func WriteJSON(path string, v any) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0600)
}
