package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5/pgxpool"
)

var history = map[string]string{
	"native-scenario-failed.json":          "28c75c31827f8a25ca8ed101e6e04b8264a7e66ee9ce18808d504d5af748c8f0",
	"native-scenario-followup-failed.json": "f5d25f36b574464508a4124ac7490d525e75d8c8cf60111a0bb637637e777619",
	"native-scenario-third-failed.json":    "b7a4b35b7b587fb8f0561cbdb18f5ad8088b7939e673ffa290e06ec10410b89b",
	"native-scenario-fourth.json":          "72d95bb5ac0bcf5bd4d8d4b28b1de6beb10d819a348c7e5efb187295ce28a1f9",
	"native-scenario-fifth.json":           "5e180087d4e1d43fe52c6afac0c35763b69a8dfe906953a18eac49416bd7cded",
}

func verifyHistory(root string) error {
	for file, hash := range history {
		b, err := os.ReadFile(filepath.Join(root, "deploy/delegation", file))
		if err != nil {
			return denied
		}
		sum := sha256.Sum256(b)
		if hex.EncodeToString(sum[:]) != hash {
			return denied
		}
	}
	if _, err := os.Stat("/baseline13"); err != nil {
		return denied
	}
	return nil
}

type pgAdmission struct {
	pool       *pgxpool.Pool
	root       string
	native8192 bool
	high       bool
}

func (p *pgAdmission) profile() (base, attempts, output int) {
	if p.high {
		return 21, 1, 8192
	}
	if p.native8192 {
		return 20, 1, 8192
	}
	return 18, 2, 512
}

func verifyComparison(root string) error {
	b, err := os.ReadFile(filepath.Join(root, "deploy/antigravity/live-comparison-20261011.json"))
	if err != nil {
		return denied
	}
	sum := sha256.Sum256(b)
	if hex.EncodeToString(sum[:]) != "6808c50e120b59a40c2c2d9b53babb7dba1e581a3fbcb29ddb3cae6b9491e472" {
		return denied
	}
	return nil
}

func (p *pgAdmission) init(ctx context.Context) error {
	if p.high && verifyHighHistory(p.root) != nil {
		return denied
	}
	base, attempts, output := p.profile()
	if p.native8192 && verifyComparison(p.root) != nil {
		return denied
	}
	// Dedicated diagnostic database only; this is not a production migration.
	_, err := p.pool.Exec(ctx, fmt.Sprintf(`CREATE TABLE IF NOT EXISTS diagnostic_window (
	 id integer PRIMARY KEY CHECK(id=1), historical integer NOT NULL CHECK(historical=%d),
	 spent integer NOT NULL CHECK(spent BETWEEN 0 AND %d), deadline timestamptz NOT NULL);
	 CREATE TABLE IF NOT EXISTS diagnostic_attempt (
	 id integer PRIMARY KEY CHECK(id BETWEEN %d AND %d), client text UNIQUE NOT NULL CHECK(client IN ('cli','go')),
	 started timestamptz NOT NULL DEFAULT clock_timestamp(), observation jsonb,
	 reserved_input_tokens integer NOT NULL DEFAULT 1048576 CHECK(reserved_input_tokens=1048576),
	 reserved_output_tokens integer NOT NULL DEFAULT %d CHECK(reserved_output_tokens=%d),
	 reserved_monetary_micros integer NOT NULL DEFAULT 0 CHECK(reserved_monetary_micros=0),
	 observed_monetary_micros bigint,
	 cost_basis text NOT NULL DEFAULT 'subscription_no_overage' CHECK(cost_basis='subscription_no_overage'));
	 INSERT INTO diagnostic_window VALUES(1,%d,0,clock_timestamp()+interval '120 seconds')`, base, attempts, base+1, base+attempts, output, output, base))
	return err
}

func (p *pgAdmission) Reserve(ctx context.Context, client string) (int, error) {
	if p.high && (client != "go" || verifyHighHistory(p.root) != nil) {
		return 0, denied
	}
	base, attempts, _ := p.profile()
	if p.native8192 && (client != "go" || verifyComparison(p.root) != nil) {
		return 0, denied
	}
	if client != "cli" && client != "go" {
		return 0, denied
	}
	if verifyHistory(p.root) != nil {
		return 0, denied
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return 0, denied
	}
	defer tx.Rollback(context.Background())
	var spent int
	if tx.QueryRow(ctx, `UPDATE diagnostic_window SET spent=spent+1 WHERE id=1 AND spent<$1 AND deadline>clock_timestamp() RETURNING spent`, attempts).Scan(&spent) != nil {
		return 0, denied
	}
	if _, err = tx.Exec(ctx, `INSERT INTO diagnostic_attempt(id,client) VALUES($1,$2)`, base+spent, client); err != nil {
		return 0, denied
	}
	if tx.Commit(ctx) != nil {
		return 0, denied
	}
	return base + spent, nil
}

func (p *pgAdmission) Observe(ctx context.Context, id int, o observation) error {
	b, err := json.Marshal(o)
	if err != nil {
		return denied
	}
	_, err = p.pool.Exec(ctx, `UPDATE diagnostic_attempt SET observation=$1 WHERE id=$2`, b, id)
	return err
}
