package telegram

import (
	"fmt"
	"html"
	"strings"
)

const telegramParseModeHTML = "HTML"

// telegramMaxMessageLen is the maximum length of a single Telegram message.
// Telegram's actual limit is 4096 characters; we use a slightly lower value
// to leave headroom for HTML formatting expansion.
const telegramMaxMessageLen = 4000

// splitMessage breaks a long text into chunks that each fit within
// Telegram's message length limit. It tries to split at paragraph
// boundaries (\n\n), then line boundaries (\n), then falls back to
// a hard cut at the limit. The caller must ensure that each chunk
// is rendered/sent separately.
func splitMessage(text string, limit int) []string {
	if limit <= 0 {
		limit = telegramMaxMessageLen
	}
	if len(text) <= limit {
		return []string{text}
	}

	var chunks []string
	for len(text) > 0 {
		if len(text) <= limit {
			chunks = append(chunks, text)
			break
		}
		cutAt := findSplitPoint(text, limit)
		chunks = append(chunks, text[:cutAt])
		text = strings.TrimLeft(text[cutAt:], "\n")
	}
	return chunks
}

// findSplitPoint locates the best split position within text[:limit].
// Prefers paragraph break (\n\n), then line break (\n), then hard cut.
func findSplitPoint(text string, limit int) int {
	window := text[:limit]
	// Prefer paragraph boundary.
	if idx := strings.LastIndex(window, "\n\n"); idx > 0 {
		return idx
	}
	// Prefer line boundary.
	if idx := strings.LastIndex(window, "\n"); idx > 0 {
		return idx
	}
	// Hard cut.
	return limit
}

func renderTelegramHTML(input string) string {
	text := strings.ReplaceAll(input, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return renderTelegramSegment(text)
}

func renderTelegramSegment(text string) string {
	var out strings.Builder
	for i := 0; i < len(text); {
		if strings.HasPrefix(text[i:], "```") {
			end := strings.Index(text[i+3:], "```")
			if end >= 0 {
				code := strings.Trim(text[i+3:i+3+end], "\n")
				out.WriteString("<pre>")
				out.WriteString(html.EscapeString(code))
				out.WriteString("</pre>")
				i += 3 + end + 3
				continue
			}
		}
		if text[i] == '`' {
			end := strings.IndexByte(text[i+1:], '`')
			if end >= 0 {
				out.WriteString("<code>")
				out.WriteString(html.EscapeString(text[i+1 : i+1+end]))
				out.WriteString("</code>")
				i += end + 2
				continue
			}
		}
		if strings.HasPrefix(text[i:], "**") {
			end := strings.Index(text[i+2:], "**")
			if end > 0 {
				out.WriteString("<b>")
				out.WriteString(html.EscapeString(text[i+2 : i+2+end]))
				out.WriteString("</b>")
				i += 2 + end + 2
				continue
			}
		}
		if text[i] == '[' {
			closeText := strings.IndexByte(text[i+1:], ']')
			if closeText >= 0 {
				openURL := i + 1 + closeText + 1
				if openURL < len(text) && openURL+1 < len(text) && text[openURL] == '(' {
					closeURL := strings.IndexByte(text[openURL+1:], ')')
					if closeURL >= 0 {
						label := text[i+1 : i+1+closeText]
						url := text[openURL+1 : openURL+1+closeURL]
						if looksLikeURL(url) {
							out.WriteString("<a href=\"")
							out.WriteString(html.EscapeString(url))
							out.WriteString("\">")
							out.WriteString(html.EscapeString(label))
							out.WriteString("</a>")
							i = openURL + 1 + closeURL + 1
							continue
						}
					}
				}
			}
		}
		out.WriteString(html.EscapeString(text[i : i+1]))
		i++
	}
	return out.String()
}

func looksLikeURL(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://")
}

// --- Tool call UX formatting ---

// toolDisplayName returns a human-friendly label for a tool name.
// It replaces underscores with spaces for readability.
func toolDisplayName(toolName string) string {
	return strings.ReplaceAll(strings.TrimSpace(toolName), "_", " ")
}

// formatToolCallStarted returns a short notification shown when a tool starts.
func formatToolCallStarted(toolName string) string {
	return fmt.Sprintf("\xF0\x9F\x94\xA7 Running tool: %s", toolDisplayName(toolName))
}

// formatToolCallFinished returns a short notification shown when a tool completes.
func formatToolCallFinished(toolName, status string, durationMs int64) string {
	icon := "\xE2\x9C\x85" // ✅
	label := "completed"
	if status == "failed" {
		icon = "\xE2\x9D\x8C" // ❌
		label = "failed"
	}
	if durationMs > 0 {
		return fmt.Sprintf("%s Tool %s %s (%.1fs)", icon, toolDisplayName(toolName), label, float64(durationMs)/1000.0)
	}
	return fmt.Sprintf("%s Tool %s %s", icon, toolDisplayName(toolName), label)
}
