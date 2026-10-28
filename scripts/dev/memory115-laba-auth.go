//go:build ignore

// Auth recovery/CAS helpers retained from the closed PC operator; the server
// host driver changes process containment, not encrypted state semantics.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/providerauth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"io"
	"os"
	"strings"
	"syscall"
	"time"
)

const local115AuthKey = "BUTLER_PROVIDER_AUTH_ANTIGRAVITY"

func local115Fail(code string) error { return errors.New(code) }

func local115ReadPrivate(path string) ([]byte, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, local115Fail("private regular reference unavailable")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() < 1 || info.Size() > 1024*1024 {
		return nil, local115Fail("private bounded regular reference unavailable")
	}
	raw, err := io.ReadAll(io.LimitReader(f, 1024*1024+1))
	if err != nil || len(raw) != int(info.Size()) {
		return nil, local115Fail("private reference read failed")
	}
	return raw, nil
}

func local115Exclusive(path string, raw []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return local115Fail("one-use private output unavailable")
	}
	_, writeErr := f.Write(raw)
	syncErr := f.Sync()
	closeErr := f.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		return local115Fail("private output persistence failed")
	}
	return nil
}

func local115JSON(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return local115Fail("safe receipt encoding failed")
	}
	return local115Exclusive(path, append(raw, '\n'))
}

// This sink validates the existing strict private-import format without
// persisting or activating credentials. PostgreSQL remains authoritative.
type local115ValidationSink struct{}

func (local115ValidationSink) Get(context.Context, string) (config.Setting, error) {
	return config.Setting{}, config.ErrSettingNotFound
}
func (local115ValidationSink) Set(context.Context, config.Setting) (config.Setting, error) {
	return config.Setting{}, nil
}
func (local115ValidationSink) Delete(context.Context, string) error {
	return config.ErrSettingNotFound
}

func local115ValidateCipher(ctx context.Context, key, encrypted string) error {
	if len(encrypted) == 0 || len(encrypted) > 128*1024 {
		return local115Fail("bounded encrypted local auth unavailable")
	}
	plain, err := (config.StaticSecretValueCodec{Key: key}).Decrypt(encrypted, local115AuthKey)
	if err != nil {
		return local115Fail("local auth encryption binding invalid")
	}
	if providerauth.NewManager(local115ValidationSink{}).ImportAntigravity(ctx, strings.NewReader(plain), "issue115-private-format-validation") != nil {
		return local115Fail("local auth strict private format invalid")
	}
	return nil
}

