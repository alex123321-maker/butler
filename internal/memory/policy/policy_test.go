package policy

import "testing"

func TestConfirmationState_IsValid(t *testing.T) {
	tests := []struct {
		state ConfirmationState
		want  bool
	}{
		{ConfirmationPending, true},
		{ConfirmationConfirmed, true},
		{ConfirmationRejected, true},
		{ConfirmationAutoConfirmed, true},
		{"unknown", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := tt.state.IsValid(); got != tt.want {
			t.Errorf("ConfirmationState(%q).IsValid() = %v, want %v", tt.state, got, tt.want)
		}
	}
}

func TestConfirmationState_IsTerminal(t *testing.T) {
	tests := []struct {
		state ConfirmationState
		want  bool
	}{
		{ConfirmationPending, false},
		{ConfirmationConfirmed, true},
		{ConfirmationRejected, true},
		{ConfirmationAutoConfirmed, true},
	}
	for _, tt := range tests {
		if got := tt.state.IsTerminal(); got != tt.want {
			t.Errorf("ConfirmationState(%q).IsTerminal() = %v, want %v", tt.state, got, tt.want)
		}
	}
}

func TestEffectiveStatus_IsValid(t *testing.T) {
	tests := []struct {
		status EffectiveStatus
		want   bool
	}{
		{EffectiveActive, true},
		{EffectiveInactive, true},
		{EffectiveSuppressed, true},
		{EffectiveExpired, true},
		{EffectiveDeleted, true},
		{"unknown", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := tt.status.IsValid(); got != tt.want {
			t.Errorf("EffectiveStatus(%q).IsValid() = %v, want %v", tt.status, got, tt.want)
		}
	}
}

func TestEffectiveStatus_IsVisible(t *testing.T) {
	tests := []struct {
		status EffectiveStatus
		want   bool
	}{
		{EffectiveActive, true},
		{EffectiveInactive, false},
		{EffectiveSuppressed, false},
		{EffectiveExpired, false},
		{EffectiveDeleted, false},
	}
	for _, tt := range tests {
		if got := tt.status.IsVisible(); got != tt.want {
			t.Errorf("EffectiveStatus(%q).IsVisible() = %v, want %v", tt.status, got, tt.want)
		}
	}
}

func TestGetCapabilities_Profile(t *testing.T) {
	caps := GetCapabilities(MemoryTypeProfile)
	if !caps.Editable {
		t.Error("Profile should be editable")
	}
	if !caps.Confirmable {
		t.Error("Profile should be confirmable")
	}
	if !caps.Suppressible {
		t.Error("Profile should be suppressible")
	}
	if !caps.Deletable {
		t.Error("Profile should be deletable")
	}
	if caps.HardDeletable {
		t.Error("Profile should not be hard-deletable (audit trail)")
	}
}

func TestGetCapabilities_Episodic(t *testing.T) {
	caps := GetCapabilities(MemoryTypeEpisodic)
	if caps.Editable {
		t.Error("Episodic content should not be editable (provenance)")
	}
	if !caps.Confirmable {
		t.Error("Episodic should be confirmable")
	}
	if !caps.Suppressible {
		t.Error("Episodic should be suppressible")
	}
	if !caps.Deletable {
		t.Error("Episodic should be deletable")
	}
}

func TestGetCapabilities_Chunk(t *testing.T) {
	caps := GetCapabilities(MemoryTypeChunk)
	if caps.Editable {
		t.Error("Chunks should not be editable (system-generated)")
	}
	if caps.Confirmable {
		t.Error("Chunks should not be confirmable (auto-confirmed)")
	}
	if !caps.Suppressible {
		t.Error("Chunks should be suppressible")
	}
	if !caps.HardDeletable {
		t.Error("Chunks should be hard-deletable for space management")
	}
}

func TestGetCapabilities_Working(t *testing.T) {
	caps := GetCapabilities(MemoryTypeWorking)
	if caps.Editable || caps.Confirmable || caps.Suppressible || caps.Deletable {
		t.Error("Working memory should not support UI management operations")
	}
}

func TestCanHelpers(t *testing.T) {
	if !CanEdit(MemoryTypeProfile) {
		t.Error("CanEdit(profile) should be true")
	}
	if CanEdit(MemoryTypeEpisodic) {
		t.Error("CanEdit(episodic) should be false")
	}
	if !CanConfirm(MemoryTypeProfile) {
		t.Error("CanConfirm(profile) should be true")
	}
	if CanConfirm(MemoryTypeChunk) {
		t.Error("CanConfirm(chunk) should be false")
	}
	if !CanSuppress(MemoryTypeChunk) {
		t.Error("CanSuppress(chunk) should be true")
	}
	if CanSuppress(MemoryTypeWorking) {
		t.Error("CanSuppress(working) should be false")
	}
	if !CanDelete(MemoryTypeEpisodic) {
		t.Error("CanDelete(episodic) should be true")
	}
	if !CanHardDelete(MemoryTypeChunk) {
		t.Error("CanHardDelete(chunk) should be true")
	}
	if CanHardDelete(MemoryTypeProfile) {
		t.Error("CanHardDelete(profile) should be false")
	}
}
