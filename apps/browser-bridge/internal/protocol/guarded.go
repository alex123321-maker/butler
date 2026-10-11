package protocol

import "time"

// Guarded dispatches arrive exclusively from the authenticated runtime wrapper.
// Resource/dispatch metadata is never emitted in ordinary tool observations.
type GuardedParams struct {
	ActionDispatchParams
	BrowserInstance string    `json:"browser_instance_id"`
	AllowedOrigin   string    `json:"allowed_origin"`
	DispatchID      string    `json:"dispatch_id"`
	ResourceID      string    `json:"resource_id"`
	ResourceFence   int64     `json:"resource_fence"`
	Deadline        time.Time `json:"deadline"`
}
type GuardedAck struct {
	BoundTabRef   string `json:"bound_tab_ref"`
	DispatchID    string `json:"dispatch_id"`
	ResourceID    string `json:"resource_id"`
	ResourceFence int64  `json:"resource_fence"`
}
type GuardedResult struct {
	ActionDispatchResult
	DispatchID    string `json:"dispatch_id"`
	ResourceFence int64  `json:"resource_fence"`
}