func local115Hash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", local115Fail("pinned artifact unavailable")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 512*1024*1024 {
		return "", local115Fail("pinned artifact shape invalid")
	}
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, 512*1024*1024+1))
	if err != nil || n != info.Size() {
		return "", local115Fail("pinned artifact hash failed")
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

type local115ControlQueries interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func local115ControlIdle(ctx context.Context, conn local115ControlQueries) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var idle bool
	err := conn.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM public.behavior76_windows WHERE NOT closed) AND NOT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND backend_type='client backend' AND pid<>pg_backend_pid())`).Scan(&idle)
	if err != nil || !idle {
		return local115Fail("local source window or credential backend is active or unavailable")
	}
	// The real source retains each fixture's task tables in its own schema;
	// only the provider journal/settings live in public. Inspect catalog names,
	// never model data, and require complete lease/Input pairs in every schema.
	var raw []byte
	if conn.QueryRow(ctx, `SELECT COALESCE(jsonb_agg(jsonb_build_object('schema',nspname,'table',relname) ORDER BY nspname,relname),'[]'::jsonb) FROM (SELECT n.nspname,c.relname FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE c.relkind='r' AND c.relname IN('session_ownership','accepted_inputs') AND n.nspname NOT LIKE 'pg_%' AND n.nspname<>'information_schema' ORDER BY n.nspname,c.relname LIMIT 513) source_tables`).Scan(&raw) != nil || len(raw) > 256*1024 {
		return local115Fail("local source task-store catalog unavailable")
	}
	var tables []struct {
		Schema string `json:"schema"`
		Table  string `json:"table"`
	}
	if json.Unmarshal(raw, &tables) != nil || len(tables) == 0 || len(tables) > 512 {
		return local115Fail("bounded complete source task stores required")
	}
	pairs := make(map[string]int)
	for _, table := range tables {
		column, bit := "", 0
		switch table.Table {
		case "session_ownership":
			column, bit = "expires_at", 1
		case "accepted_inputs":
			column, bit = "claim_expires_at", 2
		default:
			return local115Fail("unknown source task-store relation")
		}
		if table.Schema == "" || len(table.Schema) > 63 || pairs[table.Schema]&bit != 0 {
			return local115Fail("source task-store identity differs")
		}
		pairs[table.Schema] |= bit
		query := "SELECT NOT EXISTS(SELECT 1 FROM " + pgx.Identifier{table.Schema, table.Table}.Sanitize() + " WHERE " + pgx.Identifier{column}.Sanitize() + ">clock_timestamp())"
		if conn.QueryRow(ctx, query).Scan(&idle) != nil || !idle {
			return local115Fail("source task-store live lease or claim remains, or observation unavailable")
		}
	}
	for _, pair := range pairs {
		if pair != 3 {
			return local115Fail("source task-store lease/Input pair incomplete")
		}
	}
	return nil
}

func local115Return(ctx context.Context, conn local115ControlQueries, expected, latest string) error {
	if err := local115ControlIdle(ctx, conn); err != nil {
		return err
	}
	tag, err := conn.Exec(ctx, `UPDATE public.system_settings SET value=$1,updated_by='issue115-local-serialized-handback',updated_at=clock_timestamp() WHERE key=$2 AND is_secret AND value=$3`, latest, local115AuthKey, expected)
	if err != nil || tag.RowsAffected() != 1 {
		return local115Fail("local encrypted auth CAS denied; private material retained")
	}
	return nil
}

func local115WaitDatabaseExit(ctx context.Context, target interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}) error {
	for {
		var quiet bool
		if target.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND backend_type='client backend' AND pid<>pg_backend_pid())`).Scan(&quiet) != nil {
			return local115Fail("native database backend exit unavailable; auth handback prohibited")
		}
		if quiet {
			return nil
		}
		timer := time.NewTimer(25 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return local115Fail("native database backends remain; auth handback prohibited")
		case <-timer.C:
		}
	}
}

func local115Recover(ctx context.Context, target interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, planSHA, expected string, importedReceipt bool) (string, string, error) {
	var schema, table bool
	if target.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_namespace WHERE nspname='experience_native_v1'),to_regclass('public.issue90_native_windows') IS NOT NULL`).Scan(&schema, &table) != nil {
		return "", "", local115Fail("native domain recovery unavailable")
	}
	if table {
		if !schema {
			return "", "", local115Fail("native claim exists without its owning schema")
		}
		var valid bool
		if target.QueryRow(ctx, `SELECT count(*)=1 AND bool_and(window_id='experience-explicit-v1' AND window_hash=$1 AND schema_name='experience_native_v1' AND owner_id='owner') FROM public.issue90_native_windows`, planSHA).Scan(&valid) != nil || !valid {
			return "", "", local115Fail("native recovery domain identity differs")
		}
	} else if schema {
		return "", "", local115Fail("native schema has no matching claim")
	}
	var settings bool
	if schema && target.QueryRow(ctx, `SELECT to_regclass('experience_native_v1.system_settings') IS NOT NULL`).Scan(&settings) != nil {
		return "", "", local115Fail("native auth table recovery unavailable")
	}
	if settings {
		var latest string
		err := target.QueryRow(ctx, `SELECT value FROM experience_native_v1.system_settings WHERE key=$1 AND is_secret`, local115AuthKey).Scan(&latest)
		if err == nil && latest != "" {
			return latest, "latest_durable_local_setting", nil
		}
		if err != pgx.ErrNoRows {
			return "", "", local115Fail("native auth recovery uncertain")
		}
	}
	if importedReceipt {
		return "", "", local115Fail("provider imported without recoverable encrypted setting")
	}
	// No authenticated provider can exist before the durable import receipt.
	// A matching empty domain may have been allocated before that point.
	if schema {
		var entries bool
		if target.QueryRow(ctx, `SELECT to_regclass('experience_native_v1.experience_native_entries') IS NOT NULL`).Scan(&entries) != nil {
			return "", "", local115Fail("native journal recovery unavailable")
		}
		if entries {
			var n int
			if target.QueryRow(ctx, `SELECT count(*) FROM experience_native_v1.experience_native_entries`).Scan(&n) != nil || n != 0 {
				return "", "", local115Fail("native entries exist without recoverable auth")
			}
		}
	}
	return expected, "unused_original_before_provider_import", nil
}
