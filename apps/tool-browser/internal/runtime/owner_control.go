package runtime

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/butler/butler/internal/browsercontrol"
	"github.com/butler/butler/internal/security/egress"
)

// Control is reachable only through Broker's separately authenticated private
// owner channel. The same session mutex serializes owner and model execution.
func (s *Server) Control(ctx context.Context, command browsercontrol.Command) (browsercontrol.Reply, error) {
	var reply browsercontrol.Reply
	session, code := s.sessions.acquire(command.SessionID, command.RunID)
	if code != "" {
		return reply, browsercontrol.ErrDenied
	}
	defer session.mu.Unlock()
	if session.closed || !time.Now().Before(session.expires) {
		return reply, browsercontrol.ErrDenied
	}
	if command.Operation == "take" {
		// Equality is allowed only after a local release whose ACK may have
		// been lost. Broker still requires a fresh durable takeover CAS.
		if command.Epoch < session.controlEpoch || command.Epoch == session.controlEpoch && session.controller != "" || len(command.Controller) != 32 {
			return reply, browsercontrol.ErrDenied
		}
		// Fence before helper work; lost acknowledgments never resume the agent.
		session.controlEpoch, session.controller = command.Epoch, command.Controller
	} else if command.Operation != "frame" || command.Controller != "" {
		if session.controller == "" || session.controller != command.Controller || session.controlEpoch != command.Epoch {
			return reply, browsercontrol.ErrDenied
		}
	}
	switch command.Operation {
	case "frame", "take", "release":
	case "click", "double_click", "wheel", "key", "text", "navigate":
		if len(command.FrameID) > 64 || command.FrameID == "" || len(command.Text) > 4096 || len(command.Key) > 64 || math.IsNaN(command.X) || math.IsNaN(command.Y) || command.X < 0 || command.X >= 1280 || command.Y < 0 || command.Y >= 720 || math.Abs(command.DeltaX) > 2000 || math.Abs(command.DeltaY) > 2000 {
			return reply, browsercontrol.ErrDenied
		}
		if command.Button != "" && command.Button != "left" && command.Button != "middle" && command.Button != "right" {
			return reply, browsercontrol.ErrDenied
		}
		if command.Operation == "navigate" {
			u, err := parseWebURL(command.URL)
			if err != nil || !domainAllowed(u.Hostname(), session.allowedDomains) || len(command.URL) > 8192 {
				return reply, browsercontrol.ErrDenied
			}
			if _, err := (egress.Policy{}).ValidateURL(command.URL); err != nil {
				return reply, browsercontrol.ErrDenied
			}
		}
		if strings.ContainsAny(command.Key, "\r\n") {
			return reply, browsercontrol.ErrDenied
		}
	default:
		return reply, browsercontrol.ErrDenied
	}
	runCtx, cancel := context.WithTimeout(ctx, 18*time.Second)
	defer cancel()
	result, err := s.runner.Run(runCtx, Request{ToolName: "owner.control", SessionID: session.id, OwnerCommand: &command})
	if err != nil {
		s.sessions.invalidateAll()
		return reply, browsercontrol.ErrDenied
	}
	if result.Error != nil {
		return reply, fmt.Errorf("%w: %s", browsercontrol.ErrDenied, result.Error.ErrorType)
	}
	if !result.OK {
		return reply, browsercontrol.ErrDenied
	}
	// Invalidate the agent's old CAS after any manual interaction or release.
	if command.Operation != "frame" {
		session.version = 0
	}
	if command.Operation == "release" {
		session.controller = ""
		session.controlEpoch++
	}
	if result.OwnerFrame != nil {
		result.OwnerFrame.Epoch = session.controlEpoch
		result.OwnerFrame.Mode = "agent"
		if session.controller != "" {
			result.OwnerFrame.Mode = "owner"
		}
	}
	return browsercontrol.Reply{OK: true, Frame: result.OwnerFrame}, nil
}
