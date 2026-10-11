package app

import (
	"context"
	"fmt"
	"github.com/butler/butler/internal/budget"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	activity "github.com/butler/butler/apps/orchestrator/internal/activity"
	apiservice "github.com/butler/butler/apps/orchestrator/internal/api"
	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	artifacts "github.com/butler/butler/apps/orchestrator/internal/artifacts"
	"github.com/butler/butler/apps/orchestrator/internal/completion"
	deliveryevents "github.com/butler/butler/apps/orchestrator/internal/deliveryevents"
	"github.com/butler/butler/apps/orchestrator/internal/goal"
	"github.com/butler/butler/apps/orchestrator/internal/initiative"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/observability"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/apps/orchestrator/internal/restarthelper"
	runservice "github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	singletab "github.com/butler/butler/apps/orchestrator/internal/singletab"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/apps/orchestrator/internal/tools"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/extensions"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/health"
	"github.com/butler/butler/internal/logger"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/chunks"
	memorydoctor "github.com/butler/butler/internal/memory/doctor"
	"github.com/butler/butler/internal/memory/episodic"
	"github.com/butler/butler/internal/memory/indexing"
	"github.com/butler/butler/internal/memory/ingestion"
	"github.com/butler/butler/internal/memory/pipeline"
	"github.com/butler/butler/internal/memory/profile"
	"github.com/butler/butler/internal/memory/provenance"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/memory/working"
	"github.com/butler/butler/internal/metrics"
	"github.com/butler/butler/internal/modelprovider"
	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/providerfactory"
	"github.com/butler/butler/internal/skills"
	postgresstore "github.com/butler/butler/internal/storage/postgres"
)

type httpServerDeps struct {
	initiativeStore        *initiative.Store
	memoryIndexing         *indexing.Runtime
	sourceCatalog          *catalog.Store
	ownerGoals             *goal.Manager
	finalResults           *completion.Coordinator
	actionAuthorityHandler http.Handler
	skillEvidenceHandler   http.Handler
	skillClient            *skills.Client
	cfg                    config.OrchestratorConfig
	log                    *slog.Logger
	settingsStore          *config.PostgresSettingsStore
	hotConfig              *config.HotConfig
	postgres               *postgresstore.Store
	metrics                *metrics.Registry
	health                 *health.Service
	toolBrokerClient       *tools.BrokerClient
	authManager            *providerauth.Manager
	providerResult         providerfactory.Result
	executor               *flow.Service
	sessionRepo            *session.PostgresRepository
	runRepo                *runservice.PostgresRepository
	taskStore              *task.Store
	taskControls           *task.Controller
	transcriptStore        *transcript.Store
	transitionRepo         *runservice.PostgresTransitionRepository
	eventHub               *observability.Hub
	approvalRepo           *approvals.PostgresRepository
	approvalService        *approvals.Service
	singleTabService       *singletab.Service
	artifactsRepo          *artifacts.PostgresRepository
	artifactsService       *artifacts.Service
	activityRepo           *activity.PostgresRepository
	deliveryEventsRepo     *deliveryevents.PostgresRepository
	pipelineQueue          *pipeline.Queue
	pipelineWorker         *pipeline.Worker
	embeddingProvider      pipeline.EmbeddingProvider
	profileStore           *profile.Store
	episodicStore          *episodic.Store
	chunkStore             *chunks.Store
	inputWorker            *durableinput.Service
	inputStore             *durableinput.Store
	ownerAuth              *apiservice.OwnerControlAuth
}

