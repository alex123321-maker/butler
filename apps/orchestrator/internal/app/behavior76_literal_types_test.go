package app

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/internal/ingress"
)

const behavior76LiteralVersion = "behavior76-literal-supplement-v11"
const behavior76LiteralIndexVersion = "behavior76-checker-index-v6-literal"

// Independent literal control hashes remain unbound. No new native entry is
// possible merely because this public interface compiles.
const behavior76LiteralStimuliSHA = "9bc4872c85fb6b2d5e1a1b84b94b24a6a1e6d64ae749d527cbf5698431ca5d0c"
const behavior76LiteralExpectationsSHA = "f20937430079aa6c259a6917f9f1ab846875b4baff71c58cf4c72e4d1ff49865"

// This shape check is shared by the public interface and future admission.
// It is not freshness proof or permission to run a model.
func behavior76LiteralBindingsValid(stimuli, expectations string, requireBound bool) bool {
	if stimuli == "" && expectations == "" {
		return !requireBound
	}
	for _, value := range []string{stimuli, expectations} {
		decoded, err := hex.DecodeString(value)
		if err != nil || len(decoded) != 32 || hex.EncodeToString(decoded) != value {
			return false
		}
	}
	return stimuli != expectations
}

type behavior76LiteralDeadline struct {
	ResourceKey string    `json:"resource_key"`
	DeadlineAt  time.Time `json:"deadline_at"`
}

type behavior76LiteralKernelResponse struct {
	Origin               string `json:"origin"`
	ReturnedResultSHA256 string `json:"returned_result_sha256"`
	ReturnedResultJSON   string `json:"returned_result_json"`
	Kind                 string `json:"kind"`
	InputID              string `json:"input_id"`
	SessionKey           string `json:"session_key"`
	TaskID               string `json:"task_id,omitempty"`
	ReceiptStatus        string `json:"receipt_status"`
	ErrorCode            string `json:"error_code,omitempty"`
	AssistantResponse    string `json:"assistant_response"`
	ControlID            string `json:"control_id,omitempty"`
	ContinuationInputID  string `json:"continuation_input_id,omitempty"`
	OwnerEventRef        string `json:"owner_event_ref"`
	OwnerID              string `json:"owner_id"`
	OwnerEventSessionKey string `json:"owner_event_session_key"`
	OwnerEventSource     string `json:"owner_event_source"`
	OwnerEventSHA256     string `json:"owner_event_sha256"`
	OwnerEventJSON       string `json:"owner_event_json"`
}

type behavior76LiteralAutomaticInput struct {
	InputID   string `json:"input_id"`
	RunID     string `json:"run_id"`
	Status    string `json:"status"`
	ErrorCode string `json:"error_code"`
}

func behavior76LiteralRawOwnerStep(step behavior76Step) bool {
	e := step.Event
	return step.Evaluate && e.Type == "owner_message" && e.Text != "" && e.Intent == "" && e.Kind == "" && e.TaskKey == "" && e.TargetTaskKey == nil && e.Goal == nil && e.Constraints == nil && e.ExecutionPolicy == "" && e.Instruction == "" && e.ResourceKey == "" && e.NewBody == "" && e.Provenance == "" && e.SourceKey == "" && e.Operation == "" && e.Summary == nil
}

