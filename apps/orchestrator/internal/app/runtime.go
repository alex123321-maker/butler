package app

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
)

func (a *App) Run(ctx context.Context) error {
	errCh := make(chan error, 5)
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	completionDone := make(chan struct{})
	go func() {
		defer close(completionDone)
		done := make(chan struct{}, 3)
		workers := 0
		if a.outboxWorker != nil {
			workers++
			go func() { _ = a.outboxWorker.Run(runCtx); done <- struct{}{} }()
		}
		if a.auxiliaryWorker != nil {
			workers++
			go func() { _ = a.auxiliaryWorker.Run(runCtx); done <- struct{}{} }()
		}
		if a.initiativeDeliveryWorker != nil {
			workers++
			go func() { _ = a.initiativeDeliveryWorker.Run(runCtx); done <- struct{}{} }()
		}
		for range workers {
			<-done
		}
	}()
	inputDone := make(chan struct{})
	if a.inputWorker != nil {
		go func() {
			defer close(inputDone)
			if err := a.inputWorker.Run(runCtx); err != nil && runCtx.Err() == nil {
				errCh <- err
			}
		}()
	} else {
		close(inputDone)
	}

	go func() {
		a.log.Info("starting orchestrator http server", slog.String("http_addr", a.config.Shared.HTTPAddr))
		if err := a.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	go func() {
		a.log.Info("starting orchestrator grpc server", slog.String("grpc_addr", a.config.Shared.GRPCAddr))
		if err := a.grpcServer.Serve(a.grpcListen); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	if a.telegram != nil {
		go func() {
			a.log.Info("starting telegram adapter")
			if err := a.telegram.Run(runCtx); err != nil {
				errCh <- err
				return
			}
			errCh <- nil
		}()
	}

	if a.pipelineWorker != nil {
		go func() {
			a.log.Info("starting memory pipeline worker")
			if err := a.pipelineWorker.Run(runCtx); err != nil && runCtx.Err() == nil {
				a.log.Error("memory pipeline worker stopped with error", slog.String("error", err.Error()))
				errCh <- err
				return
			}
			a.log.Info("memory pipeline worker stopped")
		}()
	}

	sigCtx, stop := signal.NotifyContext(runCtx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-errCh:
		cancel()
		<-inputDone
		<-completionDone
		return a.shutdown(err)
	case <-sigCtx.Done():
		cancel()
		<-inputDone
		<-completionDone
		a.log.Info("received shutdown signal")
		return a.shutdown(nil)
	}
}

func (a *App) shutdown(runErr error) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := a.httpServer.Shutdown(ctx); err != nil {
		a.log.Error("http shutdown failed", slog.String("error", err.Error()))
		if runErr == nil {
			runErr = err
		}
	}

	a.grpcServer.GracefulStop()
	if a.grpcListen != nil {
		if err := a.grpcListen.Close(); err != nil && !errors.Is(err, net.ErrClosed) && runErr == nil {
			runErr = err
		}
	}

	if err := a.redis.Close(); err != nil && runErr == nil {
		runErr = err
	}
	if err := a.toolBroker.Close(); err != nil && runErr == nil {
		runErr = err
	}
	a.postgres.Close()

	if runErr != nil {
		a.log.Error("orchestrator stopped with error", slog.String("error", runErr.Error()))
		return runErr
	}

	a.log.Info("orchestrator stopped cleanly")
	return nil
}