func newHTTPServer(deps httpServerDeps) *http.Server {
	initiativeServer := apiservice.NewInitiativeServer(deps.initiativeStore, deps.ownerAuth)
	goalServer := apiservice.NewGoalServer(deps.ownerGoals, deps.ownerAuth)
	resultServer := apiservice.NewResultServer(deps.finalResults)
	apiServer := apiservice.NewServer(deps.inputWorker, logger.WithComponent(deps.log, "api"))
	apiServer.SetOwnerControlAuth(deps.ownerAuth)
	inputServer := apiservice.NewInputServer(deps.inputStore)
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
	promptServer.SetOwnerControlAuth(deps.ownerAuth)
	memoryServer := apiservice.NewMemoryServer(working.NewStore(deps.postgres.Pool()), deps.profileStore, deps.episodicStore, deps.chunkStore, provenance.NewStore(deps.postgres.Pool()))
	memoryServer.SetWriters(deps.profileStore, deps.episodicStore, deps.chunkStore)
	parser := ingestion.Parser{}
	if strings.TrimSpace(os.Getenv("BUTLER_MEMORY_PDF_PARSER_ENABLED")) == "true" {
		parser.Pages = ingestion.NewPDFClient()
	}
	memoryCatalogServer := &apiservice.MemoryCatalogServer{Store: deps.sourceCatalog, Auth: deps.ownerAuth, Ingestion: &ingestion.Store{Catalog: deps.sourceCatalog, Parser: parser}, Indexing: deps.memoryIndexing}
	providerServer := apiservice.NewProviderServer(deps.authManager, deps.providerResult.ProviderName, map[string]string{
		modelprovider.ProviderOpenAI:        deps.cfg.OpenAIModel,
		modelprovider.ProviderOpenAICodex:   deps.cfg.OpenAICodexModel,
		modelprovider.ProviderGitHubCopilot: deps.cfg.GitHubCopilotModel,
		modelprovider.ProviderAntigravity:   deps.cfg.AntigravityModel,
	}, strings.TrimSpace(deps.cfg.OpenAIAPIKey) != "")
	eventServer := apiservice.NewEventServer(deps.transitionRepo, deps.runRepo, deps.eventHub, logger.WithComponent(deps.log, "event-api"))
	taskViewServer := apiservice.NewTaskViewServer(deps.runRepo, deps.runRepo, deps.sessionRepo, deps.transcriptStore, deps.transitionRepo, deps.artifactsRepo, deps.deliveryEventsRepo)
	taskViewServer.SetDurableTasks(deps.taskStore)
	if deps.finalResults != nil {
		taskViewServer.SetFinalResults(deps.finalResults, deps.finalResults.Outbox)
	}
	taskControlServer := apiservice.NewTaskControlServer(deps.taskStore)
	taskControlServer.SetOwnerControlAuth(deps.ownerAuth)
	taskControlServer.SetControls(deps.taskControls)
	overviewServer := apiservice.NewOverviewServer(deps.runRepo, deps.deliveryEventsRepo)
	if deps.finalResults != nil {
		overviewServer.SetOutbox(deps.finalResults.Outbox)
	}
	approvalsServer := apiservice.NewApprovalsServer(deps.approvalRepo, deps.approvalService, deps.singleTabService)
	approvalsServer.SetOwnerControlAuth(deps.ownerAuth)
	singleTabServer := apiservice.NewSingleTabServer(deps.singleTabService)
	singleTabServer.SetRelayHeartbeatTTL(time.Duration(deps.cfg.SingleTabRelayHeartbeatTTLSeconds) * time.Second)
	extensionAuth := apiservice.NewExtensionAuthMiddleware(deps.cfg.ExtensionAPITokens)
	artifactsServer := apiservice.NewArtifactsServer(deps.artifactsRepo, deps.activityRepo, deps.artifactsService)
	artifactsServer.SetDurableTasks(deps.taskStore)
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
	taskDebugServer.SetDurableTasks(deps.taskStore)
	streamServer := apiservice.NewStreamServer(deps.eventHub, taskViewServer, overviewServer, approvalsServer, systemServer, activityServer)
	singleTabTransportMode := normalizeSingleTabTransportMode(deps.cfg.SingleTabTransportMode)
	remoteRelayEnabled := singleTabTransportMode != "native_only"

	mux := http.NewServeMux()
	mux.Handle("/api/v3/execution-budgets", apiservice.NewBudgetHandler(budget.NewLedger(deps.postgres.Pool()), deps.ownerAuth))
	var extensionClient *extensions.ControlClient
	if deps.cfg.ExtensionControlURL != "" {
		token, tokenErr := actionauth.ReadServiceToken(deps.cfg.ActionAuthorityTokenFile)
		if tokenErr == nil {
			extensionClient = extensions.NewControlClient(deps.cfg.ExtensionControlURL, token)
		} else {
			deps.log.Warn("reviewed extension control disabled: service authentication unavailable")
		}
	}
	mux.Handle("/api/v4/extensions", apiservice.ReviewedExtensions(deps.ownerAuth, extensionClient))
	mux.Handle("/api/v4/skills", apiservice.ReviewedSkills(deps.ownerAuth, deps.skillClient))
	if deps.skillEvidenceHandler != nil {
		mux.Handle("/internal/skill-evidence", deps.skillEvidenceHandler)
	}
	mux.Handle("/internal/diagnostics", coreDiagnostics(deps))
	if deps.actionAuthorityHandler != nil {
		mux.Handle("/internal/action-context", deps.actionAuthorityHandler)
	}
	mux.Handle("/health", deps.health.Handler())
	mux.Handle("/metrics", deps.metrics.Handler())
	mux.Handle("/api/v1/events", apiServer.HTTPHandler())
	mux.Handle("/api/v3/inputs/", inputServer.HandleItem())
	mux.Handle("/api/v3/owner-authority", goalServer.Handle())
	mux.Handle("/api/v3/initiatives", initiativeServer.Handle())
	mux.Handle("/api/v3/initiatives/", initiativeServer.Handle())
	mux.Handle("/api/v3/owner-goals", goalServer.Handle())
	mux.Handle("/api/v3/owner-mandates", goalServer.Handle())
	mux.Handle("/api/v3/owner-goals/tasks", goalServer.Handle())
	mux.Handle("/api/v3/tasks", taskControlServer.HandleCollection())
	mux.Handle("/api/v3/tasks/", taskControlServer.HandleItem("/api/v3/tasks/"))
	mux.Handle("/api/v1/settings/restart", apiservice.OwnerMaintenance(deps.ownerAuth, settingsServer.HandleRestart()))
	mux.Handle("/api/v1/settings", settingsServer.HandleList())
	mux.Handle("/api/v1/settings/", apiservice.OwnerMaintenance(deps.ownerAuth, settingsServer.HandleItem()))
	mux.Handle("/api/v1/prompts/system", promptServer.HandleConfig())
	mux.Handle("/api/v1/prompts/system/preview", promptServer.HandlePreview())
	mux.Handle("/api/v1/providers", providerServer.HandleList())
	mux.Handle("/api/v2/results/", resultServer.HandleByRun("/api/v2/results/"))
	mux.Handle("/api/v1/providers/", apiservice.OwnerMaintenance(deps.ownerAuth, providerServer.HandleItem()))
	mux.Handle("/api/v4/memory", memoryCatalogServer.Handle())
	mux.Handle("/api/v4/memory/", memoryCatalogServer.Handle())
	mux.Handle("/api/v1/memory", memoryCatalogServer.LegacyRead(memoryServer.HandleList()))
	mux.Handle("/api/v2/memory", memoryCatalogServer.LegacyRead(memoryServer.HandleList()))
	mux.Handle("/api/v2/memory/item", memoryCatalogServer.LegacyRead(memoryServer.HandleGet()))
	mux.Handle("/api/v2/memory/patch", memoryCatalogServer.LegacyRead(memoryServer.HandlePatch()))
	mux.Handle("/api/v2/memory/delete", memoryCatalogServer.LegacyRead(memoryServer.HandleDelete()))
	mux.Handle("/api/v2/memory/confirm", memoryCatalogServer.LegacyRead(memoryServer.HandleConfirm()))
	mux.Handle("/api/v2/memory/reject", memoryCatalogServer.LegacyRead(memoryServer.HandleReject()))
	mux.Handle("/api/v2/memory/suppress", memoryCatalogServer.LegacyRead(memoryServer.HandleSuppress()))
	mux.Handle("/api/v2/memory/unsuppress", memoryCatalogServer.LegacyRead(memoryServer.HandleUnsuppress()))
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
