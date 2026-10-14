package decisions

import (
	"context"
	"encoding/json"
	"math"
	"time"
	"unicode"

	"github.com/butler/butler/internal/budget"
	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"github.com/butler/butler/internal/localmodels"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
)

const InterpretationWindowVersion = "sentence-quote-window-v1"

// Window offsets are Unicode code points in the original, never model-created.
type InterpretationWindow struct {
	Quote string `json:"quote"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

func InterpretationWindows(text string) []InterpretationWindow {
	runes := []rune(text)
	var windows []InterpretationWindow
	start, bracket := 0, 0
	var quote rune
	flush := func(end int) {
		a, b := start, end
		for a < b && unicode.IsSpace(runes[a]) {
			a++
		}
		for b > a && unicode.IsSpace(runes[b-1]) {
			b--
		}
		if b > a {
			windows = append(windows, InterpretationWindow{string(runes[a:b]), a, b})
		}
		start = end
	}
	for i, c := range runes {
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '«':
			quote = '»'
		case '“':
			quote = '”'
		case '"':
			quote = '"'
		case '(', '[':
			bracket++
		case ')', ']':
			if bracket > 0 {
				bracket--
			}
		case '.', '!', '?', ';', '\n':
			if bracket == 0 {
				flush(i + 1)
			}
		}
	}
	flush(len(runes))
	return windows
}

type Interpretation struct {
	Intents           []string           `json:"intents"`
	Modality          string             `json:"modality"`
	Target            string             `json:"target"`
	Escalate          bool               `json:"escalate"`
	ConfidenceUnknown bool               `json:"confidence_unknown"`
	MinimumScore      float64            `json:"minimum_uncalibrated_score"`
	MinimumMargin     float64            `json:"minimum_margin"`
	Scores            map[string]float64 `json:"uncalibrated_scores,omitempty"`
}

func NormalizeInterpretation(out *pb.ClassifyOutput) (Interpretation, error) {
	o := Interpretation{Intents: []string{}, Escalate: true, MinimumScore: 1, MinimumMargin: 1, Scores: map[string]float64{}}
	if out == nil || out.InsufficientData || len(out.Scores) != 16 {
		return o, ErrDenied
	}
	allowed := map[string]bool{}
	for _, id := range localmodels.InterpretationLabels() {
		allowed[id] = true
	}
	for _, s := range out.Scores {
		if s == nil || !allowed[s.Id] || math.IsNaN(s.Value) || math.IsInf(s.Value, 0) || s.Value < 0 || s.Value > 1 {
			return o, ErrDenied
		}
		if _, exists := o.Scores[s.Id]; exists {
			return o, ErrDenied
		}
		o.Scores[s.Id] = s.Value
	}
	_, d := localmodels.InterpretationSchema()
	for _, option := range d.Intents {
		p := o.Scores["intent/"+option.ID]
		if p >= .5 {
			o.Intents = append(o.Intents, option.ID)
		}
		o.MinimumScore = math.Min(o.MinimumScore, math.Max(p, 1-p))
		o.MinimumMargin = math.Min(o.MinimumMargin, math.Abs(2*p-1))
	}
	for _, group := range []struct {
		prefix  string
		options []localmodels.InterpretationOption
		dest    *string
	}{{"modality/", d.Modality, &o.Modality}, {"target/", d.Target, &o.Target}} {
		best, second, total := -1.0, -1.0, 0.0
		for _, option := range group.options {
			p := o.Scores[group.prefix+option.ID]
			total += p
			if p > best {
				second = best
				best = p
				*group.dest = option.ID
			} else if p > second {
				second = p
			}
		}
		if math.Abs(total-1) > 1e-4 {
			return o, ErrDenied
		}
		o.MinimumScore = math.Min(o.MinimumScore, best)
		o.MinimumMargin = math.Min(o.MinimumMargin, best-second)
	}
	o.Escalate = o.MinimumScore < .65 || o.MinimumMargin < .15 || o.Target == "unresolved" || o.Modality != "direct"
	return o, nil
}

func ValidateInterpretationQuote(text string, window InterpretationWindow) error {
	runes := []rune(text)
	if window.Start < 0 || window.End <= window.Start || window.End > len(runes) || string(runes[window.Start:window.End]) != window.Quote {
		return ErrDenied
	}
	return nil
}

func InterpretationProfiles() []Profile {
	profiles := TaxonomyProfiles()
	for i := range profiles {
		profiles[i].ID = []string{"gliner2.5-multi-interpretation-v3-cpu", "gliner2.5-decide-interpretation-v3-cpu", "d1-omni-interpretation-v3-cpu"}[i]
		profiles[i].Preprocessing = localmodels.InterpretationSchemaV3
		profiles[i].FullScores = true
	}
	return profiles
}

// Interpret is an opt-in observation. It uses the current source/job boundary
// without adding a candidate publication, execution or completion interface.
func Interpret(ctx context.Context, c *catalog.Store, store *jobs.Store, b Backend, target budget.LocalTarget, p Profile, j jobs.Job, ref catalog.SourceRef) (Interpretation, *pb.InferResponse, error) {
	if c == nil || store == nil || b == nil || target.Endpoint() != "local-models:9090" || target.Identity() == "" || j.Kind != "extract" || j.Purpose != "decision_shadow" {
		return Interpretation{}, nil, ErrDenied
	}
	pinned := false
	for _, expected := range InterpretationProfiles() {
		if p == expected {
			pinned = true
		}
	}
	if !pinned {
		return Interpretation{}, nil, ErrDenied
	}
	ctx = store.Context(ctx, j)
	boundary := &Observer{catalog: c, jobs: store}
	source, err := boundary.source(ctx, ref, j.Purpose)
	if err != nil {
		return Interpretation{}, nil, ErrDenied
	}
	cat, err := b.Describe(ctx, &pb.Empty{})
	if err != nil || cat == nil {
		return Interpretation{}, nil, ErrDenied
	}
	ready := false
	for _, m := range cat.Models {
		if m != nil && m.Id == p.ID && m.Model == p.Model && m.Revision == p.Revision && m.PreprocessingVersion == p.Preprocessing && m.RuntimeVersion == p.Runtime && m.ContextTokens == 2048 && m.Precision == "FP32" && m.PreferredDevice == "cpu" && m.Enabled {
			ready = true
		}
	}
	if !ready {
		return Interpretation{}, nil, ErrDenied
	}
	timeout := time.Minute
	if deadline, ok := ctx.Deadline(); ok {
		timeout = min(timeout, time.Until(deadline))
	}
	timeout = timeout.Truncate(time.Millisecond)
	if timeout < time.Millisecond {
		return Interpretation{}, nil, ErrDenied
	}
	id, err := budget.NewOperationID()
	if err != nil {
		return Interpretation{}, nil, ErrDenied
	}
	r := &pb.InferRequest{RequestId: id, ProfileId: p.ID, CallerId: "interpretation-shadow", TimeoutMs: uint32(timeout.Milliseconds()), CpuOnly: true, Input: &pb.InferRequest_Classify{Classify: &pb.ClassifyInput{Text: source.Body, Labels: localmodels.InterpretationLabels()}}}
	wire, _ := json.Marshal(r)
	bounded, release, err := store.Admit(ctx, j, jobs.Operation{ID: id, Kind: j.Kind, Endpoint: target.Endpoint(), ServiceIdentity: target.Identity(), InputBytes: int64(len(wire)), Tokens: 7 * 2048, Timeout: timeout, Origin: source.Origin})
	if err != nil {
		return Interpretation{}, nil, ErrDenied
	}
	response, err := b.Infer(bounded, r)
	canceled := bounded.Err()
	release()
	if err != nil || canceled != nil || response == nil || response.RequestId != id || response.ProfileId != p.ID || response.Revision != p.Revision || response.Device != "cpu" || response.FallbackReason != "" {
		return Interpretation{}, response, ErrDenied
	}
	o, err := NormalizeInterpretation(response.GetClassify())
	if err != nil {
		return o, response, err
	}
	if _, err = boundary.source(ctx, ref, j.Purpose); err != nil {
		return Interpretation{}, response, ErrDenied
	}
	return o, response, nil
}

func hasIntent(intents []string, name string) bool {
	for _, s := range intents {
		if s == name {
			return true
		}
	}
	return false
}
func sameIntents(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for _, s := range a {
		if !hasIntent(b, s) {
			return false
		}
	}
	return true
}
func interpretationInput(text, context string, window InterpretationWindow) string {
	b, _ := json.Marshal(struct {
		Context string `json:"context"`
		Text    string `json:"text"`
		Focus   string `json:"focus"`
	}{context, text, window.Quote})
	return string(b)
}

// TrustedSourceAllowsOwnerIntent takes code-owned actor identity, never text.
// It only describes a shadow observation; even true grants no execution right.
func TrustedSourceAllowsOwnerIntent(sourceKind string, o Interpretation) bool {
	return sourceKind == "owner" && o.Modality == "direct" && o.Target != "unresolved" && !o.Escalate && (hasIntent(o.Intents, "request_action") || hasIntent(o.Intents, "change_constraint") || hasIntent(o.Intents, "stop_work"))
}
