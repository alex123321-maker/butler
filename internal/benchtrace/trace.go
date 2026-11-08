// Package benchtrace records the logical model boundary in an enrolled synthetic lab.
package benchtrace

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/butler/butler/internal/transport"
)

var eventID = regexp.MustCompile("^bench-[0-9a-f]{24}-[0-9]{4,5}-[0-9]{2}$")

type key struct{}
type enrollment struct{ dir, run string }

// Enroll is called by the operator before ingress. A model cannot select a path.
func Enroll(root, session, event string) error {
	if !eventID.MatchString(event) || session == "" || !filepath.IsAbs(root) {
		return errors.New("invalid trace enrollment")
	}
	dir := filepath.Join(root, event)
	if err := os.Mkdir(dir, 0700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "session"), []byte(session), 0600)
}

func Bind(ctx context.Context, root, source, session, event, run string) context.Context {
	if root == "" || source != "web" || !eventID.MatchString(event) || run == "" || !filepath.IsAbs(root) {
		return ctx
	}
	dir := filepath.Join(root, event)
	marker, err := os.ReadFile(filepath.Join(dir, "session"))
	if err != nil || string(marker) != session || session == "" {
		return ctx
	}
	return context.WithValue(ctx, key{}, enrollment{dir, run})
}

func Filename(run, operation string) string {
	id := sha256.Sum256([]byte(run + "\x00" + operation))
	return hex.EncodeToString(id[:]) + ".jsonl"
}

type Request struct {
	Command    transport.TransportCommandType `json:"command"`
	Items      []transport.InputItem          `json:"input_items,omitempty"`
	Tools      *transport.ToolSetSnapshot     `json:"tool_set"`
	ToolRef    string                         `json:"tool_call_ref,omitempty"`
	ToolResult string                         `json:"tool_result_json,omitempty"`
	Streaming  bool                           `json:"streaming,omitempty"`
	Stateful   bool                           `json:"provider_session_continuation,omitempty"`
}

// Project only logical model-visible fields. Provider session handles, opaque
// transport options, raw event payloads and adapter credentials stay private.
func request(v any) Request {
	switch r := v.(type) {
	case *transport.StartRunRequest:
		return Request{Command: transport.CommandTypeStartRun, Items: r.InputItems, Tools: transport.NewToolSetSnapshot(r.ToolDefinitions), Streaming: r.StreamingEnabled, Stateful: r.Context.ProviderSessionRef != nil}
	case *transport.ContinueRunRequest:
		return Request{Command: transport.CommandTypeContinueRun, Items: r.InputItems, Tools: r.ToolSet, Stateful: r.ProviderSessionRef != nil}
	case *transport.SubmitToolResultRequest:
		return Request{Command: transport.CommandTypeSubmitToolResult, Tools: r.ToolSet, ToolRef: r.ToolCallRef, ToolResult: r.ToolResultJSON, Stateful: r.ProviderSessionRef != nil}
	}
	return Request{}
}

type Record struct {
	Kind            string                     `json:"kind"`
	At              time.Time                  `json:"at"`
	ElapsedUS       int64                      `json:"elapsed_us"`
	Run             string                     `json:"run,omitempty"`
	Operation       string                     `json:"operation,omitempty"`
	Provider        string                     `json:"provider,omitempty"`
	Model           string                     `json:"model,omitempty"`
	Request         *Request                   `json:"request,omitempty"`
	Stage           transport.ProviderStage    `json:"stage,omitempty"`
	Event           transport.EventType        `json:"event,omitempty"`
	Delta           *transport.AssistantDelta  `json:"delta,omitempty"`
	Final           *transport.AssistantFinal  `json:"final,omitempty"`
	Tool            *transport.ToolCallRequest `json:"tool_call,omitempty"`
	Batch           *transport.ToolCallBatch   `json:"tool_batch,omitempty"`
	Usage           *transport.Usage           `json:"usage,omitempty"`
	ReasoningEffort string                     `json:"reasoning_effort,omitempty"`
	ServiceTier     string                     `json:"service_tier,omitempty"`
	CacheRetention  string                     `json:"prompt_cache_retention,omitempty"`
	ModelVersion    *string                    `json:"model_version,omitempty"`
	ErrorCode       string                     `json:"error_code,omitempty"`
	FirstOutputUS   *int64                     `json:"first_output_us,omitempty"`
	Outcome         string                     `json:"outcome,omitempty"`
}

