package transport

import "time"

type AgentExecutionLimits struct {
	InputBytes  int64
	OutputBytes int64
	WallTime    time.Duration
}

func (limits AgentExecutionLimits) Valid() bool {
	return limits.InputBytes > 0 && limits.InputBytes <= 8<<20 && limits.OutputBytes > 0 && limits.OutputBytes <= 1<<20 && limits.WallTime > 0 && limits.WallTime <= 5*time.Minute
}
