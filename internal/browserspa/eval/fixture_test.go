package eval

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPublicDocumentDoesNotContainExpectedValues(t *testing.T) {
	c := Config{Origin: "https://recipient.example", Variant: "independent01", Category: "books", Title: "Private title 7982", Marker: "Private-receipt-8251", Stock: 9761}
	var events []Event
	f, err := New(c, func(e Event) { events = append(events, e) })
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	f.ServeHTTP(w, httptest.NewRequest("GET", f.URL(), nil))
	for _, secret := range []string{c.Title, c.Marker, "9761"} {
		if strings.Contains(w.Body.String(), secret) {
			t.Fatal("expected value present in initial HTML")
		}
	}
	if len(events) != 1 || events[0].Kind != "document" {
		t.Fatal("recipient observation absent")
	}
	w = httptest.NewRecorder()
	f.ServeHTTP(w, httptest.NewRequest("POST", f.URL()+"graphql", strings.NewReader(`{"query":"mutation{buy}","variables":{"category":"books"}}`)))
	if w.Code != 422 || events[len(events)-1].Accepted {
		t.Fatal("recipient accepted unrecognized query")
	}
}
