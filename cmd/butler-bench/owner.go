package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/butler/butler/internal/singletabcontrol"
	"io"
	"net/http"
	"net/url"
	"time"
)

type ownerDecision struct {
	ID         string    `json:"approval_id"`
	Kind       string    `json:"kind"`
	Tool       string    `json:"tool,omitempty"`
	Started    time.Time `json:"started_utc"`
	DurationUS int64     `json:"duration_us"`
	Status     int       `json:"http_status,omitempty"`
	Confirmed  bool      `json:"confirmed"`
	Error      string    `json:"error,omitempty"`
}
type approvalReceipt struct {
	Approval struct {
		ID      string `json:"approval_id"`
		Run     string `json:"run_id"`
		Session string `json:"session_key"`
		Tool    string `json:"tool_name"`
		Status  string `json:"status"`
	} `json:"approval"`
}

func (c *liveClient) ownerRequest(ctx context.Context, method, path, delivery string, body []byte, result any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.endpoint+path, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("X-Butler-Delivery-ID", delivery)
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("owner transport failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusConflict {
		return resp.StatusCode, fmt.Errorf("owner endpoint returned %d", resp.StatusCode)
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 65536)).Decode(result); err != nil {
		return resp.StatusCode, fmt.Errorf("invalid owner receipt")
	}
	return resp.StatusCode, nil
}

// The ID comes from the checkpoint of the exact enrolled input. A second
// identity check on the owner receipt prevents granting another run's request.
func (c *liveClient) approve(ctx context.Context, s snapshot, session, event string) (d ownerDecision) {
	start := time.Now()
	d = ownerDecision{ID: s.approvalID, Started: start.UTC(), Kind: "approve"}
	defer func() { d.DurationUS = time.Since(start).Microseconds() }()
	id := url.PathEscape(d.ID)
	delivery := event + "-" + d.ID
	if s.guardedWait {
		d.Kind = "select_tab"
		var choices singletabcontrol.OwnerChoices
		path := "/api/v3/tab-choices/" + id
		code, err := c.ownerRequest(ctx, http.MethodGet, path, delivery, nil, &choices)
		d.Status = code
		if err != nil || code != 200 || choices.ApprovalID != d.ID {
			d.Error = "choices_unavailable"
			return
		}
		var token string
		for _, choice := range choices.Candidates {
			if choice.Origin == c.tabOrigin && c.tabOrigin != "" {
				token = choice.Token
				break
			}
		}
		if token == "" {
			d.Error = "fixture_tab_missing"
			return
		}
		body, _ := json.Marshal(map[string]string{"candidate_token": token})
		var receipt struct {
			Session string `json:"single_tab_session_id"`
		}
		d.Status, err = c.ownerRequest(ctx, http.MethodPost, path+"/select", delivery, body, &receipt)
		d.Confirmed = err == nil && d.Status == 200 && receipt.Session != ""
		if !d.Confirmed {
			d.Error = "selection_unconfirmed"
		}
		return
	}
	path := "/api/v2/approvals/" + id
	var current approvalReceipt
	code, err := c.ownerRequest(ctx, http.MethodGet, path, delivery, nil, &current)
	d.Status = code
	if err != nil || code != 200 || current.Approval.ID != d.ID || current.Approval.Run != s.run || current.Approval.Session != session {
		d.Error = "checkpoint_identity_unconfirmed"
		return
	}
	d.Tool = current.Approval.Tool
	if current.Approval.Status == "approved" {
		d.Confirmed = true
		return
	}
	var receipt approvalReceipt
	d.Status, err = c.ownerRequest(ctx, http.MethodPost, path+"/approve", delivery, nil, &receipt)
	d.Confirmed = err == nil && receipt.Approval.ID == d.ID && receipt.Approval.Run == s.run && receipt.Approval.Session == session && receipt.Approval.Status == "approved"
	if !d.Confirmed {
		d.Error = "approval_unconfirmed"
	}
	return
}
