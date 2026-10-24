package runtime

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	"github.com/butler/butler/internal/memory/sanitize"
	"github.com/butler/butler/internal/security/egress"
	"github.com/butler/butler/internal/websearch"
	"golang.org/x/net/html"
)

type searchItem struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

func (s *Server) executeSearch(ctx context.Context, req *runtimev1.ExecuteRequest) (*runtimev1.ExecuteResponse, error) {
	args, err := websearch.Decode(req.GetToolCall().GetArgsJson())
	if err != nil {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, err.Error())}, nil
	}
	resource, _ := websearch.URL(args.Query)

	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, resource, nil)
	request.Header.Set("User-Agent", "Butler/1.0 (public web research)")
	response, err := s.searchClient.Do(request)
	if err != nil {
		return &runtimev1.ExecuteResponse{Result: fetchFailedResult(req, err)}, nil
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "search provider unavailable or challenged; no results verified")}, nil
	}
	body, truncated, err := readContent(response.Body)
	if err == nil {
		_, err = textMIME(response.Header.Get("Content-Type"), body)
	}
	if err != nil {
		return &runtimev1.ExecuteResponse{Result: fetchFailedResult(req, err)}, nil
	}
	if truncated {
		return &runtimev1.ExecuteResponse{Result: fetchFailedResult(req, errResponseLimit)}, nil
	}
	items, err := parseSearch(string(body), args.Limit)
	if err != nil {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "search response was not recognized; no results verified")}, nil
	}
	return &runtimev1.ExecuteResponse{Result: completedResult(req, sanitize.JSON(mustJSON(map[string]any{"provider": "duckduckgo_lite", "query": strings.TrimSpace(args.Query), "searched_at": time.Now().UTC().Format(time.RFC3339), "results": items, "count": len(items), "evidence": "Search snippets are discovery observations. Open primary sources to verify current dates, times and claims."})))}, nil
}

// Parse only the provider's result structure. A challenge or changed markup is
// an unavailable search, never a fabricated successful empty result.
func parseSearch(body string, limit int) ([]searchItem, error) {
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	items := []searchItem{}
	seen := map[string]bool{}
	noResults := false
	active := -1
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		classes, href := "", ""
		for _, a := range n.Attr {
			if a.Key == "class" {
				classes = " " + a.Val + " "
			}
			if a.Key == "href" {
				href = a.Val
			}
		}
		if strings.Contains(classes, " no-results ") || strings.Contains(classes, " no-results__message ") {
			noResults = true
		}
		if n.Type == html.ElementNode && n.Data == "a" && strings.Contains(classes, " result-link ") {
			active = -1
			u, err := url.Parse(href)
			if err == nil {
				if (u.Hostname() == "duckduckgo.com" || u.Hostname() == "www.duckduckgo.com") && u.Path == "/l/" {
					href = u.Query().Get("uddg")
				}
				if _, err = (egress.Policy{}).ValidateURL(href); err == nil && !seen[href] && len(items) < limit {
					seen[href] = true
					items = append(items, searchItem{Title: clipSearch(extractReadableText(renderNode(n)), 300), URL: href})
					active = len(items) - 1
				}
			}
		}
		if strings.Contains(classes, " result-snippet ") && active >= 0 && items[active].Snippet == "" {
			items[active].Snippet = clipSearch(extractReadableText(renderNode(n)), 1000)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(doc)
	if len(items) == 0 && !noResults {
		return nil, fmt.Errorf("unrecognized search response")
	}
	return items, nil
}

func renderNode(n *html.Node) string {
	var b strings.Builder
	_ = html.Render(&b, n)
	return b.String()
}
func clipSearch(s string, limit int) string {
	r := []rune(s)
	if len(r) > limit {
		return string(r[:limit]) + "…"
	}
	return s
}
