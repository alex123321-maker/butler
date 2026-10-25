package runtime

import (
	"strings"
	"testing"
)

func TestDocumentLinksRetainPrimaryReferencesWithoutPromotingTrust(t *testing.T) {
	body := `<nav><a href="https://navigation.example/">menu</a></nav><a href="/schedule">Schedule</a><a href="https://publisher.example/news#date">Publisher announcement</a><a href="https://publisher.example/news">duplicate</a><a href="http://127.0.0.1/private">bad</a><a href="javascript:alert(1)">bad</a><a href="#top">top</a>`
	links := documentLinks(body, "https://overview.example/article")
	if len(links) != 2 || links[0].URL != "https://publisher.example/news" || links[1].URL != "https://overview.example/schedule" {
		t.Fatal(links)
	}
	links = documentLinks(strings.Repeat(`<a href="https://publisher.example/news">text</a>`, 100), "https://overview.example/")
	if len(links) != 1 {
		t.Fatal("duplicate link growth", len(links))
	}
}
