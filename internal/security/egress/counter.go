package egress

import (
	"context"
	"net/http"
	"sync"
)

type Counter struct {
	mu        sync.Mutex
	remaining int64
}

func NewCounter(limit int64) *Counter { return &Counter{remaining: limit} }
func (c *Counter) Take(bytes int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if bytes < 0 || bytes > c.remaining {
		return ErrLimit
	}
	c.remaining -= bytes
	return nil
}

type counterKey struct{}

func WithCounter(ctx context.Context, counter *Counter) context.Context {
	return context.WithValue(ctx, counterKey{}, counter)
}
func counterFrom(ctx context.Context) *Counter {
	value, _ := ctx.Value(counterKey{}).(*Counter)
	return value
}

// TakeRequest accounts a fixed operator-service request. It grants no access.
func TakeRequest(ctx context.Context, request *http.Request) error {
	size, err := requestSize(request, MaxRequestBytes)
	if err != nil {
		return err
	}
	if counter := counterFrom(ctx); counter != nil {
		return counter.Take(size)
	}
	return nil
}
