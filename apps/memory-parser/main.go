package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/butler/butler/internal/memory/documentparser"
	"github.com/butler/butler/internal/memory/ingestion"
)

func main() {
	engine := &documentparser.Engine{}
	slots := make(chan struct{}, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	for _, mode := range []string{"text", "ocr"} {
		mux.HandleFunc("/"+mode, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/pdf" {
				w.WriteHeader(400)
				return
			}
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
			default:
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 16*time.Second)
			defer cancel()
			b, e := io.ReadAll(http.MaxBytesReader(w, r.Body, ingestion.MaxBytes))
			if e != nil {
				w.WriteHeader(413)
				return
			}
			var pages []ingestion.Page
			if mode == "text" {
				pages, e = engine.TextLayer(ctx, b)
			} else {
				pages, e = engine.Recognize(ctx, b)
			}
			if e != nil {
				status := 422
				if errors.Is(e, ingestion.ErrLimit) || errors.Is(e, context.DeadlineExceeded) {
					status = 413
				}
				w.WriteHeader(status)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			_ = json.NewEncoder(w).Encode(pages)
		})
	}
	s := &http.Server{Addr: ":9091", Handler: mux, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 10 * time.Second, MaxHeaderBytes: 8192}
	_ = s.ListenAndServe()
}