func (f *behavior76Fixture) literalKernelReceipt(result *flow.ExecutionResult, event ingress.InputEvent) (*behavior76LiteralKernelResponse, error) {
	if result == nil || !result.Accepted || result.InputID == "" || result.SessionKey == "" || result.RunID != "" || result.AssistantResponse == "" {
		return nil, errors.New("invalid actual kernel response")
	}
	response := &behavior76LiteralKernelResponse{Origin: "input_service_kernel", ReturnedResultSHA256: behavior76Hash([]byte(behavior76JSON(result))), InputID: result.InputID, SessionKey: result.SessionKey, TaskID: result.TaskID, ReceiptStatus: result.ReceiptStatus, ErrorCode: result.ErrorCode, AssistantResponse: result.AssistantResponse, OwnerEventRef: event.EventID, OwnerID: behavior76Owner, OwnerEventSessionKey: event.SessionKey, OwnerEventSource: event.Source, OwnerEventSHA256: behavior76Hash([]byte(behavior76JSON(event)))}
	response.ReturnedResultJSON = behavior76JSON(result)
	response.OwnerEventJSON = behavior76JSON(event)
	var actual bool
	if result.ReceiptStatus == "waiting_for_user" {
		response.Kind = "task_target_clarification"
		err := f.pool.QueryRow(f.ctx, `SELECT EXISTS(SELECT 1 FROM accepted_inputs WHERE input_id=$1 AND session_key=$2 AND source='web' AND dedup_key=$3 AND status='waiting_for_user' AND COALESCE(run_id,'')='') AND NOT EXISTS(SELECT 1 FROM task_control_events WHERE owner_id=$4 AND source='web' AND delivery_id=$3)`, result.InputID, result.SessionKey, response.OwnerEventRef, behavior76Owner).Scan(&actual)
		if err != nil || !actual {
			return nil, errors.New("kernel clarification lacks actual waiting input proof")
		}
	} else if result.ReceiptStatus == "completed" {
		response.Kind = "task_control"
		err := f.pool.QueryRow(f.ctx, `SELECT c.control_id,COALESCE(c.result_json->>'continuation_input_id',''),EXISTS(SELECT 1 FROM accepted_inputs i WHERE i.input_id=$1 AND i.session_key=$2 AND i.status='completed' AND COALESCE(i.run_id,'')='') FROM task_control_events c WHERE c.owner_id=$3 AND c.source='web' AND c.delivery_id=$4 AND c.task_id=$5 AND c.result_json->>'input_id'=$1`, result.InputID, result.SessionKey, behavior76Owner, response.OwnerEventRef, result.TaskID).Scan(&response.ControlID, &response.ContinuationInputID, &actual)
		if err != nil || !actual || response.ControlID == "" {
			return nil, errors.New("kernel control lacks actual durable control/input proof")
		}
	} else {
		return nil, errors.New("unrecognized kernel response status")
	}
	return response, nil
}

type behavior76LiteralDeadlineChange struct {
	BeforeStepID string    `json:"before_step_id"`
	ResourceKey  string    `json:"resource_key"`
	DeadlineAt   time.Time `json:"deadline_at"`
	Provenance   string    `json:"provenance"`
}

// The outer envelope supplies declared experimental inputs, not judgments.
// Business dates belong to the synthetic service observation contract; they do
// not expand the task/mandate or alter any model/run/arm/window authority bound.
// All actual owner exchanges are evaluated through normal authenticated input.
type behavior76LiteralEpisode struct {
	Version             string                            `json:"version"`
	Dimension           string                            `json:"dimension"`
	Polarity            string                            `json:"polarity"`
	Episode             behavior76Episode                 `json:"episode"`
	Deadlines           []behavior76LiteralDeadline       `json:"business_deadlines"`
	DeadlineChanges     []behavior76LiteralDeadlineChange `json:"business_deadline_changes"`
	OwnerTaskSelections []behavior76LiteralTaskSelection  `json:"owner_task_selections"`
}

type behavior76LiteralTaskSelection struct {
	StepID     string `json:"step_id"`
	TaskKey    string `json:"task_key"`
	Provenance string `json:"provenance"`
}

func behavior76LiteralFamily(dimension string) string {
	switch dimension {
	case "ambiguous_object":
		return "clarification"
	case "accepted_preference_conflict":
		return "preference_lifecycle"
	case "repeated_feedback":
		return "feedback_candidate"
	case "status_initiative":
		return "status_continuation"
	case "format_amendment", "deadline_amendment", "recipient_amendment":
		return "amendment"
	case "scoped_action_stop":
		return "scoped_stop_negation"
	}
	return ""
}

