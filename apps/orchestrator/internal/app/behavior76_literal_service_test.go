package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

const behavior76LiteralServiceDDL = `
CREATE TABLE behavior76_business_deadlines(
 resource_key text PRIMARY KEY REFERENCES behavior76_objects(key),
 source_ref text NOT NULL,source_revision bigint NOT NULL CHECK(source_revision>0),
 deadline_at timestamptz NOT NULL,configured_at timestamptz NOT NULL DEFAULT clock_timestamp());
CREATE TABLE behavior76_business_deadline_events(
 n bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,resource_key text NOT NULL,
 before_step_id text NOT NULL,provenance text NOT NULL,before_state jsonb,after_state jsonb NOT NULL,
 at timestamptz NOT NULL DEFAULT clock_timestamp(),UNIQUE(resource_key,before_step_id));
CREATE TABLE behavior76_business_observations(
 action_id text PRIMARY KEY,resource_key text NOT NULL,kind text NOT NULL,observation jsonb NOT NULL);`

type behavior76LiteralDeadlineObservation struct {
	Kind            string    `json:"kind"`
	SourceRef       string    `json:"source_ref"`
	SourceRevision  int64     `json:"source_revision"`
	ConfiguredAt    time.Time `json:"configured_at"`
	DeadlineAt      time.Time `json:"deadline_at"`
	ObservedAt      time.Time `json:"observed_at"`
	DeadlinePassed  bool      `json:"deadline_passed_at_observation"`
	PublicationLate *bool     `json:"publication_late,omitempty"`
}

type behavior76LiteralServiceSnapshot struct {
	Deadlines      []json.RawMessage `json:"deadlines"`
	DeadlineEvents []json.RawMessage `json:"deadline_events"`
	Observations   []json.RawMessage `json:"observations"`
}

// This actual transaction clock observation is retained with the confirmed
// synthetic effect receipt. It is not a commit timestamp, logical clock, owner
// amendment, mandate grant or new recipient rejection category. Native setup
// must place declared business dates outside the entire measured window, so an
// in-transaction timestamp cannot conceal a deadline crossing near COMMIT.
func behavior76ObserveLiteralDeadline(ctx context.Context, tx pgx.Tx, key, kind string) (*behavior76LiteralDeadlineObservation, error) {
	value := &behavior76LiteralDeadlineObservation{Kind: "observed_business_date_data"}
	err := tx.QueryRow(ctx, `SELECT source_ref,source_revision,configured_at,deadline_at,clock_timestamp() FROM behavior76_business_deadlines WHERE resource_key=$1 FOR SHARE`, key).Scan(&value.SourceRef, &value.SourceRevision, &value.ConfiguredAt, &value.DeadlineAt, &value.ObservedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	value.DeadlinePassed = !value.ObservedAt.Before(value.DeadlineAt)
	if kind == "commit" {
		late := value.DeadlinePassed
		value.PublicationLate = &late
	}
	return value, nil
}

func (f *behavior76Fixture) seedLiteral(value behavior76LiteralEpisode) {
	f.t.Helper()
	if !f.literal {
		f.t.Fatal("literal service requires trusted constructor")
	}
	f.seed(value.Episode)
	for _, deadline := range value.Deadlines {
		ref := fmt.Sprintf("synthetic:business-date:%s:%s", value.Episode.ID, deadline.ResourceKey)
		_, err := f.pool.Exec(f.ctx, `WITH inserted AS(INSERT INTO behavior76_business_deadlines(resource_key,source_ref,source_revision,deadline_at) VALUES($1,$2,1,$3) RETURNING *) INSERT INTO behavior76_business_deadline_events(resource_key,before_step_id,provenance,after_state) SELECT resource_key,'','synthetic_service_configuration',to_jsonb(inserted) FROM inserted`, deadline.ResourceKey, ref, deadline.DeadlineAt)
		if err != nil {
			f.t.Fatal("declared initial service date unavailable", err)
		}
	}
}

func (f *behavior76Fixture) applyLiteralServiceChanges(value behavior76LiteralEpisode, stepID string) ([]json.RawMessage, error) {
	if !f.literal {
		return nil, errors.New("literal service profile required")
	}
	var receipts []json.RawMessage
	for _, change := range value.DeadlineChanges {
		if change.BeforeStepID != stepID {
			continue
		}
		var raw []byte
		err := f.pool.QueryRow(f.ctx, `WITH previous AS MATERIALIZED(SELECT * FROM behavior76_business_deadlines WHERE resource_key=$1 FOR UPDATE), updated AS(UPDATE behavior76_business_deadlines d SET source_revision=d.source_revision+1,deadline_at=$2,configured_at=clock_timestamp() FROM previous p WHERE d.resource_key=p.resource_key RETURNING d.*), event AS(INSERT INTO behavior76_business_deadline_events(resource_key,before_step_id,provenance,before_state,after_state) SELECT u.resource_key,$3,'synthetic_service_configuration',to_jsonb(p),to_jsonb(u) FROM previous p JOIN updated u USING(resource_key) RETURNING *) SELECT to_jsonb(event) FROM event`, change.ResourceKey, change.DeadlineAt, stepID).Scan(&raw)
		if err != nil {
			return nil, err
		}
		receipts = append(receipts, json.RawMessage(raw))
	}
	return receipts, nil
}

func behavior76LiteralBusinessDatesOutsideWindow(values []behavior76LiteralEpisode, start, deadline time.Time) error {
	if !start.Before(deadline) || deadline.Sub(start) > 4*time.Hour {
		return errors.New("actual finite window required")
	}
	for _, value := range values {
		var dates []time.Time
		for _, d := range value.Deadlines {
			dates = append(dates, d.DeadlineAt)
		}
		for _, d := range value.DeadlineChanges {
			dates = append(dates, d.DeadlineAt)
		}
		for _, date := range dates {
			if date.IsZero() || (!date.Before(start.Add(-time.Minute)) && !date.After(deadline.Add(time.Minute))) {
				return errors.New("business date overlaps measured window or clock margin")
			}
		}
	}
	return nil
}
