package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/butler/butler/internal/logger"
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

// DoctorServer serves REST endpoints for doctor reports.
type DoctorServer struct {
	pool    *pgxpool.Pool
	checker DoctorChecker
	log     *slog.Logger
}

// NewDoctorServer creates a new DoctorServer.
func NewDoctorServer(pool *pgxpool.Pool, checker DoctorChecker, log *slog.Logger) *DoctorServer {
	if log == nil {
		log = slog.Default()
	}
	return &DoctorServer{
		pool:    pool,
		checker: checker,
		log:     logger.WithComponent(log, "doctor-api"),
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

		ctx := r.Context()
		status, reportJSON, err := d.checker.RunCheck(ctx)
		if err != nil {
			d.log.Error("doctor check failed", slog.String("error", err.Error()))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "doctor check failed: " + err.Error()})
			return
		}

		// Store the report
		report, err := d.storeReport(ctx, status, reportJSON)
		if err != nil {
			d.log.Error("failed to store doctor report", slog.String("error", err.Error()))
			// Still return the report even if storage fails
			writeJSON(w, http.StatusOK, map[string]any{
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

		reports, err := d.listReports(r.Context(), 50)
		if err != nil {
			d.log.Error("list doctor reports failed", slog.String("error", err.Error()))
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
	err := d.pool.QueryRow(ctx,
		`INSERT INTO doctor_reports (status, checked_at, report_json) VALUES ($1, NOW(), $2) RETURNING id, status, checked_at, report_json`,
		status, reportJSON,
	).Scan(&report.ID, &report.Status, &report.CheckedAt, &report.ReportJSON)
	return report, err
}

func (d *DoctorServer) listReports(ctx context.Context, limit int) ([]DoctorReport, error) {
	if d.pool == nil {
		return []DoctorReport{}, nil
	}
	rows, err := d.pool.Query(ctx,
		`SELECT id, status, checked_at, report_json FROM doctor_reports ORDER BY checked_at DESC LIMIT $1`,
		limit,
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
		return "unhealthy", nil, err
	}

	// Parse the result to extract the status
	var parsed struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(resultJSON), &parsed); err != nil {
		return "unknown", json.RawMessage(resultJSON), nil
	}
	if parsed.Status == "" {
		parsed.Status = "unknown"
	}

	return parsed.Status, json.RawMessage(resultJSON), nil
}

// Ensure interface compliance.
var _ DoctorChecker = (*ToolBrokerDoctorChecker)(nil)
