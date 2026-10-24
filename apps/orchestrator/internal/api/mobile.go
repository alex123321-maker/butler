package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/credentials"
	"github.com/butler/butler/internal/mobile"
	"github.com/butler/butler/internal/owneridentity"
)

type MobileServer struct {
	Store *mobile.Store
	Auth  *OwnerControlAuth
}

func mobileJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func mobileError(w http.ResponseWriter, err error) {
	code := http.StatusServiceUnavailable
	message := "mobile_unavailable"
	switch {
	case errors.Is(err, mobile.ErrInvalid):
		code = 400
		message = "invalid_mobile_request"
	case errors.Is(err, mobile.ErrDenied):
		code = 401
		message = "device_unauthorized"
	case errors.Is(err, mobile.ErrConflict):
		code = 409
		message = "mobile_state_conflict"
	case errors.Is(err, mobile.ErrPaused):
		code = 423
		message = "device_paused"
	case errors.Is(err, mobile.ErrLimit):
		code = 429
		message = "rate_limited"
		w.Header().Set("Retry-After", "60")
	}
	mobileJSON(w, code, map[string]string{"error_code": message})
}
func mobileDecode(w http.ResponseWriter, r *http.Request, v any) error {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return mobile.ErrInvalid
	}
	r.Body = http.MaxBytesReader(w, r.Body, mobile.MaxBody)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return mobile.ErrInvalid
	}
	if _, err = actionauth.ArgumentsHash(string(raw)); err != nil {
		return mobile.ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil {
		return mobile.ErrInvalid
	}
	if d.Decode(new(any)) != io.EOF {
		return mobile.ErrInvalid
	}
	return nil
}
func (s *MobileServer) DeviceHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.Store == nil {
			mobileError(w, errors.New("unavailable"))
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/mobile/")
		if strings.HasPrefix(path, "pair/") {
			if r.Method != http.MethodPost {
				w.WriteHeader(405)
				return
			}
			if err := s.Store.PairLimit(r.Context()); err != nil {
				mobileError(w, err)
				return
			}
			switch path {
			case "pair/exchange", "pair/request":
				var p mobile.PairRequest
				if err := mobileDecode(w, r, &p); err != nil {
					mobileError(w, err)
					return
				}
				if path == "pair/request" {
					v, e := s.Store.RequestPair(r.Context(), p)
					if e != nil {
						mobileError(w, e)
						return
					}
					mobileJSON(w, 200, v)
				} else {
					v, e := s.Store.Exchange(r.Context(), p)
					if e != nil {
						mobileError(w, e)
						return
					}
					mobileJSON(w, 200, v)
				}
			case "pair/poll":
				var p struct {
					Session string `json:"pairing_session_id"`
					Device  string `json:"device_id"`
				}
				if err := mobileDecode(w, r, &p); err != nil {
					mobileError(w, err)
					return
				}
				v, e := s.Store.Poll(r.Context(), p.Session, p.Device)
				if e != nil {
					mobileError(w, e)
					return
				}
				mobileJSON(w, 200, v)
			default:
				w.WriteHeader(404)
			}
			return
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			mobileError(w, mobile.ErrDenied)
			return
		}
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		device := r.Header.Get("X-Butler-Device-Id")
		if path == "status" && r.Method == "GET" {
			v, e := s.Store.Status(r.Context(), device, token)
			if e != nil {
				mobileError(w, e)
				return
			}
			mobileJSON(w, 200, map[string]any{"device_id": device, "status": v, "server_name": "Butler", "server_time_epoch_ms": time.Now().UnixMilli(), "max_batch_size": mobile.MaxBatch})
			return
		}
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		switch path {
		case "otp/login":
			var candidate credentials.SMSCandidate
			if mobileDecode(w, r, &candidate) != nil {
				mobileError(w, mobile.ErrInvalid)
				return
			}
			accepted, err := s.Store.RelayLoginSMS(r.Context(), device, token, candidate)
			if err != nil {
				mobileError(w, err)
				return
			}
			mobileJSON(w, 200, map[string]bool{"accepted": accepted})
		case "notifications/batch":
			if r.Header.Get("X-Butler-Schema-Version") != "1" {
				mobileError(w, mobile.ErrInvalid)
				return
			}
			var p mobile.Batch
			if e := mobileDecode(w, r, &p); e != nil {
				mobileError(w, e)
				return
			}
			ack, e := s.Store.Accept(r.Context(), device, token, p)
			if e != nil {
				mobileError(w, e)
				return
			}
			mobileJSON(w, 200, ack)
		case "disconnect":
			var p struct {
				Device string `json:"device_id"`
				Reason string `json:"reason"`
			}
			if e := mobileDecode(w, r, &p); e != nil || p.Device != device {
				mobileError(w, mobile.ErrInvalid)
				return
			}
			if e := s.Store.Disconnect(r.Context(), device, token); e != nil {
				mobileError(w, e)
				return
			}
			w.WriteHeader(204)
		case "rotate":
			v, e := s.Store.Rotate(r.Context(), device, token)
			if e != nil {
				mobileError(w, e)
				return
			}
			mobileJSON(w, 200, map[string]string{"device_token": v})
		default:
			w.WriteHeader(404)
		}
	})
}
func (s *MobileServer) OwnerHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actor, ok := s.Auth.Authenticate(r)
		if !ok {
			mobileError(w, mobile.ErrDenied)
			return
		}
		if s.Store == nil {
			mobileError(w, errors.New("unavailable"))
			return
		}
		ctx := owneridentity.WithActor(r.Context(), owneridentity.Actor{OwnerID: actor.OwnerID, Channel: "web", DeliveryID: actor.DeliveryID})
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/mobile-owner/")
		if r.Method == "GET" {
			switch path {
			case "attention":
				v, e := s.Store.Attention(ctx, r.URL.Query().Get("reveal") == "true")
				if e != nil {
					mobileError(w, e)
					return
				}
				mobileJSON(w, 200, v)
			case "devices":
				v, e := s.Store.Devices(ctx)
				if e != nil {
					mobileError(w, e)
					return
				}
				mobileJSON(w, 200, v)
			case "events":
				v, e := s.Store.Events(ctx, r.URL.Query().Get("device_id"), r.URL.Query().Get("q"), r.URL.Query().Get("reveal") == "true")
				if e != nil {
					mobileError(w, e)
					return
				}
				mobileJSON(w, 200, v)
			default:
				w.WriteHeader(404)
			}
			return
		}
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		switch path {
		case "noise-rule":
			var p struct {
				ID      string `json:"rule_id"`
				Enabled bool   `json:"enabled"`
			}
			if e := mobileDecode(w, r, &p); e != nil {
				mobileError(w, e)
				return
			}
			if e := s.Store.ChangeNoiseRule(ctx, p.ID, p.Enabled); e != nil {
				mobileError(w, e)
				return
			}
			w.WriteHeader(204)
		case "pairing-code":
			v, e := s.Store.CreateCode(ctx)
			if e != nil {
				mobileError(w, e)
				return
			}
			mobileJSON(w, 200, v)
		case "approve":
			var p struct {
				Code string `json:"code"`
			}
			if e := mobileDecode(w, r, &p); e != nil {
				mobileError(w, e)
				return
			}
			if e := s.Store.ApprovePair(ctx, p.Code); e != nil {
				mobileError(w, e)
				return
			}
			w.WriteHeader(204)
		case "device":
			var p struct {
				ID     string `json:"device_id"`
				Status string `json:"status"`
				Days   int    `json:"retention_days"`
			}
			if e := mobileDecode(w, r, &p); e != nil {
				mobileError(w, e)
				return
			}
			if e := s.Store.ChangeDevice(ctx, p.ID, p.Status, p.Days); e != nil {
				mobileError(w, e)
				return
			}
			w.WriteHeader(204)
		case "delete":
			var p struct {
				Source   string `json:"source_id"`
				Revision int64  `json:"revision"`
			}
			if e := mobileDecode(w, r, &p); e != nil {
				mobileError(w, e)
				return
			}
			if e := s.Store.DeleteEvent(ctx, p.Source, p.Revision); e != nil {
				mobileError(w, e)
				return
			}
			w.WriteHeader(204)
		default:
			w.WriteHeader(404)
		}
	})
}
