package credentials

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrVault = errors.New("credential operation unavailable or stale")

// Vault never returns plaintext through its owner or model metadata projections.
// Its encryption key is an operator-mounted file, independent of the UI PIN.
type Vault struct {
	Pool          *pgxpool.Pool
	key           string
	AfterResponse func(context.Context, pgx.Tx, string, string) error
}

func (*Vault) String() string   { return "CredentialVault([redacted])" }
func (*Vault) GoString() string { return "CredentialVault([redacted])" }

type Entry struct {
	ID           string `json:"id"`
	Alias        string `json:"alias"`
	Title        string `json:"title"`
	SiteHint     string `json:"site_hint"`
	AccountLabel string `json:"account_label"`
	Revision     int64  `json:"revision"`
	Status       string `json:"status"`
}
type EntryInput struct {
	ID               string `json:"id"`
	Alias            string `json:"alias"`
	Title            string `json:"title"`
	SiteHint         string `json:"site_hint"`
	AccountLabel     string `json:"account_label"`
	Username         string `json:"username"`
	Password         string `json:"password"`
	ExpectedRevision int64  `json:"expected_revision"`
}

func (EntryInput) String() string   { return "CredentialEntryInput([redacted])" }
func (EntryInput) GoString() string { return "CredentialEntryInput([redacted])" }