func behavior76ReadLiteralEpisodes(reader io.Reader) ([]behavior76LiteralEpisode, error) {
	scanner := bufio.NewScanner(io.LimitReader(reader, (4<<20)+1))
	scanner.Buffer(make([]byte, 4096), 256<<10)
	var episodes []behavior76LiteralEpisode
	seen, pairs := map[string]bool{}, map[string]bool{}
	opportunities, bytes := 0, 0
	for scanner.Scan() {
		bytes += len(scanner.Bytes()) + 1
		if bytes > 4<<20 {
			return nil, errors.New("literal input file bound exceeded")
		}
		if strings.TrimSpace(scanner.Text()) == "" {
			continue
		}
		if err := behavior76LiteralCanonicalKeys(scanner.Text()); err != nil {
			return nil, err
		}
		var value behavior76LiteralEpisode
		decoder := json.NewDecoder(strings.NewReader(scanner.Text()))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&value) != nil || decoder.Decode(new(any)) != io.EOF || value.Version != behavior76LiteralVersion || behavior76LiteralFamily(value.Dimension) == "" || value.Episode.Family != behavior76LiteralFamily(value.Dimension) || (value.Polarity != "positive" && value.Polarity != "negative") || seen[value.Episode.ID] || pairs[value.Dimension+":"+value.Polarity] {
			return nil, errors.New("invalid literal envelope or declared pair")
		}
		// Reuse the original strict task/source/resource/event interface. New
		// cardinality does not waive any old per-episode validation.
		base, err := json.Marshal(value.Episode)
		if err != nil {
			return nil, errors.New("invalid literal episode")
		}
		validated, err := behavior76ReadEpisodes(strings.NewReader(string(base)))
		if err != nil || len(validated) != 1 {
			return nil, errors.New("literal episode violates original interface")
		}
		resources, steps := map[string]bool{}, map[string]bool{}
		for _, resource := range value.Episode.Initial.Resources {
			resources[resource.Key] = true
		}
		graded := 0
		selections := map[string]behavior76LiteralTaskSelection{}
		for _, selection := range value.OwnerTaskSelections {
			if selection.StepID == "" || selection.TaskKey == "" || selection.Provenance != "explicit_owner_ui" || selections[selection.StepID].StepID != "" {
				return nil, errors.New("invalid declared owner UI selection")
			}
			selections[selection.StepID] = selection
		}
		for _, step := range value.Episode.Steps {
			steps[step.ID] = true
			if step.Event.Type == "owner_control" {
				return nil, errors.New("literal input cannot pre-apply a typed owner control")
			}
			if step.Event.Type == "owner_message" && !behavior76LiteralRawOwnerStep(step) {
				return nil, errors.New("literal owner exchange must be real raw input without a resolved intent")
			}
			if step.Evaluate {
				if step.Event.Type != "owner_message" {
					return nil, errors.New("literal graded opportunity must be actual owner input")
				}
				graded++
			}
			selection, selected := selections[step.ID]
			if (step.RunTaskKey != nil && (!step.Evaluate || !selected || selection.TaskKey != *step.RunTaskKey)) || (step.RunTaskKey == nil && selected) {
				return nil, errors.New("task selection lacks matching explicit owner UI provenance")
			}
		}
		for id := range selections {
			if !steps[id] {
				return nil, errors.New("owner UI selection references absent step")
			}
		}
		wanted := 2
		if value.Dimension == "repeated_feedback" {
			wanted = 3
		}
		if graded != wanted {
			return nil, errors.New("literal graded cardinality differs from fixed plan")
		}
		deadlines := map[string]bool{}
		for _, deadline := range value.Deadlines {
			if !resources[deadline.ResourceKey] || deadlines[deadline.ResourceKey] || deadline.DeadlineAt.IsZero() || deadline.DeadlineAt.Location() != time.UTC {
				return nil, errors.New("invalid declared service business deadline")
			}
			deadlines[deadline.ResourceKey] = true
		}
		changes := map[string]bool{}
		for _, change := range value.DeadlineChanges {
			key := change.BeforeStepID + ":" + change.ResourceKey
			if !steps[change.BeforeStepID] || !deadlines[change.ResourceKey] || changes[key] || change.Provenance != "synthetic_service_configuration" || change.DeadlineAt.IsZero() || change.DeadlineAt.Location() != time.UTC {
				return nil, errors.New("invalid service deadline change provenance")
			}
			changes[key] = true
		}
		if value.Dimension == "deadline_amendment" && (len(value.Deadlines) == 0 || len(value.DeadlineChanges) == 0) {
			return nil, errors.New("deadline dimension requires actual declared observation state")
		}
		seen[value.Episode.ID], pairs[value.Dimension+":"+value.Polarity] = true, true
		opportunities += 2 * graded
		episodes = append(episodes, value)
	}
	if scanner.Err() != nil {
		return nil, scanner.Err()
	}
	if len(episodes) != 16 || len(pairs) != 16 || opportunities != 68 {
		return nil, errors.New("literal full16-episode32-arm68-opportunity plan required")
	}
	return episodes, nil
}
