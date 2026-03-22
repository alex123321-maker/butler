package restart

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

var (
	ErrServiceNotAllowed = errors.New("service is not allowed for restart")
	ErrSelfRestartDenied = errors.New("restart helper cannot restart itself")
)

type Config struct {
	Project         string
	AllowedServices []string
	SelfService     string
	Delay           time.Duration
	RestartTimeout  time.Duration
}

type RestartAcceptance struct {
	Accepted         bool     `json:"accepted"`
	Project          string   `json:"project"`
	Services         []string `json:"services"`
	ScheduledAt      string   `json:"scheduled_at"`
	DelaySeconds     int      `json:"delay_seconds"`
	SuggestedCommand string   `json:"suggested_command,omitempty"`
}

type Scheduler interface {
	Schedule(delay time.Duration, fn func())
}

type afterFuncScheduler struct{}

func (afterFuncScheduler) Schedule(delay time.Duration, fn func()) {
	time.AfterFunc(delay, fn)
}

type dockerRestarter interface {
	RestartService(ctx context.Context, project, service string, timeout time.Duration) error
}

type Service struct {
	project        string
	selfService    string
	allowed        map[string]struct{}
	delay          time.Duration
	restartTimeout time.Duration
	docker         dockerRestarter
	scheduler      Scheduler
	log            *slog.Logger
	now            func() time.Time
}

func NewService(cfg Config, docker dockerRestarter, log *slog.Logger) (*Service, error) {
	if strings.TrimSpace(cfg.Project) == "" {
		return nil, fmt.Errorf("restart project is required")
	}
	if docker == nil {
		return nil, fmt.Errorf("docker restarter is required")
	}
	if cfg.RestartTimeout <= 0 {
		return nil, fmt.Errorf("restart timeout must be greater than zero")
	}
	allowed := make(map[string]struct{}, len(cfg.AllowedServices))
	for _, service := range cfg.AllowedServices {
		trimmed := strings.TrimSpace(service)
		if trimmed == "" {
			continue
		}
		allowed[trimmed] = struct{}{}
	}
	if len(allowed) == 0 {
		return nil, fmt.Errorf("allowed services list must not be empty")
	}
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		project:        strings.TrimSpace(cfg.Project),
		selfService:    strings.TrimSpace(cfg.SelfService),
		allowed:        allowed,
		delay:          cfg.Delay,
		restartTimeout: cfg.RestartTimeout,
		docker:         docker,
		scheduler:      afterFuncScheduler{},
		log:            log,
		now:            time.Now,
	}, nil
}

func (s *Service) ScheduleRestart(services []string) (RestartAcceptance, error) {
	normalized, err := s.normalizeServices(services)
	if err != nil {
		return RestartAcceptance{}, err
	}
	accepted := RestartAcceptance{
		Accepted:         true,
		Project:          s.project,
		Services:         normalized,
		ScheduledAt:      s.now().UTC().Format(time.RFC3339),
		DelaySeconds:     int(s.delay / time.Second),
		SuggestedCommand: suggestedCommand(normalized),
	}
	if len(normalized) == 0 {
		return accepted, nil
	}

	s.scheduler.Schedule(s.delay, func() {
		s.runRestart(normalized)
	})
	s.log.Info("scheduled compose restart", slog.String("project", s.project), slog.Any("services", normalized), slog.Int("delay_seconds", accepted.DelaySeconds))
	return accepted, nil
}

func (s *Service) normalizeServices(services []string) ([]string, error) {
	seen := make(map[string]struct{}, len(services))
	result := make([]string, 0, len(services))
	for _, service := range services {
		trimmed := strings.TrimSpace(service)
		if trimmed == "" {
			continue
		}
		if trimmed == s.selfService {
			return nil, fmt.Errorf("%w: %s", ErrSelfRestartDenied, trimmed)
		}
		if _, ok := s.allowed[trimmed]; !ok {
			return nil, fmt.Errorf("%w: %s", ErrServiceNotAllowed, trimmed)
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result, nil
}

func (s *Service) runRestart(services []string) {
	for _, service := range services {
		ctx, cancel := context.WithTimeout(context.Background(), s.restartTimeout+5*time.Second)
		err := s.docker.RestartService(ctx, s.project, service, s.restartTimeout)
		cancel()
		if err != nil {
			s.log.Error("failed to restart compose service", slog.String("project", s.project), slog.String("service", service), slog.String("error", err.Error()))
			continue
		}
		s.log.Info("restarted compose service", slog.String("project", s.project), slog.String("service", service))
	}
}

func suggestedCommand(services []string) string {
	if len(services) == 0 {
		return ""
	}
	return "docker compose -f deploy/docker-compose.yml restart " + strings.Join(services, " ")
}