func NewVault(pool *pgxpool.Pool, key string) (*Vault, error) {
	if pool == nil || len(key) < 32 {
		return nil, ErrVault
	}
	return &Vault{Pool: pool, key: key}, nil
}
func VaultFromEnvironment(pool *pgxpool.Pool) (*Vault, error) {
	path := os.Getenv("BUTLER_CREDENTIAL_KEY_FILE")
	if path == "" {
		return nil, ErrVault
	}
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) > 4096 {
		return nil, ErrVault
	}
	return NewVault(pool, strings.TrimSpace(string(raw)))
}
func randomID(prefix string) string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("secure random unavailable")
	}
	return prefix + hex.EncodeToString(b)
}
func digestString(s string) string { d := sha256.Sum256([]byte(s)); return hex.EncodeToString(d[:]) }
func (v *Vault) index(owner, login string) string {
	m := hmac.New(sha256.New, []byte(v.key))
	m.Write([]byte("login\x00" + owner + "\x00" + strings.ToLower(strings.TrimSpace(login))))
	return hex.EncodeToString(m.Sum(nil))
}
func (v *Vault) seal(owner, id, field, value string) (string, error) {
	return sealValue(v.key, value, "credential-v1\x00"+owner+"\x00"+id+"\x00"+field)
}
func (v *Vault) open(owner, id, field, value string) (string, error) {
	result, err := openValue(v.key, value, "credential-v1\x00"+owner+"\x00"+id+"\x00"+field)
	if err != nil {
		return "", ErrVault
	}
	return result, nil
}
func Origin(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Opaque != "" {
		return "", ErrVault
	}
	return strings.ToLower(u.Scheme + "://" + u.Host), nil
}
func (v *Vault) List(ctx context.Context, owner, query string) ([]Entry, error) {
	rows, err := v.Pool.Query(ctx, `SELECT id,alias,title,site_hint,account_label,revision,status FROM credential_vault WHERE owner_id=$1 AND status='active' AND ($2='' OR strpos(lower(alias),lower($2))>0 OR strpos(lower(title),lower($2))>0 OR login_index=$3) ORDER BY (id IN (SELECT record_id FROM credential_vault_requests WHERE owner_id=$1 AND state='pending' AND expires_at>now())) DESC,title,id LIMIT 50`, owner, strings.TrimSpace(query), v.index(owner, query))
	if err != nil {
		return nil, ErrVault
	}
	defer rows.Close()
	entries := []Entry{}
	for rows.Next() {
		var e Entry
		if rows.Scan(&e.ID, &e.Alias, &e.Title, &e.SiteHint, &e.AccountLabel, &e.Revision, &e.Status) != nil {
			return nil, ErrVault
		}
		entries = append(entries, e)
	}
	if rows.Err() != nil {
		return nil, ErrVault
	}
	return entries, nil
}
func (v *Vault) Save(ctx context.Context, owner string, in EntryInput) (Entry, error) {
	in.Alias = strings.TrimSpace(in.Alias)
	in.Title = strings.TrimSpace(in.Title)
	if owner == "" || in.Alias == "" || in.Title == "" || len(in.Alias) > 120 || len(in.Title) > 160 || len(in.SiteHint) > 512 || len(in.AccountLabel) > 160 || len(in.Username) > 4096 || len(in.Password) > 16384 {
		return Entry{}, ErrVault
	}
	if in.ID == "" {
		if in.Username == "" || in.Password == "" || in.ExpectedRevision != 0 {
			return Entry{}, ErrVault
		}
		in.ID = randomID("cred_")
	}
	if in.SiteHint != "" {
		origin, err := Origin(in.SiteHint)
		if err != nil {
			return Entry{}, ErrVault
		}
		in.SiteHint = origin
	}
	tx, err := v.Pool.Begin(ctx)
	if err != nil {
		return Entry{}, ErrVault
	}
	defer tx.Rollback(ctx)
	var userCipher, passCipher string
	if in.ExpectedRevision > 0 {
		if tx.QueryRow(ctx, `SELECT username_cipher,password_cipher FROM credential_vault WHERE id=$1 AND owner_id=$2 AND revision=$3 AND status='active' FOR UPDATE`, in.ID, owner, in.ExpectedRevision).Scan(&userCipher, &passCipher) != nil {
			return Entry{}, ErrVault
		}
	}
	if in.Username != "" {
		userCipher, err = v.seal(owner, in.ID, "username", in.Username)
		if err != nil {
			return Entry{}, ErrVault
		}
	}
	if in.Password != "" {
		passCipher, err = v.seal(owner, in.ID, "password", in.Password)
		if err != nil {
			return Entry{}, ErrVault
		}
	}
	var loginIndex string
	if in.Username != "" {
		loginIndex = v.index(owner, in.Username)
	} else {
		if tx.QueryRow(ctx, `SELECT login_index FROM credential_vault WHERE id=$1 AND owner_id=$2`, in.ID, owner).Scan(&loginIndex) != nil {
			return Entry{}, ErrVault
		}
	}
	var e Entry
	if in.ExpectedRevision == 0 {
		err = tx.QueryRow(ctx, `INSERT INTO credential_vault(id,owner_id,alias,title,site_hint,account_label,username_cipher,password_cipher,login_index) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id,alias,title,site_hint,account_label,revision,status`, in.ID, owner, in.Alias, in.Title, in.SiteHint, in.AccountLabel, userCipher, passCipher, loginIndex).Scan(&e.ID, &e.Alias, &e.Title, &e.SiteHint, &e.AccountLabel, &e.Revision, &e.Status)
	} else {
		err = tx.QueryRow(ctx, `UPDATE credential_vault SET alias=$3,title=$4,site_hint=$5,account_label=$6,username_cipher=$7,password_cipher=$8,login_index=$9,revision=revision+1,updated_at=now() WHERE id=$1 AND owner_id=$2 AND revision=$10 AND status='active' RETURNING id,alias,title,site_hint,account_label,revision,status`, in.ID, owner, in.Alias, in.Title, in.SiteHint, in.AccountLabel, userCipher, passCipher, loginIndex, in.ExpectedRevision).Scan(&e.ID, &e.Alias, &e.Title, &e.SiteHint, &e.AccountLabel, &e.Revision, &e.Status)
	}
	if err != nil {
		return Entry{}, ErrVault
	}
	if err = v.invalidate(ctx, tx, owner, in.ID); err != nil {
		return Entry{}, ErrVault
	}
	if err = v.audit(ctx, tx, owner, in.ID, "", "saved"); err != nil {
		return Entry{}, err
	}
	if tx.Commit(ctx) != nil {
		return Entry{}, ErrVault
	}
	return e, nil
}
func (v *Vault) Remove(ctx context.Context, owner, id string, revision int64, erase bool) error {
	tx, err := v.Pool.Begin(ctx)
	if err != nil {
		return ErrVault
	}
	defer tx.Rollback(ctx)
	sql := `UPDATE credential_vault SET status='revoked',revision=revision+1,updated_at=now() WHERE id=$1 AND owner_id=$2 AND revision=$3`
	if erase {
		sql = `DELETE FROM credential_vault WHERE id=$1 AND owner_id=$2 AND revision=$3`
	}
	tag, err := tx.Exec(ctx, sql, id, owner, revision)
	if err != nil || tag.RowsAffected() != 1 {
		return ErrVault
	}
	if err = v.invalidate(ctx, tx, owner, id); err != nil {
		return ErrVault
	}
	if v.audit(ctx, tx, owner, id, "", "revoked") != nil || tx.Commit(ctx) != nil {
		return ErrVault
	}
	return nil
}
func (v *Vault) audit(ctx context.Context, tx pgx.Tx, owner, id, request, operation string) error {
	_, err := tx.Exec(ctx, `INSERT INTO credential_vault_audit(owner_id,record_id,request_id,operation) VALUES($1,$2,$3,$4)`, owner, id, request, operation)
	if err != nil {
		return ErrVault
	}
	return nil
}

