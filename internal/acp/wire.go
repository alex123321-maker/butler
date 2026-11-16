package acp

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"strings"

	"github.com/butler/butler/internal/jsonstrict"
)

type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	Result  json.RawMessage `json:"result"`
	Error   json.RawMessage `json:"error"`
}

type exchange struct {
	conn          net.Conn
	reader        *bufio.Reader
	limits        Limits
	messages      int
	bytes         int
	nextID        int
	sessionID     string
	prompting     bool
	text          strings.Builder
	modelID       string
	modelOptionID string
}

func (wire *exchange) send(payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return ErrProtocol
	}
	raw = append(raw, '\n')
	if err := wire.charge(len(raw)); err != nil {
		return err
	}
	for len(raw) > 0 {
		written, err := wire.conn.Write(raw)
		if err != nil || written == 0 {
			return ErrProtocol
		}
		raw = raw[written:]
	}
	return nil
}

func (wire *exchange) charge(size int) error {
	wire.messages++
	wire.bytes += size
	if wire.messages > wire.limits.Messages || wire.bytes > wire.limits.WireBytes {
		return ErrBudget
	}
	return nil
}

func (wire *exchange) call(method string, params any, result any) error {
	wire.nextID++
	if err := wire.send(map[string]any{"jsonrpc": "2.0", "id": wire.nextID, "method": method, "params": params}); err != nil {
		return err
	}
	for {
		raw, err := wire.reader.ReadSlice('\n')
		if errors.Is(err, bufio.ErrBufferFull) {
			return ErrBudget
		}
		if err != nil {
			return ErrProtocol
		}
		if err := wire.charge(len(raw)); err != nil {
			return err
		}
		var fields map[string]json.RawMessage
		if jsonstrict.Decode(raw, &fields) != nil || fields == nil {
			return ErrProtocol
		}
		for key := range fields {
			switch key {
			case "jsonrpc", "id", "method", "params", "result", "error", "_meta":
			default:
				return ErrProtocol
			}
		}
		if meta, exists := fields["_meta"]; exists {
			var object map[string]json.RawMessage
			if json.Unmarshal(meta, &object) != nil || object == nil {
				return ErrProtocol
			}
		}
		var incoming message
		if json.Unmarshal(raw, &incoming) != nil || incoming.JSONRPC != "2.0" {
			return ErrProtocol
		}
		if incoming.Method != "" {
			if incoming.Result != nil || incoming.Error != nil {
				return ErrProtocol
			}
			if incoming.ID != nil {
				var requestID any
				if json.Unmarshal(incoming.ID, &requestID) != nil {
					return ErrProtocol
				}
				switch requestID.(type) {
				case string, float64:
				default:
					return ErrProtocol
				}
				if incoming.Method == "session/request_permission" {
					if err := wire.send(map[string]any{"jsonrpc": "2.0", "id": incoming.ID, "result": map[string]any{"outcome": map[string]string{"outcome": "cancelled"}}}); err != nil {
						return err
					}
				} else {
					if err := wire.send(map[string]any{"jsonrpc": "2.0", "id": incoming.ID, "error": map[string]any{"code": -32601, "message": "Client method unavailable"}}); err != nil {
						return err
					}
				}
				return ErrTools
			}
			if incoming.Method != "session/update" {
				return ErrProtocol
			}
			if err := wire.update(incoming.Params); err != nil {
				return err
			}
			continue
		}
		var id int
		if fields["method"] != nil || incoming.Params != nil || json.Unmarshal(incoming.ID, &id) != nil || id != wire.nextID || (incoming.Result == nil) == (incoming.Error == nil) {
			return ErrProtocol
		}
		if incoming.Error != nil {
			var remote struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			}
			if json.Unmarshal(incoming.Error, &remote) == nil && remote.Code == -32000 && strings.EqualFold(strings.TrimSpace(remote.Message), "Authentication required") {
				return ErrAuthenticationRequired
			}
			return ErrRemote
		}
		if string(incoming.Result) == "null" || json.Unmarshal(incoming.Result, result) != nil {
			return ErrProtocol
		}
		return nil
	}
}

func (wire *exchange) update(params json.RawMessage) error {
	var update struct {
		SessionID string `json:"sessionId"`
		Update    struct {
			Kind          string          `json:"sessionUpdate"`
			Content       json.RawMessage `json:"content"`
			ConfigOptions []configOption  `json:"configOptions"`
			CurrentModeID string          `json:"currentModeId"`
		} `json:"update"`
	}
	if json.Unmarshal(params, &update) != nil || wire.sessionID == "" || update.SessionID != wire.sessionID {
		return ErrProtocol
	}
	switch update.Update.Kind {
	case "user_message_chunk":
		var content struct {
			Type string `json:"type"`
		}
		if !wire.prompting || json.Unmarshal(update.Update.Content, &content) != nil || content.Type != "text" {
			return ErrProtocol
		}
	case "config_option_update":
		if len(update.Update.ConfigOptions) == 0 {
			return ErrProtocol
		}
		for _, option := range update.Update.ConfigOptions {
			if (option.Category == "model" || option.ID == wire.modelOptionID) && (option.ID != wire.modelOptionID || option.CurrentValue != wire.modelID) {
				return ErrModel
			}
		}
	case "current_mode_update":
		if update.Update.CurrentModeID != "default" && update.Update.CurrentModeID != "plan" {
			return ErrTools
		}
	case "agent_message_chunk":
		var content struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if !wire.prompting || json.Unmarshal(update.Update.Content, &content) != nil || content.Type != "text" {
			return ErrProtocol
		}
		if wire.text.Len()+len(content.Text) > wire.limits.TextBytes {
			return ErrBudget
		}
		wire.text.WriteString(content.Text)
	case "agent_thought_chunk", "usage_update", "plan", "available_commands_update", "session_info_update":
	case "tool_call", "tool_call_update":
		return ErrTools
	default:
		return &ProtocolFailure{Stage: "session_update", Kind: "unrecognized_update"}
	}
	return nil
}
