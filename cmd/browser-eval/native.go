package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var attemptID = regexp.MustCompile(`^browser-baseline-[a-z0-9-]{1,90}$`)
var inputID = regexp.MustCompile(`^input-[a-f0-9]{32}$`)

type nativeReceipt struct {
	InputID  string `json:"input_id"`
	InputRef string `json:"input_ref"`
	RunID    string `json:"run_id,omitempty"`
	Status   string `json:"status"`
}

type nativeObservation struct {
	Delivery       string    `json:"delivery_id"`
	Input          string    `json:"input_id,omitempty"`
	Run            string    `json:"run_id,omitempty"`
	Outcome        string    `json:"observation"`
	HTTP           int       `json:"http_status,omitempty"`
	At             time.Time `json:"at"`
	ElapsedMS      int64     `json:"elapsed_ms"`
	TaskSuccess    string    `json:"task_success"`
	Effect         string    `json:"external_effect"`
	DeliveryStatus string    `json:"answer_delivery"`
}

type nativeClient struct {
	endpoint, token string
	client          *http.Client
}

func newNativeClient(endpoint, token string) (*nativeClient, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" {
		return nil, errors.New("invalid operator endpoint")
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme != "https" && !(u.Scheme == "http" && ip != nil && (ip.IsLoopback() || ip.IsPrivate())) {
		return nil, errors.New("HTTPS or literal private operator endpoint required")
	}
	if u.Host == "" || len(token) < 16 || strings.ContainsAny(token, "\r\n") {
		return nil, errors.New("invalid operator authentication")
	}
	return &nativeClient{strings.TrimRight(endpoint, "/"), token, &http.Client{
		Timeout:       15 * time.Second,
		Transport:     &http.Transport{Proxy: nil, MaxResponseHeaderBytes: 16384},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}, nil
}

func (c *nativeClient) call(ctx context.Context, method, path, delivery string, body []byte) (nativeReceipt, int, error) {
	var result nativeReceipt
	req, err := http.NewRequestWithContext(ctx, method, c.endpoint+path, bytes.NewReader(body))
	if err != nil {
		return result, 0, errors.New("invalid request")
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if method == "POST" {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Butler-Delivery-ID", delivery)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return result, 0, errors.New("operator request acknowledgement unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return result, resp.StatusCode, errors.New("operator request refused")
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 65537))
	if err != nil || len(raw) > 65536 || json.Unmarshal(raw, &result) != nil {
		return nativeReceipt{}, resp.StatusCode, errors.New("operator receipt unavailable")
	}
	if result.InputID == "" {
		result.InputID = result.InputRef
	}
	if !inputID.MatchString(result.InputID) {
		return nativeReceipt{}, resp.StatusCode, errors.New("invalid input reference")
	}
	// Never carry raw API errors, checkpoint text, assistant response or arbitrary
	// server strings into the operator's safe evidence output.
	if !regexp.MustCompile(`^run-[a-zA-Z0-9-]{1,100}$`).MatchString(result.RunID) {
		result.RunID = ""
	}
	switch result.Status {
	case "queued", "running", "waiting_for_user", "completed", "failed", "cancelled", "expired", "blocked":
	default:
		result.Status = "unavailable"
	}
	return result, resp.StatusCode, nil
}

func nativeAttempt(ctx context.Context, c *nativeClient, prompt, delivery, directory string, poll time.Duration) error {
	if !attemptID.MatchString(delivery) || len(strings.TrimSpace(prompt)) == 0 || len(prompt) > 16000 {
		return errors.New("invalid attempt")
	}
	if err := os.Mkdir(directory, 0700); err != nil {
		return errors.New("attempt directory must be new")
	}
	start := time.Now()
	hash := sha256.Sum256([]byte(prompt))
	if err := exclusiveJSON(filepath.Join(directory, "00-intent.json"), map[string]any{"delivery_id": delivery, "started_at": start.UTC(), "prompt_sha256": hex.EncodeToString(hash[:]), "observation_timeout_ms": time.Until(deadline(ctx)).Milliseconds()}); err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]string{"text": prompt})
	receipt, code, err := c.call(ctx, "POST", "/api/v3/communications/messages", delivery, body)
	observation := func(state string) nativeObservation {
		return nativeObservation{delivery, receipt.InputID, receipt.RunID, state, code, time.Now().UTC(), time.Since(start).Milliseconds(), "UNVERIFIED", "UNKNOWN", "UNVERIFIED"}
	}
	if err != nil {
		// A POST might have committed even if no acknowledgement arrived. Never
		// submit another delivery automatically, nor hide this attempt.
		return exclusiveJSON(filepath.Join(directory, "01-submission.json"), observation("SUBMISSION_UNKNOWN"))
	}
	if err := exclusiveJSON(filepath.Join(directory, "01-submission.json"), observation("ACCEPTED")); err != nil {
		return err
	}
	id := receipt.InputID
	for {
		select {
		case <-ctx.Done():
			return exclusiveJSON(filepath.Join(directory, "02-observation.json"), observation("OBSERVATION_TIMEOUT"))
		case <-time.After(poll):
		}
		next, status, e := c.call(ctx, "GET", "/api/v3/inputs/"+url.PathEscape(id), "", nil)
		if e != nil {
			if ctx.Err() != nil {
				continue
			}
			return exclusiveJSON(filepath.Join(directory, "02-observation.json"), observation("RECEIPT_UNAVAILABLE"))
		}
		if next.InputID != id {
			return exclusiveJSON(filepath.Join(directory, "02-observation.json"), observation("RECEIPT_MISMATCH"))
		}
		receipt, code = next, status
		switch receipt.Status {
		case "completed", "failed", "cancelled", "expired", "blocked", "waiting_for_user":
			return exclusiveJSON(filepath.Join(directory, "02-observation.json"), observation("INPUT_"+strings.ToUpper(receipt.Status)))
		}
	}
}

func deadline(ctx context.Context) time.Time {
	d, ok := ctx.Deadline()
	if !ok {
		return time.Now()
	}
	return d
}

func runNative(args []string) error {
	fs := flag.NewFlagSet("native", flag.ContinueOnError)
	endpoint := fs.String("endpoint", "", "trusted operator Core endpoint")
	tokenFile := fs.String("owner-token-file", "", "private owner token file reference")
	promptFile := fs.String("prompt-file", "", "UTF-8 task file, without credentials")
	delivery := fs.String("delivery-id", "", "unique browser-baseline-* identity")
	directory := fs.String("attempt-dir", "", "new private attempt directory")
	timeout := fs.Duration("timeout", 3*time.Minute, "bounded observation window; does not cancel work")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *timeout < time.Second || *timeout > 10*time.Minute || *directory == "" || fs.NArg() != 0 {
		return errors.New("invalid observation limits")
	}
	token, err := os.ReadFile(*tokenFile)
	if err != nil {
		return errors.New("private owner reference unavailable")
	}
	prompt, err := os.ReadFile(*promptFile)
	if err != nil {
		return errors.New("prompt unavailable")
	}
	c, err := newNativeClient(*endpoint, strings.TrimSpace(string(token)))
	if err != nil {
		return err
	}
	defer c.client.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	return nativeAttempt(ctx, c, string(prompt), *delivery, *directory, 1500*time.Millisecond)
}
