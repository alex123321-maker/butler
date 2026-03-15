package runtime

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/health"
	postgresstore "github.com/butler/butler/internal/storage/postgres"
	redisstore "github.com/butler/butler/internal/storage/redis"
)

type SystemReport struct {
	Status    health.Status          `json:"status"`
	CheckedAt string                 `json:"checked_at"`
	Checks    []health.CheckResult   `json:"checks"`
	Config    []config.ConfigKeyInfo `json:"config"`
}

type SystemInspector interface {
	CheckSystem(context.Context) SystemReport
}

type Checker struct {
	cfg           config.ToolDoctorConfig
	snapshot      config.Introspector
	postgresCheck func(context.Context) error
	redisCheck    func(context.Context) error
	now           func() time.Time
}

func NewChecker(cfg config.ToolDoctorConfig, snapshot config.Introspector) *Checker {
	checker := &Checker{cfg: cfg, snapshot: snapshot, now: time.Now}
	checker.postgresCheck = checker.defaultPostgresCheck
	checker.redisCheck = checker.defaultRedisCheck
	return checker
}

func (c *Checker) CheckSystem(ctx context.Context) SystemReport {
	checkedAt := c.now().UTC().Format(time.RFC3339)
	checks := []health.CheckResult{
		c.checkConfig(),
		c.runCheck(ctx, "postgres", c.cfg.Postgres.URL != "", c.postgresCheck),
		c.runCheck(ctx, "redis", c.cfg.Redis.URL != "", c.redisCheck),
	}
	return SystemReport{Status: aggregateStatus(checks), CheckedAt: checkedAt, Checks: checks, Config: configEntries(c.snapshot)}
}

func (c *Checker) checkConfig() health.CheckResult {
	result := health.CheckResult{Name: "config", Status: health.StatusHealthy, Duration: "0s", CheckedAt: c.now().UTC().Format(time.RFC3339)}
	entries := configEntries(c.snapshot)
	invalid := 0
	missing := 0
	for _, entry := range entries {
		switch entry.ValidationStatus {
		case config.ValidationStatusInvalid:
			invalid++
		case config.ValidationStatusMissing:
			missing++
		}
	}
	if invalid > 0 || missing > 0 {
		result.Status = health.StatusUnhealthy
		result.Message = fmt.Sprintf("config issues detected: invalid=%d missing=%d", invalid, missing)
		return result
	}
	result.Message = fmt.Sprintf("config snapshot contains %d keys", len(entries))
	return result
}

func (c *Checker) runCheck(ctx context.Context, name string, configured bool, fn func(context.Context) error) health.CheckResult {
	started := c.now()
	result := health.CheckResult{Name: name, CheckedAt: started.UTC().Format(time.RFC3339)}
	if !configured {
		result.Status = health.StatusDegraded
		result.Message = "not configured"
		result.Duration = "0s"
		return result
	}
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err := fn(callCtx)
	finished := c.now()
	result.Duration = finished.Sub(started).String()
	result.CheckedAt = finished.UTC().Format(time.RFC3339)
	if err != nil {
		result.Status = health.StatusUnhealthy
		result.Message = err.Error()
		return result
	}
	result.Status = health.StatusHealthy
	result.Message = "ok"
	return result
}

func (c *Checker) defaultPostgresCheck(ctx context.Context) error {
	store, err := postgresstore.Open(ctx, postgresstore.ConfigFromShared(c.cfg.Postgres), nil)
	if err != nil {
		return err
	}
	defer store.Close()
	return store.HealthCheck(ctx)
}

func (c *Checker) defaultRedisCheck(ctx context.Context) error {
	store, err := redisstore.Open(ctx, redisstore.ConfigFromShared(c.cfg.Redis), nil)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()
	return store.HealthCheck(ctx)
}

func configEntries(snapshot config.Introspector) []config.ConfigKeyInfo {
	if snapshot == nil {
		return nil
	}
	entries := snapshot.ListKeys()
	sort.Slice(entries, func(i, j int) bool { return entries[i].Key < entries[j].Key })
	return entries
}

func aggregateStatus(results []health.CheckResult) health.Status {
	status := health.StatusHealthy
	for _, result := range results {
		switch result.Status {
		case health.StatusUnhealthy:
			return health.StatusUnhealthy
		case health.StatusDegraded:
			status = health.StatusDegraded
		}
	}
	return status
}
