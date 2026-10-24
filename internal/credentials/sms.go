package credentials

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// SMSRoute is explicit owner permission for a paired device and exact sender.
// It is private owner metadata and is never returned by model credential search.
type SMSRoute struct {
	RecordID string `json:"record_id"`
	DeviceID string `json:"device_id"`
	Package  string `json:"package_name"`
	Sender   string `json:"sender"`
	Enabled  bool   `json:"enabled"`
}
type SMSCandidate struct {
	EventID  string `json:"event_id"`
	Package  string `json:"package_name"`
	Sender   string `json:"sender"`
	Code     string `json:"code"`
	PostedAt int64  `json:"posted_at_epoch_ms"`
}

type vaultQuery interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func otpSourceCurrent(ctx context.Context, q vaultQuery, id string) bool {
	var current bool
	err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM credential_vault_otps o JOIN credential_vault_requests q ON q.id=o.request_id WHERE o.request_id=$1 AND o.expires_at>now() AND (o.source_device_id='' OR EXISTS(SELECT 1 FROM credential_vault_sms_routes s JOIN mobile_devices d ON d.device_id=s.device_id AND d.owner_id=s.owner_id WHERE s.record_id=q.record_id AND s.owner_id=q.owner_id AND s.device_id=o.source_device_id AND s.revision=o.source_route_revision AND s.enabled AND d.status='ACTIVE')))`, id).Scan(&current)
	return err == nil && current
}

func (SMSCandidate) String() string   { return "SMSCandidate([redacted])" }
func (SMSCandidate) GoString() string { return "SMSCandidate([redacted])" }

type SMSDevice struct {
	ID   string `json:"device_id"`
	Name string `json:"name"`
}

func (v *Vault) SMSDevices(ctx context.Context, owner string) ([]SMSDevice, error) {
	rows, err := v.Pool.Query(ctx, `SELECT device_id,device_name FROM mobile_devices WHERE owner_id=$1 AND status='ACTIVE' ORDER BY device_name LIMIT 100`, owner)
	if err != nil {
		return nil, ErrVault
	}
	defer rows.Close()
	out := []SMSDevice{}
	for rows.Next() {
		var d SMSDevice
		if rows.Scan(&d.ID, &d.Name) != nil {
			return nil, ErrVault
		}
		out = append(out, d)
	}
	if rows.Err() != nil {
		return nil, ErrVault
	}
	return out, nil
}

func (v *Vault) SMSRoutes(ctx context.Context, owner string) ([]SMSRoute, error) {
	rows, err := v.Pool.Query(ctx, `SELECT record_id,device_id,package_name,sender,enabled FROM credential_vault_sms_routes WHERE owner_id=$1 ORDER BY record_id LIMIT 100`, owner)
	if err != nil {
		return nil, ErrVault
	}
	defer rows.Close()
	out := []SMSRoute{}
	for rows.Next() {
		var r SMSRoute
		if rows.Scan(&r.RecordID, &r.DeviceID, &r.Package, &r.Sender, &r.Enabled) != nil {
			return nil, ErrVault
		}
		out = append(out, r)
	}
	if rows.Err() != nil {
		return nil, ErrVault
	}
	return out, nil
}
func (v *Vault) SaveSMSRoute(ctx context.Context, owner string, r SMSRoute) error {
	if r.RecordID == "" || len(r.DeviceID) > 128 || len(r.Package) > 200 || len(r.Sender) > 200 {
		return ErrVault
	}
	r.Sender = strings.ToLower(strings.TrimSpace(r.Sender))
	if r.Enabled && (r.Sender == "" || r.DeviceID == "" || r.Package == "") {
		return ErrVault
	}
	tx, err := v.Pool.Begin(ctx)
	if err != nil {
		return ErrVault
	}
	defer tx.Rollback(ctx)
	var valid bool
	if tx.QueryRow(ctx, `SELECT TRUE FROM credential_vault WHERE id=$1 AND owner_id=$2 AND status='active' FOR SHARE`, r.RecordID, owner).Scan(&valid) != nil {
		return ErrVault
	}
	if r.Enabled && tx.QueryRow(ctx, `SELECT TRUE FROM mobile_devices WHERE device_id=$1 AND owner_id=$2 AND status='ACTIVE' FOR SHARE`, r.DeviceID, owner).Scan(&valid) != nil {
		return ErrVault
	}
	if _, err = tx.Exec(ctx, `INSERT INTO credential_vault_sms_routes(record_id,owner_id,device_id,package_name,sender,enabled) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(record_id) DO UPDATE SET device_id=$3,package_name=$4,sender=$5,enabled=$6,revision=credential_vault_sms_routes.revision+1 WHERE credential_vault_sms_routes.owner_id=$2`, r.RecordID, owner, r.DeviceID, r.Package, r.Sender, r.Enabled); err != nil {
		return ErrVault
	}
	if v.audit(ctx, tx, owner, r.RecordID, "", "sms_route_changed") != nil || tx.Commit(ctx) != nil {
		return ErrVault
	}
	return nil
}

// RelaySMS is called only under the paired device row lock. Notification bytes
// are DATA, not authority: stored owner routing plus an exact active login grant
// and an unambiguous challenge are required before any code is retained.
func (v *Vault) RelaySMS(ctx context.Context, tx pgx.Tx, owner, device string, c SMSCandidate) (bool, error) {
	if len(c.EventID) < 16 || len(c.EventID) > 128 {
		return false, ErrVault
	}
	if len(c.Code) < 4 || len(c.Code) > 8 || strings.IndexFunc(c.Code, func(r rune) bool { return r < '0' || r > '9' }) >= 0 || len(c.Package) > 200 || len(c.Sender) > 200 {
		return false, ErrVault
	}
	posted := time.UnixMilli(c.PostedAt)
	now := time.Now()
	if posted.Before(now.Add(-90*time.Second)) || posted.After(now.Add(5*time.Second)) {
		return false, nil
	}
	var replay bool
	if tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM credential_vault_sms_receipts WHERE device_id=$1 AND event_id=$2)`, device, c.EventID).Scan(&replay) != nil {
		return false, ErrVault
	}
	if replay {
		return true, nil
	}
	rows, err := tx.Query(ctx, `SELECT q.id,s.revision FROM credential_vault_requests q JOIN credential_vault_requests p ON p.id=q.parent_request_id JOIN credential_vault c ON c.id=q.record_id AND c.owner_id=q.owner_id JOIN credential_vault_sms_routes s ON s.record_id=c.id AND s.owner_id=q.owner_id JOIN runs r ON r.run_id=q.run_id LEFT JOIN tasks t ON t.task_id=q.task_id WHERE q.owner_id=$1 AND q.kind='otp' AND q.state='pending' AND q.expires_at>now() AND q.created_at<=$5::timestamptz+interval '30 seconds' AND p.created_at<=$5 AND p.state='approved' AND p.expires_at>now() AND p.run_id=q.run_id AND p.target_origin=q.target_origin AND p.browser_session_id=q.browser_session_id AND c.status='active' AND c.revision=q.record_revision AND s.enabled AND s.device_id=$2 AND s.package_name=$3 AND s.sender=$4 AND r.current_state='awaiting_approval' AND (q.task_id='' OR (t.owner_id=q.owner_id AND t.revision=q.task_revision AND t.constraints_revision=q.constraints_revision AND t.status='waiting_for_user')) ORDER BY q.created_at LIMIT 2 FOR UPDATE OF q`, owner, device, c.Package, strings.ToLower(strings.TrimSpace(c.Sender)), posted)
	if err != nil {
		return false, ErrVault
	}
	var ids []string
	var routeRevision int64
	for rows.Next() {
		var id string
		if rows.Scan(&id, &routeRevision) != nil {
			rows.Close()
			return false, ErrVault
		}
		ids = append(ids, id)
	}
	if rows.Err() != nil {
		rows.Close()
		return false, ErrVault
	}
	rows.Close()
	if len(ids) != 1 {
		return false, nil
	}
	id := ids[0]
	cipher, err := v.seal(owner, id, "otp", c.Code)
	if err != nil {
		return false, ErrVault
	}
	if _, err = tx.Exec(ctx, `INSERT INTO credential_vault_sms_receipts(device_id,event_id,request_id) VALUES($1,$2,$3)`, device, c.EventID, id); err != nil {
		return false, ErrVault
	}
	if _, err = tx.Exec(ctx, `INSERT INTO credential_vault_otps(request_id,cipher,expires_at,source_device_id,source_route_revision) SELECT id,$2,LEAST(expires_at,now()+interval '2 minutes'),$3,$4 FROM credential_vault_requests WHERE id=$1`, id, cipher, device, routeRevision); err != nil {
		return false, ErrVault
	}
	if _, err = tx.Exec(ctx, `UPDATE credential_vault_requests SET state='approved',expires_at=LEAST(expires_at,now()+interval '2 minutes') WHERE id=$1 AND state='pending'`, id); err != nil {
		return false, ErrVault
	}
	if v.AfterResponse != nil {
		var run string
		if tx.QueryRow(ctx, `SELECT run_id FROM credential_vault_requests WHERE id=$1`, id).Scan(&run) != nil || v.AfterResponse(ctx, tx, run, id) != nil {
			return false, ErrVault
		}
	}
	if v.audit(ctx, tx, owner, "", id, "sms_received") != nil {
		return false, ErrVault
	}
	return true, nil
}
