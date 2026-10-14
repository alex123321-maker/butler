package acceptance

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"time"
)

type MemoryRunnerAudit struct {
	Status                     string   `json:"status"`
	NativeCalls                int64    `json:"native_calls"`
	AuxiliaryCalls             int64    `json:"auxiliary_calls"`
	QuestionResults            int      `json:"question_results"`
	UnknownBroaderTaskResults  int      `json:"unknown_broader_task_results"`
	OriginalLedgerSHA256       string   `json:"original_ledger_sha256"`
	OriginalMeasurementsSHA256 string   `json:"original_measurements_sha256"`
	SourceSHA256               string   `json:"source_sha256"`
	BinarySHA256               string   `json:"binary_sha256"`
	Limits                     []string `json:"limits"`
}

// This read-only correlation must be invoked against an operator-pinned original
// directory, not an arbitrary imported set of model prose. It validates original
// raw finals, per-entry native events, durable admission, actual token dimensions,
// and independently recomputed narrow QA assessments; no inference is possible.
func AuditMemoryRunner(s MemorySuite, f MemoryFrozen, m MemoryMeasurements, directory, binarySHA string) (MemoryRunnerAudit, error) {
	a := MemoryRunnerAudit{Status: "NO_GO", SourceSHA256: f.Plan.SourceSHA256, BinarySHA256: binarySHA, Limits: []string{"Operator-pinned original directory and compiled source identity are required.", "Public synthetic QA and proposed-action observations do not prove external effects or long-lived user task completion.", "Subscription invoice, embedding tokenizer usage and host energy remain unknown; known costs are subtotals.", "This audit cannot reopen a window, execute a tool, tune controls or authorize promotion."}}
	if !validSHA(binarySHA) || f.Plan.EvidenceKind != "model_observation" {
		return a, errors.New("native binary and plan identity required")
	}
	load := func(name string, out any) ([]byte, error) {
		raw, err := ReadPrivateFile(filepath.Join(directory, name), 32<<20)
		if err != nil {
			return nil, err
		}
		return raw, json.Unmarshal(raw, out)
	}
	var ledger map[string]json.RawMessage
	raw, err := load("ledger.json", &ledger)
	if err != nil {
		return a, err
	}
	a.OriginalLedgerSHA256 = hashWindowBytes(raw)
	raw, err = ReadPrivateFile(filepath.Join(directory, "measurements.json"), 8<<20)
	if err != nil {
		return a, err
	}
	a.OriginalMeasurementsSHA256 = hashWindowBytes(raw)
	var originalMeasurements MemoryMeasurements
	if json.Unmarshal(raw, &originalMeasurements) != nil || MemoryDigest(originalMeasurements) != MemoryDigest(m) {
		return a, errors.New("original measurements differ from scored import")
	}
	var originalFreeze MemoryFrozen
	if _, err = load("frozen-before-native.json", &originalFreeze); err != nil || MemoryDigest(originalFreeze) != MemoryDigest(f) {
		return a, errors.New("pre-native freeze differs")
	}
	var windows []struct {
		PlanHash string `json:"plan_hash"`
		Calls    int64  `json:"calls"`
		Reserved int64  `json:"reserved_units"`
		Closed   bool   `json:"closed"`
	}
	if json.Unmarshal(ledger["fixture_memory_eval_window"], &windows) != nil || len(windows) != 1 || !windows[0].Closed || windows[0].PlanHash != MemoryDigest(f.Plan) || windows[0].Calls > MemorySuitePolicy(s).MaxCalls || windows[0].Reserved > MemorySuitePolicy(s).MaxTotalTokens {
		return a, errors.New("immutable window ledger denied")
	}
	var attempts []struct {
		Case      string `json:"case_id"`
		Arm       string `json:"arm"`
		Run       string `json:"run_id"`
		Input     string `json:"input_hash"`
		Output    string `json:"output_hash"`
		Operation string `json:"operation_id"`
		Outcome   string `json:"outcome"`
		In        *int64 `json:"input_tokens"`
		Out       *int64 `json:"output_tokens"`
		Total     *int64 `json:"total_tokens"`
		Version   string `json:"model_version"`
		MS        *int64 `json:"milliseconds"`
	}
	var admissions []struct {
		Operation   string `json:"operation_id"`
		Run         string `json:"run_id"`
		Provider    string `json:"provider"`
		Model       string `json:"model"`
		Reservation string `json:"reservation_id"`
		In          *int64 `json:"actual_input_tokens"`
		Out         *int64 `json:"actual_output_tokens"`
	}
	var reservations []struct {
		ID        string `json:"reservation_id"`
		Owner     string `json:"owner_id"`
		Operation string `json:"operation_id"`
		Request   struct {
			Charge struct {
				Calls      int64 `json:"model_turns"`
				Tokens     int64 `json:"tokens"`
				InputBytes int64 `json:"provider_input_bytes"`
			}
		} `json:"request_json"`
	}
	var expenses []struct {
		Record MemoryExpense `json:"record"`
	}
	var results []struct {
		Case       string           `json:"case_id"`
		Arm        string           `json:"arm"`
		Input      string           `json:"input_hash"`
		Output     string           `json:"output_hash"`
		Scope      string           `json:"task_scope"`
		Revision   int64            `json:"task_revision"`
		Assessment MemoryAssessment `json:"assessment"`
	}
	for _, parse := range []struct {
		name string
		out  any
	}{{"fixture_memory_eval_attempts", &attempts}, {"execution_model_attempts", &admissions}, {"execution_budget_reservations", &reservations}, {"fixture_memory_eval_expenses", &expenses}, {"fixture_memory_eval_results", &results}} {
		if len(ledger[parse.name]) == 0 || json.Unmarshal(ledger[parse.name], parse.out) != nil {
			return a, errors.New("original ledger schema denied")
		}
	}
	if err = memoryAuditProvenance(s, f, ledger, directory); err != nil {
		return a, err
	}
	usedAdmissions, usedReservations, usedResults := map[string]bool{}, map[string]bool{}, map[string]bool{}
	expenseMap := map[string]MemoryExpense{}
	for _, e := range expenses {
		if expenseMap[e.Record.ID].ID != "" {
			return a, errors.New("duplicate expense")
		}
		expenseMap[e.Record.ID] = e.Record
	}
	if len(expenseMap) != len(m.Expenses) {
		return a, errors.New("dropped original expense")
	}
	for _, e := range m.Expenses {
		if MemoryDigest(e) != MemoryDigest(expenseMap[e.ID]) {
			return a, errors.New("expense import differs from original")
		}
		if e.Stage == "embedding" {
			a.AuxiliaryCalls += e.Calls
		}
	}
	attemptMap := map[string]int{}
	for n, x := range attempts {
		id := x.Case + "/" + x.Arm
		if _, ok := attemptMap[id]; ok {
			return a, errors.New("duplicate generation attempt")
		}
		attemptMap[id] = n
	}
	observations := map[string]MemoryObservation{}
	for _, o := range m.Observations {
		id := o.CaseID + "/" + o.Arm
		if _, ok := observations[id]; ok {
			return a, errors.New("duplicate observation")
		}
		observations[id] = o
	}
	cases := map[string]MemoryCase{}
	for _, c := range s.Cases {
		cases[MemoryOpaqueCaseID(c)] = c
	}
	for _, i := range f.Inputs {
		id := i.CaseID + "/" + i.Arm
		o, ok := observations[id]
		if !ok {
			return a, errors.New("comparison incomplete")
		}
		n, hasAttempt := attemptMap[id]
		if i.SkipReason != "" {
			if hasAttempt || o.Status != "skipped" || o.Reason != i.SkipReason {
				return a, errors.New("planned skip was admitted")
			}
			continue
		}
		if !hasAttempt || o.Status != "completed" {
			return a, errors.New("native comparison incomplete; negative prefix retained")
		}
		x := attempts[n]
		a.NativeCalls++
		if x.Outcome != "completed" || x.Input != MemoryInputDigest(i) || x.Version != f.Plan.ModelVersion || x.Output != hashWindowBytes(MemoryObservationOriginal(o)) || MemoryDigest(x.MS) != MemoryDigest(o.Milliseconds) {
			return a, errors.New("generation metadata differs")
		}
		admitted := false
		for _, d := range admissions {
			if d.Operation == x.Operation && d.Run == x.Run && d.Provider == "antigravity" && d.Model == f.Plan.Model && MemoryDigest(d.In) == MemoryDigest(x.In) && MemoryDigest(d.Out) == MemoryDigest(x.Out) {
				for _, r := range reservations {
					if r.ID == d.Reservation && r.Owner == "memory-eval-owner:"+i.CaseID && r.Operation == d.Operation && r.Request.Charge.Calls == 1 {
						admitted = true
						if usedReservations[r.ID] || usedAdmissions[d.Operation] {
							return a, errors.New("reused admission/reservation")
						}
						usedReservations[r.ID] = true
						usedAdmissions[d.Operation] = true
					}
				}
			}
		}
		if !admitted {
			return a, errors.New("original model admission absent")
		}
		e := expenseMap[x.Operation]
		if e.Stage != "generation" || e.Calls != 1 || MemoryDigest(e.InputTokens) != MemoryDigest(x.In) || MemoryDigest(e.OutputTokens) != MemoryDigest(x.Out) || MemoryDigest(e.TotalTokens) != MemoryDigest(x.Total) || e.Microunits != nil {
			return a, errors.New("generation invoice/usage differs")
		}
		name := strings.TrimPrefix(i.CaseID, "memory:") + "-" + i.Arm
		var modelInput struct {
			Input        string
			Instructions string
			Schema       json.RawMessage
		}
		if _, err = load("input-"+name+".json", &modelInput); err != nil {
			return a, err
		}
		projection, _ := MemoryModelProjection(i)
		if modelInput.Input != string(projection) || modelInput.Instructions != i.Instructions || MemoryDigest(modelInput.Schema) != MemoryDigest(MemoryNativeSchema(i)) {
			return a, errors.New("actual model-visible input/schema differs")
		}
		final, err := ReadPrivateFile(filepath.Join(directory, "final-"+name+".txt"), 1<<20)
		if err != nil || hashWindowBytes(final) != x.Output || string(final) != string(MemoryObservationOriginal(o)) {
			return a, errors.New("original final differs")
		}
		var evs []struct {
			ID       string    `json:"event_id"`
			Run      string    `json:"run_id"`
			Provider string    `json:"provider"`
			Time     time.Time `json:"timestamp"`
			Type     string    `json:"type"`
			Version  *string   `json:"version"`
			Usage    *struct {
				InputTokens  int64
				OutputTokens int64
				TotalTokens  int64
			} `json:"usage"`
			Final *struct {
				Content      string
				FinishReason string
				Usage        *struct {
					InputTokens  int64
					OutputTokens int64
					TotalTokens  int64
				}
			} `json:"final"`
			Tool bool `json:"tool_proposed"`
		}
		if _, err = load("events-"+name+".json", &evs); err != nil {
			return a, err
		}
		finals, terminals, used, observedVersion := 0, 0, false, false
		eventIDs := map[string]bool{}
		var previous time.Time
		for _, ev := range evs {
			if ev.ID == "" || eventIDs[ev.ID] || ev.Run != x.Run || ev.Provider != "antigravity" || ev.Time.IsZero() || ev.Time.Before(previous) || terminals > 0 {
				return a, errors.New("event identity/order differs")
			}
			eventIDs[ev.ID] = true
			previous = ev.Time
			if ev.Tool || ev.Type == "transport_error" || ev.Type == "run_cancelled" || ev.Type == "run_timed_out" {
				return a, errors.New("unclean original native lifecycle")
			}
			if ev.Type == "run_completed" {
				if finals != 1 {
					return a, errors.New("terminal before final")
				}
				terminals++
			}
			if ev.Final != nil {
				finals++
				if ev.Final.Content != string(final) || ev.Final.FinishReason != "STOP" {
					return a, errors.New("event final differs")
				}
			}
			u := ev.Usage
			if u == nil && ev.Final != nil {
				u = ev.Final.Usage
			}
			if u != nil && x.In != nil && x.Out != nil && x.Total != nil && u.InputTokens == *x.In && u.OutputTokens == *x.Out && u.TotalTokens == *x.Total {
				used = true
			} else if u != nil {
				return a, errors.New("conflicting original usage")
			}
			if ev.Final != nil && ev.Final.Usage != nil && ev.Usage != nil && MemoryDigest(ev.Final.Usage) != MemoryDigest(ev.Usage) {
				return a, errors.New("conflicting final usage")
			}
			if ev.Version != nil {
				observedVersion = true
			}
			if ev.Version != nil && *ev.Version != x.Version {
				return a, errors.New("event model version differs")
			}
		}
		if finals != 1 || terminals != 1 || !used || !observedVersion {
			return a, errors.New("original usage/lifecycle incomplete")
		}
		var answer MemoryAnswer
		decodeErr := json.Unmarshal(MemoryObservationOriginal(o), &answer)
		assessment := AssessMemoryV2(cases[i.CaseID], i, answer, decodeErr == nil)
		matched := false
		for _, r := range results {
			if r.Case == i.CaseID && r.Arm == i.Arm && r.Input == x.Input && r.Output == x.Output && r.Scope == "synthetic_question_answer_and_current_instruction" && r.Revision == i.Task.Revision && MemoryDigest(r.Assessment) == MemoryDigest(assessment) {
				matched = true
				if usedResults[id] {
					return a, errors.New("duplicate QA result")
				}
				usedResults[id] = true
			}
		}
		if !matched || o.Outcome.Success != nil {
			return a, errors.New("task effect fabricated or QA result differs")
		}
		a.QuestionResults++
		a.UnknownBroaderTaskResults++
	}
	var auxiliary []struct {
		ID          string `json:"request_id"`
		Case        string `json:"case_id"`
		Reservation string `json:"reservation_id"`
		Input       string `json:"request_hash"`
		Output      string `json:"response_hash"`
		Outcome     string `json:"outcome"`
	}
	if json.Unmarshal(ledger["fixture_memory_eval_aux"], &auxiliary) != nil {
		return a, errors.New("auxiliary ledger absent")
	}
	for _, x := range auxiliary {
		if x.Outcome != "success" || expenseMap["aux:"+x.ID].Calls != 1 || expenseMap["aux:"+x.ID].CaseID != x.Case {
			return a, errors.New("auxiliary failure/misattribution")
		}
		matched := false
		for _, r := range reservations {
			if r.ID == x.Reservation && r.Owner == "memory-eval-owner:"+x.Case && r.Request.Charge.Calls == 1 && (strings.HasPrefix(r.Operation, "local:") || strings.HasPrefix(r.Operation, "aux:memory-index:")) {
				if usedReservations[r.ID] {
					return a, errors.New("reused auxiliary reservation")
				}
				usedReservations[r.ID] = true
				matched = true
			}
		}
		if !matched {
			return a, errors.New("auxiliary owning reservation absent")
		}
		var trace struct {
			Request, Response json.RawMessage
			Outcome           string
		}
		if _, err = load("aux-"+x.ID+".json", &trace); err != nil || memoryAuditCompactSHA(trace.Request) != x.Input || memoryAuditCompactSHA(trace.Response) != x.Output || trace.Outcome != x.Outcome {
			return a, errors.New("original auxiliary IPC differs")
		}
		if err = memoryAuditAuxAllowed(s, x.Case, trace.Request, ledger); err != nil {
			return a, err
		}
	}
	if len(usedAdmissions) != len(admissions) || len(usedReservations) != len(reservations) || len(usedResults) != len(results) || int64(len(auxiliary)) != a.AuxiliaryCalls {
		return a, errors.New("extra/dropped owning admission, reservation or result")
	}
	if a.NativeCalls != int64(len(attempts)) || a.NativeCalls+a.AuxiliaryCalls != windows[0].Calls {
		return a, errors.New("unattributed paid call")
	}
	// Exactly the original count of private finals; extra/retried calls cannot hide.
	files, err := filepath.Glob(filepath.Join(directory, "final-*.txt"))
	if err != nil || len(files) != int(a.NativeCalls) {
		return a, errors.New("unexpected original final count")
	}
	a.Status = "PASS_CORRELATED_MODEL_OBSERVATIONS"
	return a, nil
}
