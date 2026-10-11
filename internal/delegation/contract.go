// Package delegation defines bounded, data-only executor contracts. Orchestrator
// owns their lifecycle; the model can propose a narrower contract, not authority.
package delegation

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrDenied = errors.New("delegation is stale or outside its bounded contract")

type CredentialRef struct {
	Alias string `json:"alias"`
	Field string `json:"field"`
}
type SourceRef struct {
	ID             string `json:"source_id"`
	Revision       int64  `json:"revision"`
	AccessRevision int64  `json:"access_revision"`
	Hash           string `json:"content_hash"`
}
type Permission struct {
	Tool         string          `json:"tool"`
	ContractHash string          `json:"contract_hash"`
	Resources    []string        `json:"resources"`
	Effects      []string        `json:"effects"`
	Credentials  []CredentialRef `json:"credential_refs"`
}
type Limits struct {
	ModelTurns         int64 `json:"model_turns"`
	Tokens             int64 `json:"tokens"`
	CostMicros         int64 `json:"cost_micros"`
	ExternalCalls      int64 `json:"external_calls"`
	Children           int64 `json:"children"`
	ActiveMillis       int64 `json:"active_millis"`
	EgressBytes        int64 `json:"egress_bytes"`
	ProviderInputBytes int64 `json:"provider_input_bytes"`
}
type Contract struct {
	TaskID              string       `json:"task_id"`
	RootTaskID          string       `json:"root_task_id"`
	ParentTaskID        string       `json:"parent_task_id"`
	ParentRevision      int64        `json:"parent_revision"`
	ConstraintsRevision int64        `json:"parent_constraints_revision"`
	Revision            int64        `json:"contract_revision"`
	Permissions         []Permission `json:"permissions"`
	Sources             []SourceRef  `json:"source_refs"`
	ContractSource      SourceRef    `json:"contract_source"`
	Budget              Limits       `json:"budget"`
	Deadline            time.Time    `json:"deadline"`
	Depth               int          `json:"depth"`
	State               string       `json:"state"`
	InputID             string       `json:"input_id,omitempty"`
}

func Validate(c Contract, now time.Time) error {
	if c.TaskID == "" || c.ParentTaskID == "" || c.RootTaskID == "" || c.TaskID == c.ParentTaskID || c.TaskID == c.RootTaskID || c.ParentRevision < 1 || c.ConstraintsRevision < 1 || c.Revision != 1 || c.Depth < 1 || c.Depth > 3 || !c.Deadline.After(now) || c.Deadline.After(now.Add(24*time.Hour)) || len(c.Permissions) > 32 || len(c.Sources) > 64 {
		return ErrDenied
	}
	seen := map[string]bool{}
	for _, p := range c.Permissions {
		if p.Tool == "" || len(p.Tool) > 128 || strings.ContainsAny(p.Tool, "\x00\r\n* ?/\\") || seen[p.Tool] || !validHash(p.ContractHash) || len(p.Resources) < 1 || len(p.Resources) > 16 || len(p.Effects) < 1 || len(p.Effects) > 4 || len(p.Credentials) > 16 {
			return ErrDenied
		}
		seen[p.Tool] = true
		resources := map[string]bool{}
		for _, r := range p.Resources {
			if r == "" || len(r) > 2048 || strings.ContainsAny(r, "\x00\r\n*") || resources[r] {
				return ErrDenied
			}
			resources[r] = true
		}
		effects := map[string]bool{}
		for _, e := range p.Effects {
			if (e != "read" && e != "prepare" && e != "commit" && e != "unknown") || effects[e] {
				return ErrDenied
			}
			effects[e] = true
		}
		credentials := map[CredentialRef]bool{}
		for _, ref := range p.Credentials {
			if ref.Alias == "" || len(ref.Alias) > 256 || ref.Field == "" || len(ref.Field) > 128 || strings.ContainsAny(ref.Alias+ref.Field, "\x00\r\n*") || credentials[ref] {
				return ErrDenied
			}
			credentials[ref] = true
		}
	}
	seenSources := map[string]bool{}
	for _, r := range append(append([]SourceRef(nil), c.Sources...), c.ContractSource) {
		if r.ID == "" || len(r.ID) > 512 || strings.ContainsAny(r.ID, "\x00\r\n") || r.Revision < 1 || r.AccessRevision < 1 || !validHash(r.Hash) || seenSources[r.ID] {
			return ErrDenied
		}
		seenSources[r.ID] = true
	}
	for _, v := range []int64{c.Budget.ModelTurns, c.Budget.Tokens, c.Budget.CostMicros, c.Budget.ExternalCalls, c.Budget.Children, c.Budget.ActiveMillis, c.Budget.EgressBytes, c.Budget.ProviderInputBytes} {
		if v < 0 {
			return ErrDenied
		}
	}
	return nil
}

