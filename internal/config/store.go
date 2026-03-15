package config

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrSettingNotFound = fmt.Errorf("system setting not found")

type Setting struct {
	Key       string
	Value     string
	Component string
	IsSecret  bool
	UpdatedAt time.Time
	UpdatedBy string
}

type SecretValueCodec interface {
	Encrypt(plaintext, associatedData string) (string, error)
	Decrypt(ciphertext, associatedData string) (string, error)
}

type EnvSecretValueCodec struct{}

func (EnvSecretValueCodec) Encrypt(plaintext, associatedData string) (string, error) {
	return Encrypt(plaintext, associatedData)
}

func (EnvSecretValueCodec) Decrypt(ciphertext, associatedData string) (string, error) {
	return Decrypt(ciphertext, associatedData)
}

type StaticSecretValueCodec struct{ Key string }

func (c StaticSecretValueCodec) Encrypt(plaintext, associatedData string) (string, error) {
	return EncryptWithKey(c.Key, plaintext, associatedData)
}

func (c StaticSecretValueCodec) Decrypt(ciphertext, associatedData string) (string, error) {
	return DecryptWithKey(c.Key, ciphertext, associatedData)
}

type settingsQuerier interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type PostgresSettingsStore struct {
	db                    settingsQuerier
	codec                 SecretValueCodec
	allowPlaintextSecrets bool
}

type SettingsStoreOption func(*PostgresSettingsStore)

func WithSettingsCodec(codec SecretValueCodec) SettingsStoreOption {
	return func(store *PostgresSettingsStore) {
		store.codec = codec
	}
}

func WithPlaintextSecretStorage() SettingsStoreOption {
	return func(store *PostgresSettingsStore) {
		store.allowPlaintextSecrets = true
	}
}

func NewPostgresSettingsStore(pool *pgxpool.Pool, options ...SettingsStoreOption) *PostgresSettingsStore {
	store := &PostgresSettingsStore{db: pool, codec: EnvSecretValueCodec{}}
	for _, option := range options {
		option(store)
	}
	return store
}

func (s *PostgresSettingsStore) Get(ctx context.Context, key string) (Setting, error) {
	return s.scanSetting(s.db.QueryRow(ctx, `
		SELECT key, value, component, is_secret, updated_at, updated_by
		FROM system_settings
		WHERE key = $1
	`, strings.TrimSpace(key)))
}

func (s *PostgresSettingsStore) Set(ctx context.Context, setting Setting) (Setting, error) {
	setting = normalizeSetting(setting)
	if err := validateSetting(setting); err != nil {
		return Setting{}, err
	}

	storedValue, err := s.encodeValue(setting)
	if err != nil {
		return Setting{}, err
	}

	return s.scanSetting(s.db.QueryRow(ctx, `
		INSERT INTO system_settings (key, value, component, is_secret, updated_at, updated_by)
		VALUES ($1, $2, $3, $4, NOW(), $5)
		ON CONFLICT (key) DO UPDATE SET
			value = EXCLUDED.value,
			component = EXCLUDED.component,
			is_secret = EXCLUDED.is_secret,
			updated_at = NOW(),
			updated_by = EXCLUDED.updated_by
		RETURNING key, value, component, is_secret, updated_at, updated_by
	`, setting.Key, storedValue, setting.Component, setting.IsSecret, setting.UpdatedBy))
}

func (s *PostgresSettingsStore) Delete(ctx context.Context, key string) error {
	commandTag, err := s.db.Exec(ctx, `DELETE FROM system_settings WHERE key = $1`, strings.TrimSpace(key))
	if err != nil {
		return fmt.Errorf("delete system setting %q: %w", strings.TrimSpace(key), err)
	}
	if commandTag.RowsAffected() == 0 {
		return ErrSettingNotFound
	}
	return nil
}

func (s *PostgresSettingsStore) ListAll(ctx context.Context) ([]Setting, error) {
	rows, err := s.db.Query(ctx, `
		SELECT key, value, component, is_secret, updated_at, updated_by
		FROM system_settings
		ORDER BY key ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("query system settings: %w", err)
	}
	defer rows.Close()

	var settings []Setting
	for rows.Next() {
		setting, err := s.scanSetting(rows)
		if err != nil {
			return nil, err
		}
		settings = append(settings, setting)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate system settings: %w", err)
	}
	return settings, nil
}

func (s *PostgresSettingsStore) scanSetting(row pgx.Row) (Setting, error) {
	var setting Setting
	if err := row.Scan(&setting.Key, &setting.Value, &setting.Component, &setting.IsSecret, &setting.UpdatedAt, &setting.UpdatedBy); err != nil {
		if err == pgx.ErrNoRows {
			return Setting{}, ErrSettingNotFound
		}
		return Setting{}, err
	}
	decoded, err := s.decodeValue(setting)
	if err != nil {
		return Setting{}, err
	}
	setting.Value = decoded
	return setting, nil
}

func (s *PostgresSettingsStore) encodeValue(setting Setting) (string, error) {
	if !setting.IsSecret || s.allowPlaintextSecrets {
		return setting.Value, nil
	}
	if s.codec == nil {
		return "", ErrMissingSettingsEncryptionKey
	}
	value, err := s.codec.Encrypt(setting.Value, setting.Key)
	if err != nil {
		return "", fmt.Errorf("encrypt system setting %q: %w", setting.Key, err)
	}
	return value, nil
}

func (s *PostgresSettingsStore) decodeValue(setting Setting) (string, error) {
	if !setting.IsSecret || s.allowPlaintextSecrets {
		return setting.Value, nil
	}
	if s.codec == nil {
		return "", ErrMissingSettingsEncryptionKey
	}
	value, err := s.codec.Decrypt(setting.Value, setting.Key)
	if err != nil {
		return "", fmt.Errorf("decrypt system setting %q: %w", setting.Key, err)
	}
	return value, nil
}

func normalizeSetting(setting Setting) Setting {
	setting.Key = strings.TrimSpace(setting.Key)
	setting.Component = strings.TrimSpace(setting.Component)
	setting.UpdatedBy = strings.TrimSpace(setting.UpdatedBy)
	return setting
}

func validateSetting(setting Setting) error {
	if setting.Key == "" {
		return fmt.Errorf("key is required")
	}
	if setting.Component == "" {
		return fmt.Errorf("component is required")
	}
	if setting.UpdatedBy == "" {
		return fmt.Errorf("updated_by is required")
	}
	return nil
}
