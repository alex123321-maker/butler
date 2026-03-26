package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	activity "github.com/butler/butler/apps/orchestrator/internal/activity"
	apiservice "github.com/butler/butler/apps/orchestrator/internal/api"
	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	artifacts "github.com/butler/butler/apps/orchestrator/internal/artifacts"
	deliveryevents "github.com/butler/butler/apps/orchestrator/internal/deliveryevents"
	"github.com/butler/butler/apps/orchestrator/internal/observability"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/apps/orchestrator/internal/restarthelper"
	runservice "github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	singletab "github.com/butler/butler/apps/orchestrator/internal/singletab"
	"github.com/butler/butler/apps/orchestrator/internal/tools"
	"github.com/butler/butler/internal/config"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/health"
	"github.com/butler/butler/internal/logger"
	"github.com/butler/butler/internal/memory/chunks"
	memorydoctor "github.com/butler/butler/internal/memory/doctor"
	"github.com/butler/butler/internal/memory/episodic"
	"github.com/butler/butler/internal/memory/pipeline"
	"github.com/butler/butler/internal/memory/profile"
	"github.com/butler/butler/internal/memory/provenance"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/memory/working"
	"github.com/butler/butler/internal/metrics"
	"github.com/butler/butler/internal/modelprovider"
	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/providerfactory"
	postgresstore "github.com/butler/butler/internal/storage/postgres"
)

type httpServerDeps struct {
	cfg                config.OrchestratorConfig
	log                *slog.Logger
	settingsStore      *config.PostgresSettingsStore
	hotConfig          *config.HotConfig
	postgres           *postgresstore.Store
	metrics            *metrics.Registry
	health             *health.Service
	toolBrokerClient   *tools.BrokerClient
	authManager        *providerauth.Manager
	providerResult     providerfactory.Result
	executor           *flow.Service
	sessionRepo        *session.PostgresRepository
	runRepo            *runservice.PostgresRepository
	transcriptStore    *transcript.Store
	transitionRepo     *runservice.PostgresTransitionRepository
	eventHub           *observability.Hub
	approvalRepo       *approvals.PostgresRepository
	approvalService    *approvals.Service
	singleTabService   *singletab.Service
	artifactsRepo      *artifacts.PostgresRepository
	artifactsService   *artifacts.Service
	activityRepo       *activity.PostgresRepository
	deliveryEventsRepo *deliveryevents.PostgresRepository
	pipelineQueue      *pipeline.Queue
	pipelineWorker     *pipeline.Worker
	embeddingProvider  pipeline.EmbeddingProvider
	profileStore       *profile.Store
	episodicStore      *episodic.Store
	chunkStore         *chunks.Store
}

