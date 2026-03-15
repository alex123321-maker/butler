package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
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
	CheckDatabase(context.Context) SystemReport
	CheckContainer(context.Context) SystemReport
	CheckProvider(context.Context) SystemReport
}

type Checker struct {
	cfg            config.ToolDoctorConfig
	snapshot       config.Introspector
	postgresCheck  func(context.Context) error
	redisCheck     func(context.Context) error
	providerCheck  func(context.Context) error
	containerCheck func(context.Context, config.DoctorContainerTarget) error
	httpClient     *http.Client
	now            func() time.Time
}

func NewChecker(cfg config.ToolDoctorConfig, snapshot config.Introspector) *Checker {
	checker := &Checker{cfg: cfg, snapshot: snapshot, now: time.Now, httpClient: &http.Client{Timeout: 5 * time.Second}}
	checker.postgresCheck = checker.defaultPostgresCheck
	checker.redisCheck = checker.defaultRedisCheck
	checker.providerCheck = checker.defaultProviderCheck
	checker.containerCheck = checker.defaultContainerCheck
	return checker
}

func (c *Checker) CheckSystem(ctx context.Context) SystemReport {
	checkedAt := c.now().UTC().Format(time.RFC3339)
	checks := []health.CheckResult{
		c.checkConfig(),
	}
	checks = append(checks, c.databaseChecks(ctx)...)
	return SystemReport{Status: aggregateStatus(checks), CheckedAt: checkedAt, Checks: checks, Config: configEntries(c.snapshot)}
}

func (c *Checker) CheckDatabase(ctx context.Context) SystemReport {
	checks := c.databaseChecks(ctx)
	return SystemReport{Status: aggregateStatus(checks), CheckedAt: c.now().UTC().Format(time.RFC3339), Checks: checks}
}

func (c *Checker) CheckContainer(ctx context.Context) SystemReport {
	checks := c.containerChecks(ctx)
	return SystemReport{Status: aggregateStatus(checks), CheckedAt: c.now().UTC().Format(time.RFC3339), Checks: checks}
}

func (c *Checker) CheckProvider(ctx context.Context) SystemReport {
	checks := []health.CheckResult{c.runCheck(ctx, "provider.openai", strings.TrimSpace(c.cfg.OpenAIAPIKey) != "", c.providerCheck)}
	return SystemReport{Status: aggregateStatus(checks), CheckedAt: c.now().UTC().Format(time.RFC3339), Checks: checks}
}

func (c *Checker) databaseChecks(ctx context.Context) []health.CheckResult {
	return []health.CheckResult{
		c.runCheck(ctx, "postgres", c.cfg.Postgres.URL != "", c.postgresCheck),
		c.runCheck(ctx, "redis", c.cfg.Redis.URL != "", c.redisCheck),
	}
}

func (c *Checker) containerChecks(ctx context.Context) []health.CheckResult {
	if len(c.cfg.ContainerTargets) == 0 {
		return []health.CheckResult{{Name: "containers", Status: health.StatusDegraded, Message: "not configured", Duration: "0s", CheckedAt: c.now().UTC().Format(time.RFC3339)}}
	}
	checks := make([]health.CheckResult, 0, len(c.cfg.ContainerTargets))
	for _, target := range c.cfg.ContainerTargets {
		target := target
		checks = append(checks, c.runCheck(ctx, "container."+target.Name, strings.TrimSpace(target.URL) != "", func(callCtx context.Context) error {
			return c.containerCheck(callCtx, target)
		}))
	}
	return checks
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

func (c *Checker) defaultProviderCheck(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.cfg.OpenAIBaseURL, "/")+"/models", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.OpenAIAPIKey)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("provider returned status %d", resp.StatusCode)
	}
	return nil
}

func (c *Checker) defaultContainerCheck(ctx context.Context, target config.DoctorContainerTarget) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.URL, nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("health endpoint returned status %d", resp.StatusCode)
	}
	var report health.Report
	if err := json.NewDecoder(resp.Body).Decode(&report); err == nil && report.Status == health.StatusUnhealthy {
		return fmt.Errorf("health endpoint reported unhealthy")
	}
	return nil
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
