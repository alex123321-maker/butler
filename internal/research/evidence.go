// Package research checks the provenance of a research answer, not the truth or
// institutional authority of arbitrary web prose. Page content remains DATA.
package research

import (
	"encoding/json"
	"golang.org/x/net/html"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/butler/butler/internal/memory/transcript"
)

type Evidence struct {
	URL   string `json:"url"`
	Quote string `json:"quote"`
	Role  string `json:"role"` // primary or secondary: a model attribution, not authority
}
type Page struct {
	URL, Text string
	ReadAt    time.Time
}

func CanonicalURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
		return ""
	}
	u.Fragment = ""
	u.Host = strings.ToLower(u.Host)
	if u.Path == "" {
		u.Path = "/"
	}
	return u.String()
}
func normalized(s string) string { return strings.Join(strings.Fields(s), " ") }

func IsReadAttempt(c transcript.ToolCall) bool {
	var a struct{ URL, Text, HTML, Method string }
	switch c.ToolName {
	case "web.search", "web.fetch", "web.fetch_batch":
		return true
	case "web.extract":
		return json.Unmarshal([]byte(c.ArgsJSON), &a) == nil && a.URL != "" && a.Text == "" && a.HTML == ""
	case "http.request":
		return json.Unmarshal([]byte(c.ArgsJSON), &a) == nil && (a.Method == "" || strings.EqualFold(a.Method, "GET") || strings.EqualFold(a.Method, "HEAD"))
	case "browser.read", "browser.open", "browser.navigate", "browser.snapshot", "browser.extract_text":
		return true
	}
	return false
}

// Pages only accepts successful actual reads. Search snippets, extraction of
// model-supplied text, failed calls and previous-run receipts are not evidence.
func Pages(calls []transcript.ToolCall, run string) ([]Page, bool) {
	var pages []Page
	researched := false
	for _, c := range calls {
		if c.RunID != run {
			continue
		}
		// Ordinary HTTP/browser automation is not automatically research. Those
		// readers can corroborate a research request without imposing citations
		// on a sign-in, form submission or other unrelated owner action.
		if IsReadAttempt(c) && (c.ToolName == "web.search" || c.ToolName == "web.fetch" || c.ToolName == "web.fetch_batch" || c.ToolName == "web.extract") {
			researched = true
		}
		if c.FinishedAt != nil && c.Status == "completed" {
			if p, ok := alternativePage(c); ok {
				pages = append(pages, p)
				continue
			}
		}
		if c.FinishedAt == nil || c.Status != "completed" || (c.ToolName != "web.fetch" && c.ToolName != "web.fetch_batch") {
			continue
		}
		var result struct {
			Results []json.RawMessage `json:"items"`
		}
		if json.Unmarshal([]byte(c.ResultJSON), &result) != nil {
			continue
		}
		items := []json.RawMessage{json.RawMessage(c.ResultJSON)}
		if c.ToolName == "web.fetch_batch" {
			items = result.Results
		}
		for _, raw := range items {
			var p struct {
				URL       string `json:"final_url"`
				Requested string `json:"requested_url"`
				Text      string `json:"content_text"`
				Status    int    `json:"status_code"`
			}
			if json.Unmarshal(raw, &p) != nil || p.Status < 200 || p.Status >= 300 || strings.TrimSpace(p.Text) == "" {
				continue
			}
			if u := CanonicalURL(p.URL); u != "" {
				pages = append(pages, Page{u, normalized(p.Text), *c.FinishedAt})
			}
			if u := CanonicalURL(p.Requested); u != "" && u != CanonicalURL(p.URL) {
				pages = append(pages, Page{u, normalized(p.Text), *c.FinishedAt})
			}
		}
	}
	return pages, researched
}