type Writer struct {
	mu    sync.Mutex
	f     *os.File
	start time.Time
	first *int64
	err   error
}

func Begin(ctx context.Context, run, op, provider, model string, command any) (*Writer, error) {
	e, ok := ctx.Value(key{}).(enrollment)
	if !ok || e.run != run {
		return nil, nil
	}
	f, err := os.OpenFile(filepath.Join(e.dir, Filename(run, op)), os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0600)
	if err != nil {
		return nil, err
	}
	w := &Writer{f: f, start: time.Now()}
	r := request(command)
	w.append(Record{Kind: "request", Run: run, Operation: op, Provider: provider, Model: model, Request: &r}, w.start)
	if w.err != nil {
		f.Close()
		return nil, w.err
	}
	return w, nil
}

func (w *Writer) append(r Record, at time.Time) {
	if w.f == nil || w.err != nil {
		return
	}
	r.At = at.UTC()
	r.ElapsedUS = at.Sub(w.start).Microseconds()
	w.err = json.NewEncoder(w.f).Encode(r)
}

func (w *Writer) ObserveStages(ctx context.Context) context.Context {
	if w == nil {
		return ctx
	}
	return transport.WithProviderStageObserver(ctx, func(stage transport.ProviderStage) {
		transport.ObserveProviderStage(ctx, stage)
		w.mu.Lock()
		defer w.mu.Unlock()
		w.append(Record{Kind: "stage", Stage: stage}, time.Now())
	})
}

func (w *Writer) Event(e transport.TransportEvent) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	now := time.Now()
	r := Record{Kind: "event", Event: e.EventType, Final: e.AssistantFinal, Tool: e.ToolCall, Batch: e.ToolCallBatch, Usage: e.Usage, ModelVersion: e.ProviderModelVersion, ReasoningEffort: e.ProviderReasoningEffort, ServiceTier: e.ProviderServiceTier, CacheRetention: e.ProviderCacheRetention}
	if e.AssistantDelta != nil && e.AssistantDelta.DeltaType == "text" {
		r.Delta = e.AssistantDelta
	}
	if e.TransportError != nil {
		r.ErrorCode = transport.DiagnosticCode(e.TransportError)
	}
	if w.first == nil && (r.Delta != nil && r.Delta.Content != "" || r.Final != nil && r.Final.Content != "" || r.Tool != nil || r.Batch != nil) {
		d := now.Sub(w.start).Microseconds()
		w.first = &d
	}
	w.append(r, now)
}

func (w *Writer) End(outcome string) error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil {
		return w.err
	}
	w.append(Record{Kind: "end", Outcome: outcome, FirstOutputUS: w.first}, time.Now())
	w.err = errors.Join(w.err, w.f.Sync(), w.f.Close())
	w.f = nil
	return w.err
}

// Inspect checks the stream without loading a whole artifact or imposing a
// scanner line-size limit on its (potentially large) model context. The caller
// can still archive partial bytes for diagnosis.
func Inspect(src io.Reader, run, op, provider, model string) (Record, error) {
	reader := bufio.NewReader(src)
	var end Record
	lineNum := 0
	var last int64
	var version *string
	var effort string
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			var r Record
			if err != nil || json.Unmarshal(line, &r) != nil || r.At.IsZero() || r.ElapsedUS < last || end.Kind != "" {
				return end, errors.New("invalid or truncated model trace")
			}
			if lineNum == 0 {
				if r.Kind != "request" || r.Request == nil || r.Run != run || r.Operation != op || r.Model != model || r.Provider != provider || r.Request.Command == "" {
					return end, errors.New("model request identity mismatch")
				}
			} else {
				switch r.Kind {
				case "stage", "event":
					if r.ModelVersion != nil && r.ReasoningEffort != "" {
						if version != nil && (*version != *r.ModelVersion || effort != r.ReasoningEffort) {
							return end, errors.New("conflicting native response identity")
						}
						version, effort = r.ModelVersion, r.ReasoningEffort
					}
				case "end":
					if r.Outcome == "" {
						return end, errors.New("missing model outcome")
					}
					end = r
				default:
					return end, fmt.Errorf("unexpected trace record %q", r.Kind)
				}
			}
			lineNum++
			last = r.ElapsedUS
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return end, err
		}
	}
	if end.Kind == "" {
		return end, errors.New("model trace incomplete")
	}
	end.ModelVersion, end.ReasoningEffort = version, effort
	return end, nil
}
