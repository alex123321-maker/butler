package runtime

import (
	"net/url"
	"strings"

	"github.com/butler/butler/internal/security/egress"
	"golang.org/x/net/html"
)

type DocumentLink struct {
	URL  string `json:"url"`
	Text string `json:"text"`
}

// Links retain the route from an overview to its primary references. They are
// untrusted discovery DATA, never an official-source label or permission to fetch.
func documentLinks(body, finalURL string) []DocumentLink {
	base, err := url.Parse(finalURL)
	if err != nil {
		return []DocumentLink{}
	}
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return []DocumentLink{}
	}
	var external, local []DocumentLink
	seen := map[string]bool{}
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style" || n.Data == "nav" || n.Data == "header" || n.Data == "footer") {
			return
		}
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, a := range n.Attr {
				if a.Key == "href" {
					u, e := url.Parse(strings.TrimSpace(a.Val))
					if e != nil || a.Val == "" || strings.HasPrefix(a.Val, "#") {
						break
					}
					u = base.ResolveReference(u)
					u.Fragment = ""
					if _, e = (egress.Policy{}).ValidateURL(u.String()); e != nil || u.User != nil || len(u.String()) > 2048 || seen[u.String()] {
						break
					}
					text := clipSearch(extractReadableText(renderNode(n)), 160)
					if text == "" {
						break
					}
					seen[u.String()] = true
					link := DocumentLink{URL: u.String(), Text: text}
					if strings.EqualFold(u.Hostname(), base.Hostname()) {
						if len(local) < 12 {
							local = append(local, link)
						}
					} else if len(external) < 40 {
						external = append(external, link)
					}
					break
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(doc)
	return append(external, local...)
}
