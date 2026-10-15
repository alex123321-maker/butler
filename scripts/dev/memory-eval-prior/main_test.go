package main

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestRenewalClockRoundTripsPostgresDeadline(t *testing.T) {
	input := time.Date(2026, 10, 15, 14, 30, 0, 123456789, time.FixedZone("operator", 7*3600))
	issued := renewalTimestamp(input)
	deadline := issued.Add(30 * time.Minute)
	m := pgtype.NewMap()
	codec := pgtype.TimestamptzCodec{}
	value := pgtype.Timestamptz{Time: deadline, Valid: true}
	plan := codec.PlanEncode(m, pgtype.TimestamptzOID, pgtype.BinaryFormatCode, value)
	encoded, err := plan.Encode(value, nil)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := codec.DecodeValue(m, pgtype.TimestamptzOID, pgtype.BinaryFormatCode, encoded)
	if err != nil {
		t.Fatal(err)
	}
	stored, ok := decoded.(time.Time)
	if !ok || !stored.Equal(deadline) || issued.Location() != time.UTC || issued.After(input) || input.Sub(issued) >= time.Microsecond || deadline.Sub(issued) != 30*time.Minute {
		t.Fatal("operator mandate differs after actual PG codec", issued, deadline, decoded)
	}
}