func newHTTPServer(deps httpServerDeps) *http.Server {
	apiServer := apiservice.NewServer(deps.executor, logger.WithComponent(deps.log, "api"))
	viewServer := apiservice.NewViewServer(deps.sessionRepo, deps.runRepo, deps.transcriptStore, logger.WithComponent(deps.log, "view-api"))
	doctorChecker := apiservice.NewToolBrokerDoctorChecker(func(ctx context.Context, toolName, argsJSON string) (string, error) {
		result, err := deps.toolBrokerClient.ExecuteToolCall(ctx, &toolbrokerv1.ToolCall{
			ToolCallId: fmt.Sprintf("doctor-%d", time.Now().UnixNano()),
			ToolName:   toolName,
			ArgsJson:   argsJSON,
			Status:     "pending",
			StartedAt:  time.Now().UTC().Format(time.RFC3339Nano),
		})
		if err != nil {
			return "", err
		}
		return result.GetResultJson(), nil
	})
	memoryDoctor := memorydoctor.NewReporter(deps.pipelineQueue, deps.postgres)
	memoryDoctor.SetPipelineEnabled(deps.pipelineWorker != nil)
	memoryDoctor.SetEmbeddingConfigured(deps.embeddingProvider != nil)
	doctorServer := apiservice.NewDoctorServer(deps.postgres.Pool(), doctorChecker, memoryDoctor, deps.artifactsService, logger.WithComponent(deps.log, "doctor-api"))
	settingsService := config.NewSettingsService(deps.settingsStore, deps.hotConfig)
	settingsServer := apiservice.NewSettingsServer(settingsService, restarthelper.NewClient(deps.cfg.RestartHelperURL))
	promptServer := apiservice.NewPromptServer(deps.executor)
	memoryServer := apiservice.NewMemoryServer(working.NewStore(deps.postgres.Pool()), deps.profileStore, deps.episodicStore, deps.chunkStore, provenance.NewStore(deps.postgres.Pool()))
	memoryServer.SetWriters(deps.profileStore, deps.episodicStore, deps.chunkStore)
	providerServer := apiservice.NewProviderServer(deps.authManager, deps.providerResult.ProviderName, map[string]string{
		modelprovider.ProviderOpenAI:        deps.cfg.OpenAIModel,
		modelprovider.ProviderOpenAICodex:   deps.cfg.OpenAICodexModel,
		modelprovider.ProviderGitHubCopilot: deps.cfg.GitHubCopilotModel,
	}, strings.TrimSpace(deps.cfg.OpenAIAPIKey) != "")
	eventServer := apiservice.NewEventServer(deps.transitionRepo, deps.runRepo, deps.eventHub, logger.WithComponent(deps.log, "event-api"))
	taskViewServer := apiservice.NewTaskViewServer(deps.runRepo, deps.runRepo, deps.sessionRepo, deps.transcriptStore, deps.transitionRepo, deps.artifactsRepo, deps.deliveryEventsRepo)
	overviewServer := apiservice.NewOverviewServer(deps.runRepo, deps.deliveryEventsRepo)
	approvalsServer := apiservice.NewApprovalsServer(deps.approvalRepo, deps.approvalService, deps.singleTabService)
	singleTabServer := apiservice.NewSingleTabServer(deps.singleTabService)
	singleTabServer.SetRelayHeartbeatTTL(time.Duration(deps.cfg.SingleTabRelayHeartbeatTTLSeconds) * time.Second)
	extensionAuth := apiservice.NewExtensionAuthMiddleware(deps.cfg.ExtensionAPITokens)
	artifactsServer := apiservice.NewArtifactsServer(deps.artifactsRepo, deps.activityRepo, deps.artifactsService)
	activityServer := apiservice.NewActivityServer(deps.activityRepo)
	systemServer := apiservice.NewSystemServer(
		deps.postgres.Pool(),
		deps.runRepo,
		deps.approvalRepo,
		deps.providerResult.ProviderName,
		strings.TrimSpace(deps.cfg.OpenAIAPIKey) != "",
		deps.pipelineWorker != nil,
		normalizeSingleTabTransportMode(deps.cfg.SingleTabTransportMode),
		len(deps.cfg.ExtensionAPITokens) > 0,
		deps.cfg.SingleTabRelayHeartbeatTTLSeconds,
	)
	taskDebugServer := apiservice.NewTasksDebugServer(deps.runRepo, deps.transcriptStore)
	streamServer := apiservice.NewStreamServer(deps.eventHub, taskViewServer, overviewServer, approvalsServer, systemServer, activityServer)
	singleTabTransportMode := normalizeSingleTabTransportMode(deps.cfg.SingleTabTransportMode)
	remoteRelayEnabled := singleTabTransportMode != "native_only"

	mux := http.NewServeMux()
	mux.Handle("/health", deps.health.Handler())
	mux.Handle("/metrics", deps.metrics.Handler())
	mux.Handle("/api/v1/events", apiServer.HTTPHandler())
	mux.Handle("/api/v1/settings/restart", settingsServer.HandleRestart())
	mux.Handle("/api/v1/settings", settingsServer.HandleList())
	mux.Handle("/api/v1/settings/", settingsServer.HandleItem())
	mux.Handle("/api/v1/prompts/system", promptServer.HandleConfig())
	mux.Handle("/api/v1/prompts/system/preview", promptServer.HandlePreview())
	mux.Handle("/api/v1/providers", providerServer.HandleList())
	mux.Handle("/api/v1/providers/", providerServer.HandleItem())
	mux.Handle("/api/v1/memory", memoryServer.HandleList())
	mux.Handle("/api/v2/memory", memoryServer.HandleList())
	mux.Handle("/api/v2/memory/item", memoryServer.HandleGet())
	mux.Handle("/api/v2/memory/patch", memoryServer.HandlePatch())
	mux.Handle("/api/v2/memory/delete", memoryServer.HandleDelete())
	mux.Handle("/api/v2/memory/confirm", memoryServer.HandleConfirm())
	mux.Handle("/api/v2/memory/reject", memoryServer.HandleReject())
	mux.Handle("/api/v2/memory/suppress", memoryServer.HandleSuppress())
	mux.Handle("/api/v2/memory/unsuppress", memoryServer.HandleUnsuppress())
	mux.Handle("/api/tasks", taskViewServer.HandleListTasks())
	mux.Handle("/api/tasks/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/debug") {
			taskDebugServer.HandleGetTaskDebug("/api/tasks/").ServeHTTP(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/artifacts") {
			artifactsServer.HandleListTaskArtifacts("/api/tasks/").ServeHTTP(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/activity") {
			artifactsServer.HandleListTaskActivity("/api/tasks/").ServeHTTP(w, r)
			return
		}
		taskViewServer.HandleGetTaskDetail("/api/tasks/").ServeHTTP(w, r)
	}))
	mux.Handle("/api/overview", overviewServer.HandleGetOverview())
	mux.Handle("/api/approvals", approvalsServer.HandleListApprovals())
	mux.Handle("/api/approvals/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/select-tab") {
			approvalsServer.HandleSelectTab("/api/approvals/").ServeHTTP(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/approve") {
			approvalsServer.HandleApprove("/api/approvals/").ServeHTTP(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/reject") {
			approvalsServer.HandleReject("/api/approvals/").ServeHTTP(w, r)
			return
		}
		approvalsServer.HandleGetApproval("/api/approvals/").ServeHTTP(w, r)
	}))
	mux.Handle("/api/v2/tasks", taskViewServer.HandleListTasks())
	mux.Handle("/api/v2/tasks/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/debug") {
			taskDebugServer.HandleGetTaskDebug("/api/v2/tasks/").ServeHTTP(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/artifacts") {
			artifactsServer.HandleListTaskArtifacts("/api/v2/tasks/").ServeHTTP(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/activity") {
			artifactsServer.HandleListTaskActivity("/api/v2/tasks/").ServeHTTP(w, r)
			return
		}
		taskViewServer.HandleGetTaskDetail("/api/v2/tasks/").ServeHTTP(w, r)
	}))
	mux.Handle("/api/v2/overview", overviewServer.HandleGetOverview())
	mux.Handle("/api/v2/approvals", approvalsServer.HandleListApprovals())
	mux.Handle("/api/v2/single-tab/bind-requests", singleTabServer.HandleCreateBindRequest())
	mux.Handle("/api/v2/single-tab/session", singleTabServer.HandleGetActiveSession())
	mux.Handle("/api/v2/single-tab/extension-instances", systemServer.HandleListExtensionInstances())
	if remoteRelayEnabled {
		mux.Handle("/api/v2/single-tab/actions/dispatch", singleTabServer.HandleRelayDispatchAction())
		mux.Handle("/api/v2/extension/single-tab/bind-requests", extensionAuth(singleTabServer.HandleCreateBindRequest()))
		mux.Handle("/api/v2/extension/single-tab/bind-requests/next", extensionAuth(singleTabServer.HandleExtensionPollNextBindRequest()))
		mux.Handle("/api/v2/extension/single-tab/bind-requests/", extensionAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/result") {
				singleTabServer.HandleExtensionResolveBindRequest("/api/v2/extension/single-tab/bind-requests/").ServeHTTP(w, r)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		})))
		mux.Handle("/api/v2/extension/single-tab/session", extensionAuth(singleTabServer.HandleGetActiveSession()))
		mux.Handle("/api/v2/extension/single-tab/actions/next", extensionAuth(singleTabServer.HandleExtensionPollNextAction()))
		mux.Handle("/api/v2/extension/single-tab/actions/", extensionAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/result") {
				singleTabServer.HandleExtensionResolveAction("/api/v2/extension/single-tab/actions/").ServeHTTP(w, r)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		})))
	}
	mux.Handle("/api/v2/single-tab/session/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/state") {
			singleTabServer.HandleUpdateSessionState("/api/v2/single-tab/session/").ServeHTTP(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/release") {
			singleTabServer.HandleReleaseSession("/api/v2/single-tab/session/").ServeHTTP(w, r)
			return
		}
		singleTabServer.HandleGetSession("/api/v2/single-tab/session/").ServeHTTP(w, r)
	}))
	mux.Handle("/api/v2/approvals/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/select-tab") {
			approvalsServer.HandleSelectTab("/api/v2/approvals/").ServeHTTP(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/approve") {
			approvalsServer.HandleApprove("/api/v2/approvals/").ServeHTTP(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/reject") {
			approvalsServer.HandleReject("/api/v2/approvals/").ServeHTTP(w, r)
			return
		}
		approvalsServer.HandleGetApproval("/api/v2/approvals/").ServeHTTP(w, r)
	}))
	mux.Handle("/api/v2/artifacts", artifactsServer.HandleListArtifacts())
	mux.Handle("/api/v2/artifacts/browser-captures", artifactsServer.HandleCreateBrowserCapture())
	mux.Handle("/api/v2/artifacts/", artifactsServer.HandleGetArtifact("/api/v2/artifacts/"))
	mux.Handle("/api/v2/activity", activityServer.HandleListActivity())
	mux.Handle("/api/single-tab/bind-requests", singleTabServer.HandleCreateBindRequest())
	mux.Handle("/api/single-tab/session", singleTabServer.HandleGetActiveSession())
	mux.Handle("/api/single-tab/extension-instances", systemServer.HandleListExtensionInstances())
	mux.Handle("/api/single-tab/session/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/state") {
			singleTabServer.HandleUpdateSessionState("/api/single-tab/session/").ServeHTTP(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/release") {
			singleTabServer.HandleReleaseSession("/api/single-tab/session/").ServeHTTP(w, r)
			return
		}
		singleTabServer.HandleGetSession("/api/single-tab/session/").ServeHTTP(w, r)
	}))
	mux.Handle("/api/v2/system", systemServer.HandleGetSystem())
	mux.Handle("/api/system", systemServer.HandleGetSystem())
	mux.Handle("/api/v2/stream", streamServer.HandleStream())
	mux.Handle("/api/stream", streamServer.HandleStream())
	mux.Handle("/api/v1/sessions", viewServer.HandleListSessions())
	mux.Handle("/api/v1/sessions/", viewServer.HandleGetSession())
	mux.Handle("/api/v1/runs/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/transcript") {
			viewServer.HandleGetRunTranscript().ServeHTTP(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/events") {
			eventServer.HandleSSE().ServeHTTP(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/transitions") {
			eventServer.HandleListTransitions().ServeHTTP(w, r)
			return
		}
		viewServer.HandleGetRun().ServeHTTP(w, r)
	}))
	mux.Handle("/api/v1/doctor/check", doctorServer.HandleRunCheck())
	mux.Handle("/api/v1/doctor/reports", doctorServer.HandleListReports())
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"service": deps.cfg.Shared.ServiceName,
			"status":  "starting",
		})
	})

	return &http.Server{
		Addr:              deps.cfg.Shared.HTTPAddr,
		Handler:           instrumentHTTP(deps.cfg.Shared.ServiceName, deps.metrics, corsMiddleware(mux)),
		ReadHeaderTimeout: 5 * time.Second,
	}
}

func normalizeSingleTabTransportMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "native_only", "remote_preferred":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "dual"
	}
}
