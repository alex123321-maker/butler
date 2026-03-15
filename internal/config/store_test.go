package config

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresSettingsStoreSetEncryptsSecretValues(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	fake := &fakeSettingsQuerier{}
	store := &PostgresSettingsStore{db: fake, codec: StaticSecretValueCodec{Key: "unit-test-key"}}

	fake.queryRowFn = func(_ context.Context, _ string, args ...any) pgx.Row {
		storedValue, ok := args[1].(string)
		if !ok {
			t.Fatalf("expected stored value arg to be a string")
		}
		if storedValue == "secret-value" {
			t.Fatal("expected secret value to be encrypted before persistence")
		}
		return fakeRow{values: []any{"BUTLER_OPENAI_API_KEY", storedValue, "orchestrator", true, now, "unit-test"}}
	}

	setting, err := store.Set(ctx, Setting{Key: "BUTLER_OPENAI_API_KEY", Value: "secret-value", Component: "orchestrator", IsSecret: true, UpdatedBy: "unit-test"})
	if err != nil {
		t.Fatalf("Set returned error: %v", err)
	}
	if setting.Value != "secret-value" {
		t.Fatalf("expected decrypted value, got %q", setting.Value)
	}
}

func TestPostgresSettingsStoreGetDecryptsSecretValues(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	codec := StaticSecretValueCodec{Key: "unit-test-key"}
	ciphertext, err := codec.Encrypt("secret-value", "BUTLER_OPENAI_API_KEY")
	if err != nil {
		t.Fatalf("Encrypt returned error: %v", err)
	}

	store := &PostgresSettingsStore{
		db: &fakeSettingsQuerier{queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return fakeRow{values: []any{"BUTLER_OPENAI_API_KEY", ciphertext, "orchestrator", true, now, "unit-test"}}
		}},
		codec: codec,
	}

	setting, err := store.Get(ctx, "BUTLER_OPENAI_API_KEY")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if setting.Value != "secret-value" {
		t.Fatalf("expected decrypted value, got %q", setting.Value)
	}
}

func TestPostgresSettingsStoreDeleteReturnsNotFound(t *testing.T) {
	store := &PostgresSettingsStore{db: &fakeSettingsQuerier{execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
		return pgconn.NewCommandTag("DELETE 0"), nil
	}}}

	if err := store.Delete(context.Background(), "missing"); !errors.Is(err, ErrSettingNotFound) {
		t.Fatalf("expected ErrSettingNotFound, got %v", err)
	}
}

func TestPostgresSettingsStoreListAllReturnsDecodedSettings(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	codec := StaticSecretValueCodec{Key: "unit-test-key"}
	ciphertext, err := codec.Encrypt("secret-value", "BUTLER_OPENAI_API_KEY")
	if err != nil {
		t.Fatalf("Encrypt returned error: %v", err)
	}

	rows := &fakeRows{values: [][]any{
		{"BUTLER_LOG_LEVEL", "debug", "orchestrator", false, now, "unit-test"},
		{"BUTLER_OPENAI_API_KEY", ciphertext, "orchestrator", true, now, "unit-test"},
	}}
	store := &PostgresSettingsStore{db: &fakeSettingsQuerier{queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
		return rows, nil
	}}, codec: codec}

	settings, err := store.ListAll(ctx)
	if err != nil {
		t.Fatalf("ListAll returned error: %v", err)
	}

	want := []Setting{
		{Key: "BUTLER_LOG_LEVEL", Value: "debug", Component: "orchestrator", IsSecret: false, UpdatedAt: now, UpdatedBy: "unit-test"},
		{Key: "BUTLER_OPENAI_API_KEY", Value: "secret-value", Component: "orchestrator", IsSecret: true, UpdatedAt: now, UpdatedBy: "unit-test"},
	}
	if !reflect.DeepEqual(settings, want) {
		t.Fatalf("unexpected settings: %+v", settings)
	}
}

type fakeSettingsQuerier struct {
	execFn     func(context.Context, string, ...any) (pgconn.CommandTag, error)
	queryFn    func(context.Context, string, ...any) (pgx.Rows, error)
	queryRowFn func(context.Context, string, ...any) pgx.Row
}

func (f *fakeSettingsQuerier) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if f.execFn == nil {
		return pgconn.CommandTag{}, nil
	}
	return f.execFn(ctx, sql, args...)
}

func (f *fakeSettingsQuerier) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if f.queryFn == nil {
		return &fakeRows{}, nil
	}
	return f.queryFn(ctx, sql, args...)
}

func (f *fakeSettingsQuerier) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if f.queryRowFn == nil {
		return fakeRow{err: pgx.ErrNoRows}
	}
	return f.queryRowFn(ctx, sql, args...)
}

type fakeRow struct {
	values []any
	err    error
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	for idx, value := range r.values {
		reflect.ValueOf(dest[idx]).Elem().Set(reflect.ValueOf(value))
	}
	return nil
}

type fakeRows struct {
	values [][]any
	index  int
	err    error
}

func (r *fakeRows) Close() {}

func (r *fakeRows) Err() error { return r.err }

func (r *fakeRows) CommandTag() pgconn.CommandTag { return pgconn.CommandTag{} }

func (r *fakeRows) FieldDescriptions() []pgconn.FieldDescription { return nil }

func (r *fakeRows) Next() bool {
	return r.index < len(r.values)
}

func (r *fakeRows) Scan(dest ...any) error {
	if r.index >= len(r.values) {
		return pgx.ErrNoRows
	}
	row := r.values[r.index]
	r.index++
	for idx, value := range row {
		reflect.ValueOf(dest[idx]).Elem().Set(reflect.ValueOf(value))
	}
	return nil
}

func (r *fakeRows) Values() ([]any, error) { return nil, nil }

func (r *fakeRows) RawValues() [][]byte { return nil }

func (r *fakeRows) Conn() *pgx.Conn { return nil }

func (r *fakeRows) FromPool() bool { return false }

func (r *fakeRows) NextRow() bool { return r.Next() }

func (r *fakeRows) ScanRow(dest ...any) error { return r.Scan(dest...) }

func (r *fakeRows) CloseErr() error { return nil }

func (r *fakeRows) ValuesErr() error { return nil }

func (r *fakeRows) FieldDescriptionsErr() error { return nil }

var _ settingsQuerier = (*fakeSettingsQuerier)(nil)
var _ pgx.Rows = (*fakeRows)(nil)
var _ pgx.Row = fakeRow{}
var _ = pgxpool.Pool{}
