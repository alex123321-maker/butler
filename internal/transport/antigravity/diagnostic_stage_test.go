package antigravity

import (
	"context"
	"errors"
	"io"
	"net/http"
	"reflect"
	"sync"
	"testing"

	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/transport"
)

type unavailableStageAuth struct{}

func (unavailableStageAuth) ResolveAntigravity(context.Context) (providerauth.AntigravityAuth, error) {
	return providerauth.AntigravityAuth{}, errors.New("PRIVATE_AUTH_ERROR")
}

func (a unavailableStageAuth) ResolveAntigravityCloudCode(ctx context.Context) (providerauth.AntigravityAuth, error) {
	return a.ResolveAntigravity(ctx)
}

func TestProviderDiagnosticStageSeparatesAuthHTTPAndStreamFailure(t *testing.T) {
	for _, high := range []bool{false, true} {
		for _, tc := range []struct {
			name     string
			auth     bool
			response bool
			stages   []transport.ProviderStage
		}{
			{"authentication", true, false, []transport.ProviderStage{transport.ProviderStageAuthentication}},
			{"round_trip", false, false, []transport.ProviderStage{transport.ProviderStageAuthentication, transport.ProviderStageSerialization, transport.ProviderStageHTTP}},
			{"response_stream", false, true, []transport.ProviderStage{transport.ProviderStageAuthentication, transport.ProviderStageSerialization, transport.ProviderStageHTTP, transport.ProviderStageStream}},
		} {
			t.Run(map[bool]string{false: "tiered", true: "high"}[high]+"/"+tc.name, func(t *testing.T) {
				var mu sync.Mutex
				var stages []transport.ProviderStage
				ctx := transport.WithProviderStageObserver(context.Background(), func(s transport.ProviderStage) { mu.Lock(); defer mu.Unlock(); stages = append(stages, s) })
				calls := 0
				p := newFixture(t, func(*http.Request) (*http.Response, error) {
					calls++
					if !tc.response {
						return nil, errors.New("PRIVATE_NETWORK_ERROR")
					}
					r := response("")
					r.Body = io.NopCloser(incompleteFailureReader{})
					return r, nil
				})
				if tc.auth {
					p.config.AuthSource = unavailableStageAuth{}
				} else if high {
					p.config.AuthSource = cloudAuthSource{project: "discovered-project"}
				}
				r := request("private-stage")
				if high {
					r.Context.ModelName = FlashHighModel
				}
				stream, err := p.StartRun(ctx, r)
				if err != nil {
					t.Fatal("fixture admission failed")
				}
				events := collect(t, stream)
				if findError(events) == nil {
					t.Fatal("phase failure was not retained")
				}
				for _, e := range events {
					if e.ToolCall != nil || e.AssistantFinal != nil || e.Usage != nil {
						t.Fatal("diagnostic phase created an action or cost")
					}
				}
				mu.Lock()
				defer mu.Unlock()
				if !reflect.DeepEqual(stages, tc.stages) || tc.auth && calls != 0 || !tc.auth && calls != 1 {
					t.Fatal("failure phase or once-only request attribution changed")
				}
			})
		}
	}
}
