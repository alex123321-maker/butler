// mobile-lab runs the production MobileServer against an isolated *_test
// database. It never starts a model or connects to an existing Butler instance.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/api"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/mobile"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	initialize := flag.Bool("initialize", false, "initialize a NEW disposable test database and deletion anchor")
	initializeOnly := flag.Bool("initialize-only", false, "initialize a NEW disposable database/anchor, then exit")
	requirePendingRestore := flag.Bool("require-pending-restore", false, "acceptance: prove stale restored state is denied before reconciliation")
	flag.Parse()
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(os.Getenv("MOBILE_LAB_POSTGRES_URL"))
	if err != nil || !strings.HasSuffix(cfg.ConnConfig.Database, "_test") {
		log.Fatal("isolated test database required")
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		log.Fatal("database unavailable")
	}
	defer pool.Close()
	manifest := catalog.NewFileManifest(os.Getenv("MOBILE_LAB_MANIFEST"))
	if *initialize || *initializeOnly {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT to_regclass('memory_sources') IS NOT NULL`).Scan(&exists); err != nil || exists {
			log.Fatal("initialization requires empty test database")
		}
		if _, err = pool.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS vector`); err != nil {
			log.Fatal("vector unavailable")
		}
		files, _ := filepath.Glob("migrations/*.up.sql")
		sort.Strings(files)
		if len(files) == 0 {
			log.Fatal("migrations unavailable")
		}
		for _, file := range files {
			raw, e := os.ReadFile(file)
			if e != nil {
				log.Fatal("migration read failed")
			}
			if _, e = pool.Exec(ctx, string(raw)); e != nil {
				log.Fatalf("migration %s failed", filepath.Base(file))
			}
		}
		if err = manifest.Initialize(); err != nil {
			log.Fatal("anchor initialization failed")
		}
		if *initializeOnly {
			return
		}
	}
	sources := catalog.New(pool, manifest)
	if *requirePendingRestore {
		if sources.CheckRestore(ctx) != catalog.ErrRestore {
			log.Fatal("expected a stale restored deletion watermark")
		}
		log.Print("stale restored database denied before reconciliation")
	}
	if err = sources.Reconcile(ctx); err != nil {
		log.Fatal("anchor reconciliation failed")
	}
	token, err := os.ReadFile(os.Getenv("MOBILE_LAB_OWNER_TOKEN_FILE"))
	if err != nil || len(strings.TrimSpace(string(token))) < 32 {
		log.Fatal("private owner token file required")
	}
	store := &mobile.Store{Pool: pool, Catalog: sources}
	server := &api.MobileServer{Store: store, Auth: api.NewOwnerControlAuth(strings.TrimSpace(string(token)), "mobile-lab-owner")}
	owner := http.NewServeMux()
	owner.Handle("/api/v1/mobile-owner/", server.OwnerHandler())
	device := http.NewServeMux()
	device.Handle("/api/v1/mobile/", server.DeviceHandler())
	device.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if sources.CheckRestore(r.Context()) != nil {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(204)
	})
	go func() {
		log.Fatal((&http.Server{Addr: ":8081", Handler: owner, ReadHeaderTimeout: 5 * time.Second}).ListenAndServe())
	}()
	go func() {
		for range time.Tick(time.Minute) {
			if store.Cleanup(ctx) != nil {
				log.Print("retention deferred")
			}
		}
	}()
	log.Print("isolated mobile receiver ready; owner port 8081, device port 8080")
	log.Fatal((&http.Server{Addr: ":8080", Handler: device, ReadHeaderTimeout: 5 * time.Second}).ListenAndServe())
}
