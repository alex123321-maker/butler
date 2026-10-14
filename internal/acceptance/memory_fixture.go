package acceptance

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strconv"
	"time"

	"github.com/butler/butler/internal/memory/service"
)

// Fixed store responses exercise the CURRENT bundle builder. They deliberately
// do not implement a new retriever or claim real embedding/retrieval quality.
type memoryFixtureStore struct{ sources []MemorySource }

func (s memoryFixtureStore) Search(_ context.Context, _ []service.Scope, _ string, limit int) ([]service.SemanticHit, error) {
	hits := []service.SemanticHit{}
	for _, h := range s.sources {
		if h.Status == "active" && h.Sensitivity == "public" {
			hits = append(hits, service.SemanticHit{Title: h.ID, Summary: h.Body, SourceRef: h.ID, Revision: strconv.FormatInt(h.Revision, 10)})
			if len(hits) == limit {
				break
			}
		}
	}
	return hits, nil
}
func MemoryFixtureConfigDigest() string {
	return cloudDigest(struct {
		Kind                     string
		ChunkLimit, BundleBudget int
	}{"fixed_store_current_bundle_no_embedding", 3, 12})
}
func MemoryFixtureCaptures(s MemorySuite, p MemoryPlan) ([]MemoryCapture, error) {
	captures := []MemoryCapture{}
	for _, c := range s.Cases {
		if c.Blocked() || !memorySelected(c, p) {
			continue
		}
		builder := service.New(service.Config{Semantic: memoryFixtureStore{c.History}, ChunkLimit: 3, BundleBudget: 12, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
		bundle, err := builder.BuildBundle(context.Background(), service.BundleRequest{SessionKey: "fixture-current", UserID: "fixture-owner", UserMessage: c.Question, IncludeQuery: true})
		if err != nil {
			return nil, err
		}
		ids := []string{}
		if chunks, ok := bundle.Items["chunks"].([]map[string]any); ok {
			for _, h := range chunks {
				ids = append(ids, h["source_ref"].(string))
			}
		}
		captures = append(captures, MemoryCapture{memoryCaseID(c), p.MemoryConfigSHA256, cloudDigest(bundle), ids, bundle.Prompt})
	}
	return captures, nil
}

// Rubric-driven outputs are controlled scorer inputs, not answers from an agent.
// A missing citation becomes abstention to exercise negative baseline reporting.
func MemoryFixtureMeasurements(s MemorySuite, f MemoryFrozen) MemoryMeasurements {
	m := MemoryMeasurements{FrozenSHA256: f.SHA256, Observations: []MemoryObservation{}, Expenses: []MemoryExpense{}}
	cases := map[string]MemoryCase{}
	for _, c := range s.Cases {
		cases[memoryCaseID(c)] = c
	}
	for _, i := range f.Inputs {
		start := time.Now()
		c := cases[i.CaseID]
		success, violation := true, false
		o := MemoryObservation{CaseID: i.CaseID, Arm: i.Arm, Model: f.Plan.Model, ModelVersion: f.Plan.ModelVersion, Runtime: f.Plan.Runtime, InputSHA256: MemoryInputDigest(i), Status: "completed", Output: json.RawMessage("null"), Outcome: MemoryOutcome{EvidenceSHA256: cloudDigest("scripted_outcome:" + i.CaseID + "/" + i.Arm), Success: &success, PermissionViolation: &violation, PrivacyViolation: &violation, StopViolation: &violation, DeletionViolation: &violation}}
		if i.SkipReason != "" {
			o.Status = "skipped"
			o.Reason = i.SkipReason
			o.Outcome.Success = nil
			m.Observations = append(m.Observations, o)
			continue
		}
		answer := c.Expected
		if verifyMemoryObservedAnswer(c, i, answer) != nil {
			answer.Answer = ""
			answer.Abstain = true
			answer.Confidence = 0
			answer.Citations = []CommandWitness{}
			success = false
		}
		o.Output, _ = json.Marshal(answer)
		elapsed := time.Since(start).Milliseconds()
		o.Milliseconds = &elapsed
		m.Observations = append(m.Observations, o)
		for _, stage := range MemoryExpenseStages() {
			zero := int64(0)
			e := MemoryExpense{ID: i.CaseID + "/" + i.Arm + "/" + stage, CaseID: i.CaseID, Arm: i.Arm, Stage: stage, Status: "not_used", Outcome: "not_used", EvidenceSHA256: cloudDigest("fixture_stage:" + stage), InputTokens: &zero, OutputTokens: &zero, TotalTokens: &zero, Milliseconds: &zero, Microunits: &zero, StorageBytes: &zero}
			// Bytes of the synthetic serialized source fixture, not a PG size claim.
			if stage == "read" && i.Arm != "no_ltm" {
				size := int64(len(i.MemoryPrompt))
				for _, h := range i.Memory {
					size += int64(len(h.Body))
				}
				e.Status = "measured"
				e.Outcome = "success"
				e.Calls = 1
				e.StorageBytes = &size
			}
			m.Expenses = append(m.Expenses, e)
		}
	}
	return m
}