func (v *Vault) invalidate(ctx context.Context, tx pgx.Tx, owner, id string) error {
	rows, err := tx.Query(ctx, `UPDATE credential_vault_requests SET state='denied' WHERE owner_id=$1 AND record_id=$2 AND state IN ('pending','approved') RETURNING run_id,id`, owner, id)
	if err != nil {
		return ErrVault
	}
	var changed [][2]string
	for rows.Next() {
		var r [2]string
		if rows.Scan(&r[0], &r[1]) != nil {
			rows.Close()
			return ErrVault
		}
		changed = append(changed, r)
	}
	if rows.Err() != nil {
		rows.Close()
		return ErrVault
	}
	rows.Close()
	if v.AfterResponse != nil {
		for _, r := range changed {
			if v.AfterResponse(ctx, tx, r[0], r[1]) != nil {
				return ErrVault
			}
		}
	}
	return nil
}
func (v *Vault) PinConfigured(ctx context.Context, owner string) (bool, error) {
	var ok bool
	err := v.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM credential_vault_pins WHERE owner_id=$1)`, owner).Scan(&ok)
	if err != nil {
		return false, ErrVault
	}
	return ok, nil
}

// A keyed verifier prevents offline enumeration of the six-digit PIN from a DB backup.
func (v *Vault) pinHash(owner, salt, pin string) string {
	m := hmac.New(sha256.New, []byte(v.key))
	m.Write([]byte("pin\x00" + owner + "\x00" + salt + "\x00" + pin))
	return hex.EncodeToString(m.Sum(nil))
}
func (v *Vault) Unlock(ctx context.Context, owner, pin, confirm string) (string, error) {
	if len(pin) != 6 || strings.IndexFunc(pin, func(c rune) bool { return c < '0' || c > '9' }) >= 0 {
		return "", ErrVault
	}
	tx, err := v.Pool.Begin(ctx)
	if err != nil {
		return "", ErrVault
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "vault-pin:"+owner); err != nil {
		return "", ErrVault
	}
	var salt, verifier string
	var failures int
	var blocked time.Time
	var rev int64
	err = tx.QueryRow(ctx, `SELECT salt,verifier,failures,blocked_until,revision FROM credential_vault_pins WHERE owner_id=$1 FOR UPDATE`, owner).Scan(&salt, &verifier, &failures, &blocked, &rev)
	if errors.Is(err, pgx.ErrNoRows) {
		if confirm != pin {
			return "", ErrVault
		}
		salt = randomID("")
		rev = 1
		if _, err = tx.Exec(ctx, `INSERT INTO credential_vault_pins(owner_id,salt,verifier) VALUES($1,$2,$3)`, owner, salt, v.pinHash(owner, salt, pin)); err != nil {
			return "", ErrVault
		}
	} else {
		if err != nil || blocked.After(time.Now()) {
			return "", ErrVault
		}
		if !hmac.Equal([]byte(verifier), []byte(v.pinHash(owner, salt, pin))) {
			_, err = tx.Exec(ctx, `UPDATE credential_vault_pins SET failures=LEAST(failures+1,12),blocked_until=clock_timestamp()+make_interval(secs=>LEAST(900,power(2,LEAST(failures,10))::int)) WHERE owner_id=$1`, owner)
			if err == nil {
				_ = tx.Commit(ctx)
			}
			return "", ErrVault
		}
	}
	token := randomID("")
	if _, err = tx.Exec(ctx, `UPDATE credential_vault_pins SET failures=0 WHERE owner_id=$1`, owner); err != nil {
		return "", ErrVault
	}
	if _, err = tx.Exec(ctx, `DELETE FROM credential_vault_unlocks WHERE expires_at<=now()`); err != nil {
		return "", ErrVault
	}
	if _, err = tx.Exec(ctx, `INSERT INTO credential_vault_unlocks(token_hash,owner_id,pin_revision,expires_at) VALUES($1,$2,$3,now()+interval '5 minutes')`, digestString(token), owner, rev); err != nil {
		return "", ErrVault
	}
	if tx.Commit(ctx) != nil {
		return "", ErrVault
	}
	return token, nil
}
func (v *Vault) CheckUnlock(ctx context.Context, owner, token string) error {
	if len(token) != 64 {
		return ErrVault
	}
	tag, err := v.Pool.Exec(ctx, `UPDATE credential_vault_unlocks u SET expires_at=now()+interval '5 minutes' FROM credential_vault_pins p WHERE u.token_hash=$1 AND u.owner_id=$2 AND p.owner_id=u.owner_id AND p.revision=u.pin_revision AND u.expires_at>now()`, digestString(token), owner)
	if err != nil || tag.RowsAffected() != 1 {
		return ErrVault
	}
	return nil
}
func (v *Vault) Lock(ctx context.Context, owner, token string) error {
	_, err := v.Pool.Exec(ctx, `DELETE FROM credential_vault_unlocks WHERE owner_id=$1 AND token_hash=$2`, owner, digestString(token))
	if err != nil {
		return ErrVault
	}
	return nil
}

func (v *Vault) ChangePIN(ctx context.Context, owner, pin, confirm string) error {
	if pin != confirm || len(pin) != 6 || strings.IndexFunc(pin, func(c rune) bool { return c < '0' || c > '9' }) >= 0 {
		return ErrVault
	}
	tx, err := v.Pool.Begin(ctx)
	if err != nil {
		return ErrVault
	}
	defer tx.Rollback(ctx)
	salt := randomID("")
	tag, err := tx.Exec(ctx, `UPDATE credential_vault_pins SET salt=$2,verifier=$3,revision=revision+1,failures=0,blocked_until=now() WHERE owner_id=$1`, owner, salt, v.pinHash(owner, salt, pin))
	if err != nil || tag.RowsAffected() != 1 {
		return ErrVault
	}
	if _, err = tx.Exec(ctx, `DELETE FROM credential_vault_unlocks WHERE owner_id=$1`, owner); err != nil {
		return ErrVault
	}
	if v.audit(ctx, tx, owner, "", "", "pin_changed") != nil || tx.Commit(ctx) != nil {
		return ErrVault
	}
	return nil
}
