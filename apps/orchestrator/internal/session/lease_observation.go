package session

import "github.com/jackc/pgx/v5/pgxpool"

// LeaseObservationError records only a fixed operation phase and capacity.
// The underlying cause remains inspectable through errors.Is/As; Error never
// includes a DSN, query, parameter, lease identifier or database error text.
type LeaseObservationError struct {
	Phase                 string
	Acquired, Idle, Total int32
	cause                 error
}

func (e *LeaseObservationError) Error() string { return "lease observation failed" }
func (e *LeaseObservationError) Unwrap() error { return e.cause }

func leaseObservationError(pool *pgxpool.Pool, phase string, cause error) error {
	stat := pool.Stat()
	return &LeaseObservationError{Phase: phase, Acquired: stat.AcquiredConns(), Idle: stat.IdleConns(), Total: stat.TotalConns(), cause: cause}
}
