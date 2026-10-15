package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	artifacts "github.com/butler/butler/apps/orchestrator/internal/artifacts"
	"github.com/butler/butler/internal/logger"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DoctorReport represents a stored doctor report.
type DoctorReport struct {
	ID         int64           `json:"id"`
	Status     string          `json:"status"`
	CheckedAt  time.Time       `json:"checked_at"`
	ReportJSON json.RawMessage `json:"report"`
}

// DoctorChecker runs a system check and returns the raw report JSON.
type DoctorChecker interface {
	RunCheck(ctx context.Context) (status string, reportJSON json.RawMessage, err error)
}

type MemoryDoctorReporter interface {
	Report(ctx context.Context) (map[string]any, error)
}

// DoctorServer serves REST endpoints for doctor reports.
type DoctorServer struct {
	ownerAuth *OwnerControlAuth
	pool      *pgxpool.Pool
	checker   DoctorChecker
	memory    MemoryDoctorReporter
	artifacts *artifacts.Service
	log       *slog.Logger
}

func (d *DoctorServer) SetOwnerControlAuth(auth *OwnerControlAuth) { d.ownerAuth = auth }

// NewDoctorServer creates a new DoctorServer.
func NewDoctorServer(pool *pgxpool.Pool, checker DoctorChecker, memory MemoryDoctorReporter, artifactsService *artifacts.Service, log *slog.Logger) *DoctorServer {
	if log == nil {
		log = slog.Default()
	}
	return &DoctorServer{
		pool:      pool,
		checker:   checker,
		memory:    memory,
		artifacts: artifactsService,
		log:       logger.WithComponent(log, "doctor-api"),
	}
}

// HandleRunCheck handles POST /api/v1/doctor/check
// Triggers a doctor check via the tool broker, stores the report, and returns it.
func (d *DoctorServer) HandleRunCheck() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		actor, ok := d.ownerAuth.Authenticate(r)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "owner authorization required"})
			return
		}
		ctx := owneridentity.WithActor(r.Context(), actor)
		var request struct {
			InputID string `json:"input_id"`
		}
		if r.ContentLength != 0 {
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&request); err != nil {
				writeJSON(w, 400, map[string]string{"error": "invalid diagnostic request"})
				return
			}
		}
		var status string
		var reportJSON json.RawMessage
		var err error
		if request.InputID != "" {
			if reader, ok := d.checker.(interface {
				ReadCheck(context.Context, string) (string, json.RawMessage, error)
			}); ok {
				status, reportJSON, err = reader.ReadCheck(ctx, request.InputID)
			} else {
				err = fmt.Errorf("diagnostic reader unavailable")
			}
		} else if d.checker != nil {
			status, reportJSON, err = d.checker.RunCheck(ctx)
		} else {
			err = fmt.Errorf("checker unavailable")
		}
		combined := map[string]any{}
		if json.Unmarshal(reportJSON, &combined) != nil || combined == nil {
			combined = map[string]any{}
			status = "unknown"
		}
		if err != nil {
			d.log.Warn("doctor check unavailable")
			status = "unknown"
			combined["check_error"] = "execution_unavailable"
		}

		if d.memory != nil {
			memoryReport, memErr := d.memory.Report(ctx)
			if memoryReport == nil {
				memoryReport = map[string]any{"status": "unknown", "reason": "report_missing"}
			}
			if memErr != nil {
				memoryReport["status"] = "unknown"
				memoryReport["reason"] = "reporter_failed"
			}
			combined["memory"] = memoryReport
			memoryStatus, _ := memoryReport["status"].(string)
			status = combineDoctorStatus(status, memoryStatus)
		} else {
			combined["memory"] = map[string]any{"status": "unknown", "reason": "not_observed"}
			status = combineDoctorStatus(status, "unknown")
		}
		combined["status"] = status
		combined["owner_id"] = actor.OwnerID
		if combined["checked_at"] == nil {
			combined["checked_at"] = time.Now().UTC().Format(time.RFC3339Nano)
		}
		reportJSON, _ = json.Marshal(combined)
		if execution, ok := combined["execution"].(map[string]any); ok {
			if s, _ := execution["status"].(string); s == "queued" || s == "running" || s == "waiting_for_user" {
				// Retain the receipt before navigating to Approvals. Polling may
				// advance this pending snapshot; a final report is immutable.
				report, storeErr := d.storeReport(ctx, status, reportJSON)
				if storeErr != nil {
					report = DoctorReport{Status: status, CheckedAt: time.Now().UTC(), ReportJSON: reportJSON}
				}
				writeJSON(w, http.StatusAccepted, toDoctorReportDTO(report))
				return
			}
		}

		if d.artifacts != nil {
			if execution, ok := combined["execution"].(map[string]any); ok {
				runID, _ := execution["run_id"].(string)
				sessionKey, _ := execution["session_key"].(string)
				if runID != "" && sessionKey != "" {
					_, _ = d.artifacts.SaveDoctorReport(ctx, runID, sessionKey, status, string(reportJSON), time.Now().UTC())
				}
			}
		}

		// Store the report
		report, err := d.storeReport(ctx, status, reportJSON)
		if err != nil {
			d.log.Error("failed to store doctor report")
			// Still return the report even if storage fails
			writeJSON(w, http.StatusOK, map[string]any{
				"id": 0, "checked_at": time.Now().UTC().Format(time.RFC3339Nano),
				"report": json.RawMessage(reportJSON),
				"status": status,
				"stored": false,
			})
			return
		}

		writeJSON(w, http.StatusOK, toDoctorReportDTO(report))
	})
}

