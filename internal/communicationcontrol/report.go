package communicationcontrol

import "strings"

// ValidReportBlock rejects fields a renderer would silently discard. The flat
// typed V1 document has no raw HTML, executable code, nested blocks or URL media.
func ValidReportBlock(b Block) bool {
	text := strings.TrimSpace(b.Text) != ""
	items, rows, ref, alt := len(b.Items) > 0, len(b.Rows) > 0, b.ArtifactRef != "", b.Alt != ""
	switch b.Type {
	case "paragraph", "heading", "quote", "code":
		return text && !items && !rows && !ref && !alt
	case "list":
		if b.Text != "" || !items || rows || ref || alt {
			return false
		}
		for _, item := range b.Items {
			if strings.TrimSpace(item) == "" {
				return false
			}
		}
		return true
	case "table":
		if b.Text != "" || items || !rows || ref || alt {
			return false
		}
		width := len(b.Rows[0])
		if width == 0 || width > 20 {
			return false
		}
		for _, row := range b.Rows {
			if len(row) != width {
				return false
			}
		}
		return true
	case "divider":
		return b.Text == "" && !items && !rows && !ref && !alt
	case "image":
		return b.Text == "" && !items && !rows && ref && strings.TrimSpace(b.Alt) != ""
	case "document":
		return b.Text == "" && !items && !rows && ref
	}
	return false
}