func alternativePage(c transcript.ToolCall) (Page, bool) {
	var args struct{ URL, Text, HTML, Method string }
	if json.Unmarshal([]byte(c.ArgsJSON), &args) != nil {
		return Page{}, false
	}
	var out struct {
		URL         string          `json:"final_url"`
		Text        string          `json:"content_text"`
		BrowserText string          `json:"text"`
		Body        string          `json:"body"`
		Status      int             `json:"status_code"`
		OK          bool            `json:"ok"`
		Error       json.RawMessage `json:"error"`
		Local       bool            `json:"local_preparation"`
		Network     string          `json:"network_state"`
	}
	if json.Unmarshal([]byte(c.ResultJSON), &out) != nil {
		return Page{}, false
	}
	switch c.ToolName {
	case "web.extract":
		if args.URL == "" || args.Text != "" || args.HTML != "" || out.Status < 200 || out.Status >= 300 {
			return Page{}, false
		}
	case "http.request":
		if (args.Method != "" && !strings.EqualFold(args.Method, "GET")) || out.Status < 200 || out.Status >= 300 {
			return Page{}, false
		}
		out.URL = args.URL
		out.Text = readable(out.Body)
	case "browser.read", "browser.open", "browser.navigate", "browser.snapshot", "browser.extract_text":
		if !out.OK || out.Local || out.Network == "frozen" || len(out.Error) > 0 && string(out.Error) != "null" {
			return Page{}, false
		}
		out.Text = out.BrowserText
	default:
		return Page{}, false
	}
	u := CanonicalURL(out.URL)
	if u == "" || strings.TrimSpace(out.Text) == "" {
		return Page{}, false
	}
	return Page{u, normalized(out.Text), *c.FinishedAt}, true
}

func readable(body string) string {
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return ""
	}
	var out strings.Builder
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style") {
			return
		}
		if n.Type == html.TextNode {
			out.WriteString(n.Data)
			out.WriteByte(' ')
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(doc)
	return out.String()
}

func Supports(e Evidence, pages []Page, now time.Time) bool {
	u, quote := CanonicalURL(e.URL), normalized(e.Quote)
	if u == "" || utf8.RuneCountInString(quote) < 12 || utf8.RuneCountInString(quote) > 600 {
		return false
	}
	for _, p := range pages {
		if p.URL == u && !p.ReadAt.After(now.Add(time.Minute)) && now.Sub(p.ReadAt) <= 24*time.Hour && strings.Contains(p.Text, quote) {
			return true
		}
	}
	return false
}

var links = regexp.MustCompile(`https?://[^\s<>"\x60\]\)]+`)

// Check returns finite actionable feedback. Primary attribution still needs
// semantic evaluation: an opened page or a word "official" cannot prove it.
func Check(text string, evidence []Evidence, pages []Page, now time.Time, detailed bool) string {
	if !detailed && utf8.RuneCountInString(text) > 1800 {
		return "Keep the research answer within 1800 characters: requested facts, material uncertainty, source links and at most one useful follow-up. Omit background and the research process. Use a report only when the owner asked for detail."
	}
	if len(evidence) == 0 {
		return "The research result needs evidence from opened primary pages. Add evidence entries (url, exact short quote, role primary/secondary). Search snippets and previous answers do not verify current facts. If current primary evidence is unavailable, publish a concise blocked response explaining what remains unverified."
	}
	primary := false
	for _, e := range evidence {
		if (e.Role != "primary" && e.Role != "secondary") || !Supports(e, pages, now) {
			return "An evidence entry is not supported by a successful fresh page read in this run. Open the cited page with web.fetch and use an exact short quote from its content_text; do not invent a quote or use a search snippet."
		}
		primary = primary || e.Role == "primary"
	}
	if !primary {
		return "Use a current first-party announcement, schedule or issuer page as primary evidence. Overview pages are discovery aids. If first-party confirmation is unavailable, publish a concise blocked response that distinguishes secondary indications from verified facts."
	}
	linked := false
	for _, raw := range links.FindAllString(text, -1) {
		u := CanonicalURL(strings.TrimRight(raw, ".,;:!?"))
		found := false
		for _, e := range evidence {
			found = found || u != "" && u == CanonicalURL(e.URL)
		}
		if !found {
			return "Every source link in the answer must refer to an evidence entry from a page actually read in this run. Remove unsupported links or read and cite them."
		}
		linked = true
	}
	if !linked {
		return "Include a direct link to the primary page supporting the answer."
	}
	return ""
}