// HandleListReports handles GET /api/v1/doctor/reports
func (d *DoctorServer) HandleListReports() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		actor, ok := d.ownerAuth.Authenticate(r)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "owner authorization required"})
			return
		}

		reports, err := d.listReports(r.Context(), actor.OwnerID, 50)
		if err != nil {
			d.log.Error("list doctor reports failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list reports"})
			return
		}

		items := make([]doctorReportDTO, 0, len(reports))
		for _, rpt := range reports {
			items = append(items, toDoctorReportDTO(rpt))
		}
		writeJSON(w, http.StatusOK, map[string]any{"reports": items})
	})
}

func (d *DoctorServer) storeReport(ctx context.Context, status string, reportJSON json.RawMessage) (DoctorReport, error) {
	if d.pool == nil {
		return DoctorReport{}, context.Canceled
	}
	var report DoctorReport
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return report, err
	}
	defer tx.Rollback(ctx)
	var key struct {
		Owner string `json:"owner_id"`
		Input string `json:"input_id"`
	}
	_ = json.Unmarshal(reportJSON, &key)
	if key.Input != "" && key.Owner != "" {
		if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "doctor:"+key.Owner+":"+key.Input); err != nil {
			return report, err
		}
		err = tx.QueryRow(ctx, `SELECT id,status,checked_at,report_json FROM doctor_reports WHERE report_json->>'owner_id'=$1 AND report_json->>'input_id'=$2 LIMIT 1`, key.Owner, key.Input).Scan(&report.ID, &report.Status, &report.CheckedAt, &report.ReportJSON)
		if err == nil {
			var stored struct {
				Execution struct {
					Status string `json:"status"`
				} `json:"execution"`
			}
			_ = json.Unmarshal(report.ReportJSON, &stored)
			switch stored.Execution.Status {
			case "queued", "running", "waiting_for_user":
				err = tx.QueryRow(ctx, `UPDATE doctor_reports SET status=$2,checked_at=clock_timestamp(),report_json=$3 WHERE id=$1 RETURNING id,status,checked_at,report_json`, report.ID, status, reportJSON).Scan(&report.ID, &report.Status, &report.CheckedAt, &report.ReportJSON)
				if err != nil {
					return report, err
				}
			}
			return report, tx.Commit(ctx)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return report, err
		}
	}
	err = tx.QueryRow(ctx,
		`INSERT INTO doctor_reports (status, checked_at, report_json) VALUES ($1, NOW(), $2) RETURNING id, status, checked_at, report_json`,
		status, reportJSON,
	).Scan(&report.ID, &report.Status, &report.CheckedAt, &report.ReportJSON)
	if err != nil {
		return report, err
	}
	return report, tx.Commit(ctx)
}

