package artifacts

import (
	"context"
	"testing"
	"time"
)

type memoryRepo struct {
	items map[string]Record
}

func newMemoryRepo() *memoryRepo {
	return &memoryRepo{items: map[string]Record{}}
}

func (m *memoryRepo) CreateArtifact(_ context.Context, params CreateParams) (Record, error) {
	rec := Record{
		ArtifactID:    params.ArtifactID,
		RunID:         params.RunID,
		SessionKey:    params.SessionKey,
		ArtifactType:  params.ArtifactType,
		Title:         params.Title,
		Summary:       params.Summary,
		ContentText:   params.ContentText,
		ContentJSON:   params.ContentJSON,
		ContentFormat: params.ContentFormat,
		SourceType:    params.SourceType,
		SourceRef:     params.SourceRef,
		CreatedAt:     params.CreatedAt,
		UpdatedAt:     params.CreatedAt,
	}
	m.items[rec.ArtifactID] = rec
	return rec, nil
}

func (m *memoryRepo) GetArtifactByID(_ context.Context, artifactID string) (Record, error) {
	item, ok := m.items[artifactID]
	if !ok {
		return Record{}, ErrArtifactNotFound
	}
	return item, nil
}

func (m *memoryRepo) ListArtifacts(_ context.Context, params ListParams) ([]Record, error) {
	items := make([]Record, 0, len(m.items))
	for _, item := range m.items {
		items = append(items, item)
	}
	return items, nil
}

func (m *memoryRepo) ListArtifactsByRun(_ context.Context, runID string, limit int) ([]Record, error) {
	items := make([]Record, 0)
	for _, item := range m.items {
		if item.RunID == runID {
			items = append(items, item)
		}
	}
	return items, nil
}

func TestArtifactsServiceSavesUsefulResults(t *testing.T) {
	t.Parallel()

	repo := newMemoryRepo()
	svc := NewService(repo)
	now := time.Now().UTC()

	assistant, err := svc.SaveAssistantFinal(context.Background(), "run-1", "telegram:chat:1", "final answer", now)
	if err != nil {
		t.Fatalf("SaveAssistantFinal returned error: %v", err)
	}
	if assistant.ArtifactType != TypeAssistantFinal {
		t.Fatalf("expected assistant_final, got %q", assistant.ArtifactType)
	}

	tool, err := svc.SaveToolResult(context.Background(), "run-1", "telegram:chat:1", "tool-1", "http.request", "completed", `{"status":200}`, now)
	if err != nil {
		t.Fatalf("SaveToolResult returned error: %v", err)
	}
	if tool.ArtifactType != TypeToolResult {
		t.Fatalf("expected tool_result, got %q", tool.ArtifactType)
	}

	doctor, err := svc.SaveDoctorReport(context.Background(), "run-1", "telegram:chat:1", "healthy", `{"ok":true}`, now)
	if err != nil {
		t.Fatalf("SaveDoctorReport returned error: %v", err)
	}
	if doctor.ArtifactType != TypeDoctorReport {
		t.Fatalf("expected doctor_report, got %q", doctor.ArtifactType)
	}

	capture, err := svc.SaveBrowserCapture(context.Background(), "run-1", "telegram:chat:1", "tool-call-1", "single-tab-1", "https://example.com", "Example", "data:image/png;base64,abc", now)
	if err != nil {
		t.Fatalf("SaveBrowserCapture returned error: %v", err)
	}
	if capture.ArtifactType != TypeBrowserCapture {
		t.Fatalf("expected browser_capture, got %q", capture.ArtifactType)
	}

	if len(repo.items) != 4 {
		t.Fatalf("expected 4 artifacts persisted, got %d", len(repo.items))
	}
}
