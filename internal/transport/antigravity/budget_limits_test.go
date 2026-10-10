package antigravity

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/butler/butler/internal/transport"
)

func TestNativeSerializedBudgetIncludesHistoryBeforeNetwork(t *testing.T) {
	var calls atomic.Int32
	p := newFixture(t, func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return response(frame(`{"text":"done"}`, "STOP")), nil
	})
	r := request("bounded-native")
	r.InputItems = append(r.InputItems, transport.InputItem{Role: "user", Content: strings.Repeat("context", 1024)})
	stream, err := p.StartRun(transport.WithInputByteLimit(context.Background(), 128), r)
	if err != nil {
		t.Fatal(err)
	}
	result := findError(collect(t, stream))
	if result == nil || result.Type != transport.ErrorTypeInputBytesExceeded || calls.Load() != 0 {
		t.Fatal("oversize context reached native network", result, calls.Load())
	}
	input, output := p.InferenceLimits()
	if input != 1048576 || output != 8192 {
		t.Fatal("unverified declared native allowance", input, output)
	}
}
