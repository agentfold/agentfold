package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"orquestrator/internal/store/sqlcgen"
)

// Postgres channels notified when jobs are enqueued (the NotifyJobs query)
// and when another process changes the board (NotifyChange). Variables only
// so tests can break LISTEN.
var (
	jobsChannel    = "orq_jobs"
	changesChannel = "orq_changes"
)

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
	var id uuid.UUID
	err := s.inTx(ctx, func(q sqlcgen.Querier) error {
		var err error
		id, err = insertJob(ctx, q, j)
		return err
	})
	return id, err
}

// insertJob adds a job within a transaction; the workers wake on commit.
func insertJob(ctx context.Context, q sqlcgen.Querier, j NewJob) (uuid.UUID, error) {
	payload, err := json.Marshal(j.Payload)
	if err != nil {
		return uuid.Nil, err
	}
	row, err := q.InsertJob(ctx, sqlcgen.InsertJobParams{
		Kind: j.Kind, Payload: payload, Requirements: jsonObject(j.Requirements), MaxAttempts: int32(max(j.MaxAttempts, 1)), //nolint:gosec // small
	})
	if err != nil {
		return uuid.Nil, err
	}
	return row.ID, q.NotifyJobs(ctx)
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

// listener holds a connection of its own listening on a channel, released
// by Close.
type listener struct {
	conn *pgxpool.Conn
}

func (s *Store) listen(ctx context.Context, channel string) (listener, error) {
	conn, err := s.db.Acquire(ctx)
	if err != nil {
		return listener{}, err
	}
	if _, err := conn.Exec(ctx, "LISTEN "+channel); err != nil {
		conn.Release()
		return listener{}, err
	}
	return listener{conn: conn}, nil
}

// Close gives the connection back to the pool. No wait must be running.
func (l listener) Close() {
	l.conn.Release()
}

// JobListener receives the notifications of new jobs.
type JobListener struct {
	listener
}

// ListenJobs holds a connection listening for new jobs.
func (s *Store) ListenJobs(ctx context.Context) (*JobListener, error) {
	l, err := s.listen(ctx, jobsChannel)
	if err != nil {
		return nil, err
	}
	return &JobListener{l}, nil
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

// change is the payload of a change notification.
type change struct {
	Type string    `json:"type"`
	ID   uuid.UUID `json:"id"`
}

// NotifyChange announces a change made outside the server, which streams
// it to the board. typ is "<entity>.<change>", as in the board events.
func (s *Store) NotifyChange(ctx context.Context, typ string, id uuid.UUID) error {
	data, _ := json.Marshal(change{Type: typ, ID: id}) // a string and a uuid
	return s.q.NotifyChange(ctx, string(data))
}

// ChangeListener receives the changes announced by NotifyChange.
type ChangeListener struct {
	listener
}

// ListenChanges holds a connection listening for changes.
func (s *Store) ListenChanges(ctx context.Context) (*ChangeListener, error) {
	l, err := s.listen(ctx, changesChannel)
	if err != nil {
		return nil, err
	}
	return &ChangeListener{l}, nil
}

// Next waits for the next change; it fails when ctx ends, the connection
// breaks or the notification is not a change.
func (l *ChangeListener) Next(ctx context.Context) (string, uuid.UUID, error) {
	n, err := l.conn.Conn().WaitForNotification(ctx)
	if err != nil {
		return "", uuid.Nil, err
	}
	var c change
	if err := json.Unmarshal([]byte(n.Payload), &c); err != nil {
		return "", uuid.Nil, fmt.Errorf("change notification %q: %w", n.Payload, err)
	}
	return c.Type, c.ID, nil
}