// CheckTaskTx is also called by model, Broker and final-result authority paths.
// Old ordinary-task fixtures may omit the new schema; a present broken schema
// fails closed. It never writes another subsystem's tables or borrows a pool.
func CheckTaskTx(ctx context.Context, tx pgx.Tx, id string) error {
	if id == "" {
		return nil
	}
	var exists bool
	if e := tx.QueryRow(ctx, `SELECT to_regclass('task_delegations') IS NOT NULL`).Scan(&exists); e != nil {
		return e
	}
	if !exists {
		return nil
	}
	_, e := tx.Exec(ctx, `SELECT butler_task_check_delegation($1)`, id)
	return e
}

func LoadTx(ctx context.Context, tx pgx.Tx, id string) (*Contract, error) {
	var exists bool
	if e := tx.QueryRow(ctx, `SELECT to_regclass('task_delegations') IS NOT NULL`).Scan(&exists); e != nil {
		return nil, e
	}
	if !exists {
		return nil, nil
	}
	var raw, budgetRaw []byte
	var state, input string
	var root, parent, source string
	var parentRevision, constraintsRevision, contractRevision int64
	var depth int
	var deadline, created time.Time
	e := tx.QueryRow(ctx, `SELECT contract_json,state,input_id,root_task_id,parent_task_id,parent_revision,parent_constraints_revision,contract_revision,contract_source_id,budget_json,deadline,depth,created_at FROM task_delegations WHERE task_id=$1`, id).Scan(&raw, &state, &input, &root, &parent, &parentRevision, &constraintsRevision, &contractRevision, &source, &budgetRaw, &deadline, &depth, &created)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	var c Contract
	var limits Limits
	if e = json.Unmarshal(raw, &c); e != nil || json.Unmarshal(budgetRaw, &limits) != nil || Validate(c, created) != nil || c.TaskID != id || c.RootTaskID != root || c.ParentTaskID != parent || c.ParentRevision != parentRevision || c.ConstraintsRevision != constraintsRevision || c.Revision != contractRevision || c.ContractSource.ID != source || c.Budget != limits || c.Depth != depth || !c.Deadline.Equal(deadline) {
		return nil, ErrDenied
	}
	c.State, c.InputID = state, input
	return &c, nil
}

func validHash(hash string) bool {
	if len(hash) != 64 || strings.ToLower(hash) != hash {
		return false
	}
	_, err := hex.DecodeString(hash)
	return err == nil
}

func Allows(c *Contract, tool, hash, resource, effect string, refs []CredentialRef) bool {
	if c == nil {
		return true
	}
	for _, p := range c.Permissions {
		if p.Tool != tool || p.ContractHash != hash || !has(p.Resources, resource) || !has(p.Effects, effect) {
			continue
		}
		for _, r := range refs {
			found := false
			for _, a := range p.Credentials {
				if r == a {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
		return true
	}
	return false
}
func has(values []string, v string) bool {
	for _, s := range values {
		if s == v {
			return true
		}
	}
	return false
}
