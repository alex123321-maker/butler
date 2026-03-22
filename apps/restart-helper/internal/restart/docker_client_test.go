package restart

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestDockerClientRestartServiceRestartsMatchingContainers(t *testing.T) {
	t.Helper()

	var listed bool
	restarted := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/containers/json":
			listed = true
			if got := r.URL.Query().Get("all"); got != "1" {
				t.Fatalf("expected all=1, got %q", got)
			}
			filters := make(map[string][]string)
			if err := json.Unmarshal([]byte(r.URL.Query().Get("filters")), &filters); err != nil {
				t.Fatalf("decode filters: %v", err)
			}
			if labels := filters["label"]; len(labels) != 2 || labels[0] != "com.docker.compose.project=butler" || labels[1] != "com.docker.compose.service=web" {
				t.Fatalf("unexpected label filters: %+v", labels)
			}
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"Id": "b-container", "Labels": map[string]string{"com.docker.compose.service": "web"}},
				{"Id": "a-container", "Labels": map[string]string{"com.docker.compose.service": "web"}},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/containers/a-container/restart":
			restarted = append(restarted, "a-container")
			if got := r.URL.Query().Get("t"); got != "15" {
				t.Fatalf("expected timeout 15, got %q", got)
			}
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/containers/b-container/restart":
			restarted = append(restarted, "b-container")
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client, err := NewDockerClient(server.URL, 5*time.Second)
	if err != nil {
		t.Fatalf("new docker client: %v", err)
	}

	if err := client.RestartService(context.Background(), "butler", "web", 15*time.Second); err != nil {
		t.Fatalf("restart service: %v", err)
	}
	if !listed {
		t.Fatal("expected list containers call")
	}
	if want := []string{"a-container", "b-container"}; len(restarted) != len(want) || restarted[0] != want[0] || restarted[1] != want[1] {
		t.Fatalf("unexpected restarted containers: %+v", restarted)
	}
}

func TestDockerClientPing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/_ping" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte("OK"))
	}))
	defer server.Close()

	client, err := NewDockerClient(server.URL, 5*time.Second)
	if err != nil {
		t.Fatalf("new docker client: %v", err)
	}
	if err := client.Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
}

func TestNewDockerClientRejectsUnsupportedScheme(t *testing.T) {
	_, err := NewDockerClient((&url.URL{Scheme: "npipe", Host: "docker"}).String(), time.Second)
	if err == nil {
		t.Fatal("expected unsupported scheme error")
	}
}
