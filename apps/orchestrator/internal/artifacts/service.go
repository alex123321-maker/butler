package artifacts

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) SaveAssistantFinal(ctx context.Context, runID, sessionKey, content string, createdAt time.Time) (Record, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return Record{}, nil
	}
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	title := "Assistant final response"
	summary := summarize(trimmed, 180)
	return s.repo.CreateArtifact(ctx, CreateParams{
		ArtifactID:    artifactID("assistant", runID),
		RunID:         runID,
		SessionKey:    sessionKey,
		ArtifactType:  TypeAssistantFinal,
		Title:         title,
		Summary:       summary,
		ContentText:   trimmed,
		ContentJSON:   fmt.Sprintf(`{"kind":"assistant_final","length":%d}`, len(trimmed)),
		ContentFormat: "text",
		SourceType:    "message",
		SourceRef:     runID,
		CreatedAt:     createdAt,
	})
}

func (s *Service) SaveToolResult(ctx context.Context, runID, sessionKey, toolCallID, toolName, status, resultJSON string, createdAt time.Time) (Record, error) {
	if strings.TrimSpace(toolCallID) == "" {
		return Record{}, nil
	}
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	title := fmt.Sprintf("Tool result: %s", strings.TrimSpace(toolName))
	summary := summarize(strings.TrimSpace(resultJSON), 180)
	return s.repo.CreateArtifact(ctx, CreateParams{
		ArtifactID:    artifactID("tool", toolCallID),
		RunID:         runID,
		SessionKey:    sessionKey,
		ArtifactType:  TypeToolResult,
		Title:         title,
		Summary:       summary,
		ContentText:   strings.TrimSpace(resultJSON),
		ContentJSON:   fmt.Sprintf(`{"tool_call_id":%q,"tool_name":%q,"status":%q}`, toolCallID, toolName, status),
		ContentFormat: "json",
		SourceType:    "tool_call",
		SourceRef:     toolCallID,
		CreatedAt:     createdAt,
	})
}

func (s *Service) SaveDoctorReport(ctx context.Context, runID, sessionKey, status, reportJSON string, createdAt time.Time) (Record, error) {
	if strings.TrimSpace(reportJSON) == "" {
		return Record{}, nil
	}
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	title := fmt.Sprintf("Doctor report (%s)", strings.TrimSpace(status))
	summary := summarize(strings.TrimSpace(reportJSON), 180)
	return s.repo.CreateArtifact(ctx, CreateParams{
		ArtifactID:    artifactID("doctor", runID+status+createdAt.Format(time.RFC3339Nano)),
		RunID:         runID,
		SessionKey:    sessionKey,
		ArtifactType:  TypeDoctorReport,
		Title:         title,
		Summary:       summary,
		ContentText:   strings.TrimSpace(reportJSON),
		ContentJSON:   fmt.Sprintf(`{"status":%q}`, status),
		ContentFormat: "json",
		SourceType:    "doctor",
		SourceRef:     runID,
		CreatedAt:     createdAt,
	})
}

func (s *Service) SaveBrowserCapture(ctx context.Context, runID, sessionKey, toolCallID, singleTabSessionID, currentURL, currentTitle, imageDataURL string, createdAt time.Time) (Record, error) {
	if strings.TrimSpace(imageDataURL) == "" {
		return Record{}, nil
	}
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	title := "Browser capture"
	if trimmedTitle := strings.TrimSpace(currentTitle); trimmedTitle != "" {
		title = fmt.Sprintf("Browser capture: %s", trimmedTitle)
	}
	summary := summarize(firstNonEmpty(currentURL, currentTitle, singleTabSessionID), 180)
	sourceRef := firstNonEmpty(toolCallID, singleTabSessionID, runID)
	contentJSON := fmt.Sprintf(`{"kind":"browser_capture","single_tab_session_id":%q,"tool_call_id":%q,"current_url":%q,"current_title":%q,"image_data_url":%q}`, singleTabSessionID, toolCallID, currentURL, currentTitle, imageDataURL)

	return s.repo.CreateArtifact(ctx, CreateParams{
		ArtifactID:    artifactID("capture", sourceRef),
		RunID:         runID,
		SessionKey:    sessionKey,
		ArtifactType:  TypeBrowserCapture,
		Title:         title,
		Summary:       summary,
		ContentText:   "",
		ContentJSON:   contentJSON,
		ContentFormat: "image_data_url",
		SourceType:    "single_tab_capture",
		SourceRef:     sourceRef,
		CreatedAt:     createdAt,
	})
}

func artifactID(prefix, seed string) string {
	if strings.TrimSpace(seed) == "" {
		seed = "unknown"
	}
	var raw [4]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("artifact-%s-%d", prefix, time.Now().UTC().UnixNano())
	}
	return fmt.Sprintf("artifact-%s-%s-%s", prefix, safeSeed(seed), hex.EncodeToString(raw[:]))
}

func safeSeed(seed string) string {
	clean := strings.NewReplacer(":", "-", "/", "-", " ", "-").Replace(strings.TrimSpace(seed))
	if len(clean) > 24 {
		clean = clean[:24]
	}
	return clean
}

func summarize(value string, max int) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) <= max {
		return trimmed
	}
	return trimmed[:max]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
