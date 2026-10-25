package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"orquestrator/internal/store/sqlcgen"
)

// jobsChannel is the Postgres channel notified when jobs are enqueued (see
// the NotifyJobs query). A variable only so tests can break LISTEN.
var jobsChannel = "orq_jobs"

// ErrLeaseLost means another worker took the job over (its lease expired).
var ErrLeaseLost = errors.New("job lease lost")

// Job is a unit of work for a worker.
type Job struct {
	ID       uuid.UUID
	Kind     string
	Payload  json.RawMessage
	Attempts int
}

// NewJob describes a job to enqueue.
type NewJob struct {
	Kind    string
	Payload any
	// Requirements must be contained in a worker's capabilities, e.g. {"host": "x"}.
	Requirements map[string]string
	// MaxAttempts counts runs after a worker died mid-job; 0 means 1.
	MaxAttempts int
}

// EnqueueJob adds a job and wakes the workers.
func (s *Store) EnqueueJob(ctx context.Context, j NewJob) (uuid.UUID, error) {
	payload, err := json.Marshal(j.Payload)
	if err != nil {
		return uuid.Nil, err
	}
	requirements := jsonObject(j.Requirements)
	var id uuid.UUID
	err = s.inTx(ctx, func(q sqlcgen.Querier) error {
		row, err := q.InsertJob(ctx, sqlcgen.InsertJobParams{
			Kind: j.Kind, Payload: payload, Requirements: requirements, MaxAttempts: int32(max(j.MaxAttempts, 1)), //nolint:gosec // small
		})
		if err != nil {
			return err
		}
		id = row.ID
		return q.NotifyJobs(ctx) // delivered on commit
	})
	return id, err
}

// ClaimJob leases the next job the worker can run for lease; nil when
// there is none.
func (s *Store) ClaimJob(ctx context.Context, worker string, capabilities map[string]string, lease time.Duration) (*Job, error) {
	caps := jsonObject(capabilities)
	row, err := s.q.ClaimJob(ctx, sqlcgen.ClaimJobParams{Worker: &worker, LeaseSeconds: lease.Seconds(), Capabilities: caps})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &Job{ID: row.ID, Kind: row.Kind, Payload: row.Payload, Attempts: int(row.Attempts)}, nil
}

// ExtendJobLease renews the lease of a running job.
func (s *Store) ExtendJobLease(ctx context.Context, id uuid.UUID, worker string, lease time.Duration) error {
	n, err := s.q.ExtendJobLease(ctx, sqlcgen.ExtendJobLeaseParams{ID: id, Worker: &worker, LeaseSeconds: lease.Seconds()})
	return leaseResult(n, err)
}

// FinishJob marks a job done, or failed with jobErr.
func (s *Store) FinishJob(ctx context.Context, id uuid.UUID, worker string, jobErr error) error {
	status, lastErr := "done", (*string)(nil)
	if jobErr != nil {
		msg := jobErr.Error()
		status, lastErr = "failed", &msg
	}
	n, err := s.q.FinishJob(ctx, sqlcgen.FinishJobParams{ID: id, Worker: &worker, Status: status, LastError: lastErr})
	return leaseResult(n, err)
}

// jsonObject encodes m as a JSON object, {} when nil.
func jsonObject(m map[string]string) []byte {
	if m == nil {
		return []byte(`{}`)
	}
	data, _ := json.Marshal(m) // string maps always marshal
	return data
}

func leaseResult(n int64, err error) error {
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrLeaseLost
	}
	return nil
}

// JobListener receives the notifications of new jobs on a connection of
// its own, released by Close.
type JobListener struct {
	conn *pgxpool.Conn
}

// ListenJobs holds a connection listening for new jobs.
func (s *Store) ListenJobs(ctx context.Context) (*JobListener, error) {
	conn, err := s.db.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := conn.Exec(ctx, "LISTEN "+jobsChannel); err != nil {
		conn.Release()
		return nil, err
	}
	return &JobListener{conn: conn}, nil
}

// Close gives the connection back to the pool. Wait must not be running.
func (l *JobListener) Close() {
	l.conn.Release()
}

// Wait returns when a job is enqueued or after timeout, whichever comes
// first; it fails only when ctx ends or the connection breaks.
func (l *JobListener) Wait(ctx context.Context, timeout time.Duration) error {
	wctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	_, err := l.conn.Conn().WaitForNotification(wctx)
	if err != nil && ctx.Err() == nil && errors.Is(err, context.DeadlineExceeded) {
		return nil
	}
	return err
}
