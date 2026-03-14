package health

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"time"
)

type Status string

const (
	StatusHealthy   Status = "healthy"
	StatusDegraded  Status = "degraded"
	StatusUnhealthy Status = "unhealthy"
)

type CheckResult struct {
	Name      string `json:"name"`
	Status    Status `json:"status"`
	Message   string `json:"message,omitempty"`
	Duration  string `json:"duration"`
	CheckedAt string `json:"checked_at"`
}

type Report struct {
	Status    Status        `json:"status"`
	Service   string        `json:"service"`
	CheckedAt string        `json:"checked_at"`
	Checks    []CheckResult `json:"checks"`
}

type Checker interface {
	Name() string
	Check(ctx context.Context) error
}

type FuncChecker struct {
	CheckName string
	Fn        func(context.Context) error
}

func (c FuncChecker) Name() string {
	return c.CheckName
}

func (c FuncChecker) Check(ctx context.Context) error {
	if c.Fn == nil {
		return nil
	}
	return c.Fn(ctx)
}

type Service struct {
	serviceName string
	checkers    []Checker
	now         func() time.Time
}

func New(serviceName string, checkers ...Checker) *Service {
	return &Service{
		serviceName: serviceName,
		checkers:    checkers,
		now:         time.Now,
	}
}

func (s *Service) Check(ctx context.Context) Report {
	checkedAt := s.now().UTC().Format(time.RFC3339)
	results := make([]CheckResult, 0, len(s.checkers))

	for _, checker := range s.checkers {
		started := s.now()
		err := checker.Check(ctx)
		finished := s.now()

		result := CheckResult{
			Name:      checker.Name(),
			Status:    StatusHealthy,
			Duration:  finished.Sub(started).String(),
			CheckedAt: finished.UTC().Format(time.RFC3339),
		}
		if err != nil {
			result.Status = StatusUnhealthy
			result.Message = err.Error()
		}

		results = append(results, result)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Name < results[j].Name
	})

	return Report{
		Status:    aggregateStatus(results),
		Service:   s.serviceName,
		CheckedAt: checkedAt,
		Checks:    results,
	}
}

func (s *Service) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		report := s.Check(r.Context())
		statusCode := http.StatusOK
		if report.Status == StatusUnhealthy {
			statusCode = http.StatusServiceUnavailable
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		_ = json.NewEncoder(w).Encode(report)
	})
}

func aggregateStatus(results []CheckResult) Status {
	if len(results) == 0 {
		return StatusHealthy
	}
	for _, result := range results {
		if result.Status == StatusUnhealthy {
			return StatusUnhealthy
		}
	}
	for _, result := range results {
		if result.Status == StatusDegraded {
			return StatusDegraded
		}
	}
	return StatusHealthy
}
