package restart

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"reflect"
	"testing"
	"time"
)

type fakeDockerRestarter struct {
	restarted []string
	errByName map[string]error
}

func (f *fakeDockerRestarter) RestartService(_ context.Context, _ string, service string, _ time.Duration) error {
	f.restarted = append(f.restarted, service)
	if err := f.errByName[service]; err != nil {
		return err
	}
	return nil
}

type fakeScheduler struct {
	delay time.Duration
	fn    func()
}

func (f *fakeScheduler) Schedule(delay time.Duration, fn func()) {
	f.delay = delay
	f.fn = fn
}

func TestServiceScheduleRestartDeduplicatesAndRunsAllowedServices(t *testing.T) {
	docker := &fakeDockerRestarter{}
	service, err := NewService(Config{
		Project:         "butler",
		AllowedServices: []string{"web", "orchestrator"},
		SelfService:     "restart-helper",
		Delay:           2 * time.Second,
		RestartTimeout:  15 * time.Second,
	}, docker, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	scheduler := &fakeScheduler{}
	service.scheduler = scheduler
	service.now = func() time.Time { return time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC) }

	accepted, err := service.ScheduleRestart([]string{"web", "orchestrator", "web"})
	if err != nil {
		t.Fatalf("schedule restart: %v", err)
	}
	if !accepted.Accepted {
		t.Fatal("expected accepted restart")
	}
	if !reflect.DeepEqual(accepted.Services, []string{"web", "orchestrator"}) {
		t.Fatalf("unexpected services: %+v", accepted.Services)
	}
	if scheduler.fn == nil {
		t.Fatal("expected scheduled function")
	}
	if scheduler.delay != 2*time.Second {
		t.Fatalf("unexpected delay: %s", scheduler.delay)
	}

	scheduler.fn()
	if !reflect.DeepEqual(docker.restarted, []string{"web", "orchestrator"}) {
		t.Fatalf("unexpected restarted services: %+v", docker.restarted)
	}
}

func TestServiceScheduleRestartRejectsDisallowedService(t *testing.T) {
	service, err := NewService(Config{
		Project:         "butler",
		AllowedServices: []string{"web"},
		SelfService:     "restart-helper",
		Delay:           time.Second,
		RestartTimeout:  10 * time.Second,
	}, &fakeDockerRestarter{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = service.ScheduleRestart([]string{"tool-broker"})
	if !errors.Is(err, ErrServiceNotAllowed) {
		t.Fatalf("expected ErrServiceNotAllowed, got %v", err)
	}
}

func TestServiceScheduleRestartRejectsSelfService(t *testing.T) {
	service, err := NewService(Config{
		Project:         "butler",
		AllowedServices: []string{"restart-helper", "web"},
		SelfService:     "restart-helper",
		Delay:           time.Second,
		RestartTimeout:  10 * time.Second,
	}, &fakeDockerRestarter{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = service.ScheduleRestart([]string{"restart-helper"})
	if !errors.Is(err, ErrSelfRestartDenied) {
		t.Fatalf("expected ErrSelfRestartDenied, got %v", err)
	}
}
