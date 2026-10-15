package artifacts

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) SaveAssistantFinal(ctx context.Context, runID, sessionKey, content string, createdAt time.Time) (Record, error) {
	trimmed := cleanArtifactString(strings.TrimSpace(content))
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
	toolName = cleanArtifactString(strings.TrimSpace(toolName))
	status = cleanArtifactString(strings.TrimSpace(status))
	resultJSON = cleanArtifactString(strings.TrimSpace(resultJSON))
	title := fmt.Sprintf("Tool result: %s", toolName)
	summary := summarize(resultJSON, 180)
	return s.repo.CreateArtifact(ctx, CreateParams{
		ArtifactID:    artifactID("tool", toolCallID),
		RunID:         runID,
		SessionKey:    sessionKey,
		ArtifactType:  TypeToolResult,
		Title:         title,
		Summary:       summary,
		ContentText:   resultJSON,
		ContentJSON:   fmt.Sprintf(`{"tool_call_id":%q,"tool_name":%q,"status":%q}`, toolCallID, toolName, status),
		ContentFormat: "json",
		SourceType:    "tool_call",
		SourceRef:     toolCallID,
		CreatedAt:     createdAt,
	})
}

func (s *Service) SaveDoctorReport(ctx context.Context, runID, sessionKey, status, reportJSON string, createdAt time.Time) (Record, error) {
	reportJSON = cleanArtifactString(strings.TrimSpace(reportJSON))
	status = cleanArtifactString(strings.TrimSpace(status))
	if reportJSON == "" {
		return Record{}, nil
	}
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	title := fmt.Sprintf("Doctor report (%s)", status)
	summary := summarize(reportJSON, 180)
	return s.repo.CreateArtifact(ctx, CreateParams{
		ArtifactID:    artifactID("doctor", runID+status+createdAt.Format(time.RFC3339Nano)),
		RunID:         runID,
		SessionKey:    sessionKey,
		ArtifactType:  TypeDoctorReport,
		Title:         title,
		Summary:       summary,
		ContentText:   reportJSON,
		ContentJSON:   fmt.Sprintf(`{"status":%q}`, status),
		ContentFormat: "json",
		SourceType:    "doctor",
		SourceRef:     runID,
		CreatedAt:     createdAt,
	})
}

func (s *Service) SaveBrowserCapture(ctx context.Context, runID, sessionKey, toolCallID, singleTabSessionID, currentURL, currentTitle, imageDataURL string, createdAt time.Time) (Record, error) {
	singleTabSessionID = cleanArtifactString(strings.TrimSpace(singleTabSessionID))
	currentURL = cleanArtifactString(strings.TrimSpace(currentURL))
	currentTitle = cleanArtifactString(strings.TrimSpace(currentTitle))
	imageDataURL = cleanArtifactString(strings.TrimSpace(imageDataURL))
	imageDataURL = normalizeImageDataURL(imageDataURL)
	if imageDataURL == "" {
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
	trimmed := cleanArtifactString(strings.TrimSpace(value))
	if len(trimmed) <= max {
		return trimmed
	}
	for max > 0 && !utf8.RuneStart(trimmed[max]) {
		max--
	}
	return trimmed[:max]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := cleanArtifactString(strings.TrimSpace(value)); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func cleanArtifactString(value string) string {
	if value == "" {
		return ""
	}
	return strings.ToValidUTF8(value, "\uFFFD")
}

func normalizeImageDataURL(value string) string {
	value = strings.TrimSpace(cleanArtifactString(value))
	if value == "" || !strings.HasPrefix(strings.ToLower(value), "data:image/") {
		return value
	}
	parts := strings.SplitN(value, ",", 2)
	if len(parts) != 2 {
		return value
	}
	meta := strings.TrimSpace(parts[0])
	payload := strings.TrimSpace(parts[1])
	if !strings.Contains(strings.ToLower(meta), ";base64") {
		return value
	}
	payload = strings.Map(func(r rune) rune {
		switch r {
		case '\r', '\n', '\t', ' ':
			return -1
		default:
			return r
		}
	}, payload)
	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(payload)
		if err != nil {
			return value
		}
	}
	return meta + "," + base64.StdEncoding.EncodeToString(decoded)
}
