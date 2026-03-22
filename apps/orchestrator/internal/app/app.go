package app

import (
	"log/slog"
	"net"
	"net/http"

	telegramadapter "github.com/butler/butler/apps/orchestrator/internal/channel/telegram"
	"github.com/butler/butler/apps/orchestrator/internal/tools"
	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/health"
	"github.com/butler/butler/internal/memory/pipeline"
	"github.com/butler/butler/internal/metrics"
	postgresstore "github.com/butler/butler/internal/storage/postgres"
	redisstore "github.com/butler/butler/internal/storage/redis"
	"google.golang.org/grpc"
)

type App struct {
	config         config.OrchestratorConfig
	log            *slog.Logger
	metrics        *metrics.Registry
	health         *health.Service
	postgres       *postgresstore.Store
	redis          *redisstore.Store
	httpServer     *http.Server
	grpcServer     *grpc.Server
	grpcListen     net.Listener
	telegram       *telegramadapter.Adapter
	toolBroker     *tools.BrokerClient
	pipelineWorker *pipeline.Worker
}
