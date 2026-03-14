package transport

type CapabilitySnapshot struct {
	SupportsStreaming        bool
	SupportsToolCalls        bool
	SupportsBatchToolCalls   bool
	SupportsStatefulSessions bool
	SupportsCancel           bool
	SupportsUsageMetadata    bool
}

func (c CapabilitySnapshot) SupportsStatefulResume() bool {
	return c.SupportsStatefulSessions
}
