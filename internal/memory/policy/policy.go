// Package policy defines memory management rules for the task-centric UI.
// It implements C-01: Memory policy model (editable/confirmable/suppressible).
package policy

// ConfirmationState represents the confirmation status of a memory entry.
type ConfirmationState string

const (
	// ConfirmationPending means the entry awaits user confirmation.
	ConfirmationPending ConfirmationState = "pending"
	// ConfirmationConfirmed means the user explicitly confirmed the entry.
	ConfirmationConfirmed ConfirmationState = "confirmed"
	// ConfirmationRejected means the user rejected the entry.
	ConfirmationRejected ConfirmationState = "rejected"
	// ConfirmationAutoConfirmed means the entry was auto-confirmed by the system.
	ConfirmationAutoConfirmed ConfirmationState = "auto_confirmed"
)

// IsValid returns true if the confirmation state is recognized.
func (c ConfirmationState) IsValid() bool {
	switch c {
	case ConfirmationPending, ConfirmationConfirmed, ConfirmationRejected, ConfirmationAutoConfirmed:
		return true
	}
	return false
}

// IsTerminal returns true if the confirmation state is final (confirmed or rejected).
func (c ConfirmationState) IsTerminal() bool {
	return c == ConfirmationConfirmed || c == ConfirmationRejected || c == ConfirmationAutoConfirmed
}

// EffectiveStatus represents the effective visibility/lifecycle status of a memory entry.
type EffectiveStatus string

const (
	// EffectiveActive means the entry is active and visible in retrieval.
	EffectiveActive EffectiveStatus = "active"
	// EffectiveInactive means the entry is inactive (e.g., superseded profile entry).
	EffectiveInactive EffectiveStatus = "inactive"
	// EffectiveSuppressed means the entry was soft-suppressed by user action.
	EffectiveSuppressed EffectiveStatus = "suppressed"
	// EffectiveExpired means the entry exceeded its expires_at time.
	EffectiveExpired EffectiveStatus = "expired"
	// EffectiveDeleted means the entry was soft-deleted (kept for audit).
	EffectiveDeleted EffectiveStatus = "deleted"
)

// IsValid returns true if the effective status is recognized.
func (e EffectiveStatus) IsValid() bool {
	switch e {
	case EffectiveActive, EffectiveInactive, EffectiveSuppressed, EffectiveExpired, EffectiveDeleted:
		return true
	}
	return false
}

// IsVisible returns true if the entry should appear in normal retrieval.
func (e EffectiveStatus) IsVisible() bool {
	return e == EffectiveActive
}

// ActorType represents who performed an action on a memory entry.
type ActorType string

const (
	ActorUser     ActorType = "user"
	ActorSystem   ActorType = "system"
	ActorPipeline ActorType = "pipeline"
)

// MemoryType represents the class of memory being managed.
type MemoryType string

const (
	MemoryTypeProfile  MemoryType = "profile"
	MemoryTypeEpisodic MemoryType = "episodic"
	MemoryTypeChunk    MemoryType = "chunk"
	MemoryTypeWorking  MemoryType = "working"
)

// Capabilities defines what management operations are allowed for a memory type.
type Capabilities struct {
	// Editable means the content can be modified by the user.
	Editable bool
	// Confirmable means the entry can require user confirmation.
	Confirmable bool
	// Suppressible means the entry can be soft-suppressed.
	Suppressible bool
	// Deletable means the entry can be soft-deleted.
	Deletable bool
	// HardDeletable means the entry can be permanently removed.
	HardDeletable bool
	// Expirable means the entry can have an expiration time.
	Expirable bool
}

// GetCapabilities returns the management capabilities for a memory type.
func GetCapabilities(memType MemoryType) Capabilities {
	switch memType {
	case MemoryTypeProfile:
		return Capabilities{
			Editable:      true,  // Profile entries can be edited
			Confirmable:   true,  // Profile can require confirmation
			Suppressible:  true,  // Profile can be suppressed
			Deletable:     true,  // Profile can be soft-deleted
			HardDeletable: false, // Keep for audit trail
			Expirable:     true,  // Profile can have TTL
		}
	case MemoryTypeEpisodic:
		return Capabilities{
			Editable:      false, // Episode content is immutable (provenance)
			Confirmable:   true,  // Episodes can require confirmation
			Suppressible:  true,  // Episodes can be suppressed
			Deletable:     true,  // Episodes can be soft-deleted
			HardDeletable: false, // Keep for audit trail
			Expirable:     true,  // Episodes can expire
		}
	case MemoryTypeChunk:
		return Capabilities{
			Editable:      false, // Chunks are system-generated
			Confirmable:   false, // Chunks are auto-confirmed
			Suppressible:  true,  // Chunks can be suppressed
			Deletable:     true,  // Chunks can be soft-deleted
			HardDeletable: true,  // Chunks can be hard-deleted (space)
			Expirable:     true,  // Chunks can expire
		}
	case MemoryTypeWorking:
		return Capabilities{
			Editable:      false, // Working memory is internal state
			Confirmable:   false, // Working memory is ephemeral
			Suppressible:  false, // Working memory is not suppressible
			Deletable:     false, // Cleared by session lifecycle
			HardDeletable: false, // Managed by session lifecycle
			Expirable:     false, // Managed by session lifecycle
		}
	default:
		return Capabilities{}
	}
}

// CanConfirm returns true if the memory type supports confirmation actions.
func CanConfirm(memType MemoryType) bool {
	return GetCapabilities(memType).Confirmable
}

// CanEdit returns true if the memory type supports content editing.
func CanEdit(memType MemoryType) bool {
	return GetCapabilities(memType).Editable
}

// CanSuppress returns true if the memory type supports suppression.
func CanSuppress(memType MemoryType) bool {
	return GetCapabilities(memType).Suppressible
}

// CanDelete returns true if the memory type supports soft deletion.
func CanDelete(memType MemoryType) bool {
	return GetCapabilities(memType).Deletable
}

// CanHardDelete returns true if the memory type supports permanent removal.
func CanHardDelete(memType MemoryType) bool {
	return GetCapabilities(memType).HardDeletable
}

// CanExpire returns true if the memory type supports expiration.
func CanExpire(memType MemoryType) bool {
	return GetCapabilities(memType).Expirable
}