func (d *DoctorServer) listReports(ctx context.Context, owner string, limit int) ([]DoctorReport, error) {
	if d.pool == nil {
		return []DoctorReport{}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	rows, err := d.pool.Query(ctx,
		`SELECT id, status, checked_at, report_json FROM doctor_reports WHERE NOT (report_json ? 'owner_id') OR report_json->>'owner_id'=$1 ORDER BY checked_at DESC LIMIT $2`,
		owner, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reports []DoctorReport
	for rows.Next() {
		var r DoctorReport
		if err := rows.Scan(&r.ID, &r.Status, &r.CheckedAt, &r.ReportJSON); err != nil {
			return nil, err
		}
		reports = append(reports, r)
	}
	if reports == nil {
		reports = []DoctorReport{}
	}
	return reports, rows.Err()
}

// --- DTOs ---

type doctorReportDTO struct {
	ID        int64           `json:"id"`
	Status    string          `json:"status"`
	CheckedAt string          `json:"checked_at"`
	Report    json.RawMessage `json:"report"`
}

func toDoctorReportDTO(r DoctorReport) doctorReportDTO {
	return doctorReportDTO{
		ID:        r.ID,
		Status:    r.Status,
		CheckedAt: r.CheckedAt.UTC().Format(time.RFC3339),
		Report:    r.ReportJSON,
	}
}

// ToolBrokerDoctorChecker calls doctor.check_system via the tool broker.
type ToolBrokerDoctorChecker struct {
	execute func(ctx context.Context, toolName, argsJSON string) (string, error)
}

// NewToolBrokerDoctorChecker creates a checker that calls doctor.check_system through the tool broker.
func NewToolBrokerDoctorChecker(execute func(ctx context.Context, toolName, argsJSON string) (string, error)) *ToolBrokerDoctorChecker {
	return &ToolBrokerDoctorChecker{execute: execute}
}

func (c *ToolBrokerDoctorChecker) RunCheck(ctx context.Context) (string, json.RawMessage, error) {
	resultJSON, err := c.execute(ctx, "doctor.check_system", "{}")
	if err != nil {
		return "unknown", json.RawMessage(`{"status":"unknown","check_error":"transport_failed"}`), err
	}

	// Parse the result to extract the status
	var envelope struct {
		Status string          `json:"status"`
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	if json.Unmarshal([]byte(resultJSON), &envelope) != nil {
		return "unknown", json.RawMessage(`{"status":"unknown","check_error":"invalid_result"}`), nil
	}
	if envelope.Status == "denied" || envelope.Status == "policy_denied" {
		return "degraded", doctorToolFailure(envelope.Result, "degraded", "authorization_denied"), nil
	}
	if envelope.Status == "failed" || envelope.Status == "error" || len(envelope.Error) > 0 && string(envelope.Error) != "null" && string(envelope.Error) != "{}" {
		return "unhealthy", doctorToolFailure(envelope.Result, "unhealthy", "tool_failed"), nil
	}
	if envelope.Status != "completed" && envelope.Status != "success" {
		return "unknown", json.RawMessage(`{"status":"unknown","check_error":"tool_status"}`), nil
	}
	var report map[string]any
	if json.Unmarshal(envelope.Result, &report) != nil || report == nil {
		return "unknown", json.RawMessage(`{"status":"unknown","check_error":"missing_result"}`), nil
	}
	status, _ := report["status"].(string)
	status = combineDoctorStatus(status, "healthy")
	if checks, ok := report["checks"].([]any); ok && len(checks) > 0 {
		for _, item := range checks {
			if check, ok := item.(map[string]any); ok {
				value, _ := check["status"].(string)
				status = combineDoctorStatus(status, value)
			} else {
				status = combineDoctorStatus(status, "unknown")
			}
		}
	} else {
		status = combineDoctorStatus(status, "unknown")
	}
	checked, _ := report["checked_at"].(string)
	when, err := time.Parse(time.RFC3339Nano, checked)
	if err != nil || time.Since(when) > 5*time.Minute || when.After(time.Now().Add(time.Second)) {
		status = combineDoctorStatus(status, "unknown")
		report["check_error"] = "observation_stale_or_missing"
	}
	report["status"] = status
	raw, _ := json.Marshal(report)
	return status, raw, nil
}

func doctorToolFailure(raw json.RawMessage, status, reason string) json.RawMessage {
	var report map[string]any
	if json.Unmarshal(raw, &report) != nil || report == nil {
		report = map[string]any{}
	}
	report["status"], report["check_error"] = status, reason
	b, _ := json.Marshal(report)
	return b
}

func combineDoctorStatus(a, b string) string {
	normalize := func(s string) string {
		switch s {
		case "healthy", "degraded", "unhealthy":
			return s
		}
		return "unknown"
	}
	a, b = normalize(a), normalize(b)
	for _, s := range []string{"unhealthy", "degraded", "unknown"} {
		if a == s || b == s {
			return s
		}
	}
	return "healthy"
}

// Ensure interface compliance.
var _ DoctorChecker = (*ToolBrokerDoctorChecker)(nil)
