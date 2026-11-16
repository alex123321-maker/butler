package acp

import (
	"context"
	"errors"
	"sync/atomic"
	"time"
)

var (
	ErrBinding                = errors.New("acp probe: stale or invalid owning binding")
	ErrResume                 = errors.New("acp probe: provider session resume is unsupported")
	ErrConsumed               = errors.New("acp probe: instance already consumed")
	ErrBudget                 = errors.New("acp probe: protocol budget exhausted")
	ErrProtocol               = errors.New("acp probe: invalid or interrupted protocol")
	ErrRemote                 = errors.New("acp probe: agent rejected request")
	ErrAuthenticationRequired = errors.New("acp runtime requires a dedicated Google login")
	ErrTools                  = errors.New("acp probe: unmediated agent tools are unsupported")
	ErrModel                  = errors.New("acp probe: exact requested model is unavailable")
	ErrProcess                = errors.New("acp probe: process qualification failed")
	ErrProcessUnsupported     = errors.New("acp probe: process qualification requires Linux")
)

type Binding struct {
	OwnerID           string
	SessionID         string
	TaskID            string
	RunID             string
	TaskVersion       int64
	AuthorityVersion  int64
	ConstraintVersion int64
}

type ProtocolFailure struct{ Stage, Kind string }

func (failure *ProtocolFailure) Error() string {
	return "acp protocol failure: " + failure.Stage + ": " + failure.Kind
}
func (*ProtocolFailure) Unwrap() error { return ErrProtocol }

type Limits struct {
	WallTime    time.Duration
	Messages    int
	WireBytes   int
	TextBytes   int
	PromptBytes int
}

type Request struct {
	Binding         Binding
	ResumeSessionID string
	ModelID         string
	Prompt          string
	BeforePrompt    func(context.Context) error
}

type ProcessSpec struct {
	Executable string
	SHA256     string
	Args       []string
	profileDir string
	proxied    bool
}

type Result struct {
	Text       string
	ModelID    string
	StopReason string
	UsageState string
}

type Probe struct {
	binding Binding
	limits  Limits
	used    atomic.Bool
}

func NewProbe(binding Binding, limits Limits) (*Probe, error) {
	if binding.OwnerID == "" || binding.SessionID == "" || binding.TaskID == "" || binding.RunID == "" || binding.TaskVersion <= 0 || binding.AuthorityVersion <= 0 || binding.ConstraintVersion <= 0 {
		return nil, ErrBinding
	}
	if limits.WallTime <= 0 || limits.WallTime > 300*time.Second || limits.Messages <= 0 || limits.Messages > 512 || limits.WireBytes <= 0 || limits.WireBytes > 4*1024*1024 || limits.TextBytes <= 0 || limits.TextBytes > 64*1024 || limits.PromptBytes < 0 || limits.PromptBytes > 2<<20 {
		return nil, ErrBudget
	}
	return &Probe{binding: binding, limits: limits}, nil
}

type configOption struct {
	ID           string `json:"id"`
	Category     string `json:"category"`
	Type         string `json:"type"`
	CurrentValue string `json:"currentValue"`
	Options      []struct {
		Value string `json:"value"`
	} `json:"options"`
}
