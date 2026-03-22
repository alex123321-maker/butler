package protocol

type Request struct {
	ID     string `json:"id"`
	Method string `json:"method"`
	Params any    `json:"params,omitempty"`
}

type Response struct {
	ID     string        `json:"id,omitempty"`
	OK     bool          `json:"ok"`
	Result any           `json:"result,omitempty"`
	Error  *ErrorPayload `json:"error,omitempty"`
}

type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type BindRequestParams struct {
	RunID         string             `json:"run_id"`
	SessionKey    string             `json:"session_key"`
	ToolCallID    string             `json:"tool_call_id,omitempty"`
	RequestedVia  string             `json:"requested_via,omitempty"`
	BrowserHint   string             `json:"browser_hint,omitempty"`
	RequestSource string             `json:"request_source,omitempty"`
	TabCandidates []BindTabCandidate `json:"tab_candidates"`
}

type BindTabCandidate struct {
	InternalTabRef string `json:"internal_tab_ref"`
	Title          string `json:"title"`
	Domain         string `json:"domain"`
	CurrentURL     string `json:"current_url"`
	FaviconURL     string `json:"favicon_url,omitempty"`
	DisplayLabel   string `json:"display_label,omitempty"`
}

type SessionGetActiveParams struct {
	SessionKey string `json:"session_key"`
}

type ActionDispatchParams struct {
	SingleTabSessionID string `json:"single_tab_session_id"`
	BoundTabRef        string `json:"bound_tab_ref"`
	ActionType         string `json:"action_type"`
	ArgsJSON           string `json:"args_json,omitempty"`
}

type ActionDispatchResult struct {
	SingleTabSessionID string `json:"single_tab_session_id"`
	SessionStatus      string `json:"session_status,omitempty"`
	ResultJSON         string `json:"result_json,omitempty"`
	CurrentURL         string `json:"current_url,omitempty"`
	CurrentTitle       string `json:"current_title,omitempty"`
}
