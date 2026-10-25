package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

const lease = time.Minute

func TestJobLifecycle(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	caps := map[string]string{"host": "a"}

	none, err := s.ClaimJob(ctx, "w1", caps, lease)
	if err != nil || none != nil {
		t.Fatalf("empty queue: %v, %v", none, err)
	}
	other, err := s.EnqueueJob(ctx, NewJob{Kind: "agent.run", Payload: map[string]string{"n": "2"}, Requirements: map[string]string{"host": "b"}})
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.EnqueueJob(ctx, NewJob{Kind: "agent.run", Payload: map[string]string{"n": "1"}})
	if err != nil {
		t.Fatal(err)
	}

	job, err := s.ClaimJob(ctx, "w1", caps, lease)
	if err != nil || job == nil || job.ID != id || job.Attempts != 1 || string(job.Payload) != `{"n": "1"}` {
		t.Fatalf("ClaimJob = %+v, %v", job, err)
	}
	if again, _ := s.ClaimJob(ctx, "w2", caps, lease); again != nil {
		t.Fatalf("job %s requires host b and must not be claimed (got %s)", other, again.ID)
	}
	if err := s.ExtendJobLease(ctx, id, "w1", lease); err != nil {
		t.Fatal(err)
	}
	if err := s.ExtendJobLease(ctx, id, "w2", lease); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("other worker: %v", err)
	}
	if err := s.FinishJob(ctx, id, "w1", errors.New("boom")); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishJob(ctx, id, "w1", nil); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("finishing twice: %v", err)
	}
	var status, lastErr string
	if err := pool.QueryRow(ctx, `SELECT status, last_error FROM jobs WHERE id = $1`, id).Scan(&status, &lastErr); err != nil || status != "failed" || lastErr != "boom" {
		t.Fatalf("status %q, last_error %q, %v", status, lastErr, err)
	}

	byB, err := s.ClaimJob(ctx, "w3", map[string]string{"host": "b"}, lease)
	if err != nil || byB == nil || byB.ID != other {
		t.Fatalf("host b: %+v, %v", byB, err)
	}
	if err := s.FinishJob(ctx, other, "w3", nil); err != nil {
		t.Fatal(err)
	}
}

func TestExpiredLeases(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	id, err := s.EnqueueJob(ctx, NewJob{Kind: "k", Payload: nil, MaxAttempts: 2})
	if err != nil {
		t.Fatal(err)
	}
	if job, err := s.ClaimJob(ctx, "dead", nil, time.Millisecond); err != nil || job == nil {
		t.Fatalf("first claim: %+v, %v", job, err)
	}
	time.Sleep(10 * time.Millisecond)
	job, err := s.ClaimJob(ctx, "alive", nil, time.Millisecond)
	if err != nil || job == nil || job.ID != id || job.Attempts != 2 {
		t.Fatalf("takeover: %+v, %v", job, err)
	}
	if err := s.FinishJob(ctx, id, "dead", nil); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("dead worker finishing: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	if job, err := s.ClaimJob(ctx, "third", nil, lease); err != nil || job != nil {
		t.Fatalf("no attempts left: %+v, %v", job, err)
	}
}

func TestJobErrors(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	if _, err := s.EnqueueJob(ctx, NewJob{Kind: "k", Payload: func() {}}); err == nil {
		t.Error("unmarshalable payload accepted")
	}
	if _, err := s.EnqueueJob(cancelled(), NewJob{Kind: "k"}); err == nil {
		t.Error("EnqueueJob: expected error")
	}
	failOn(s, "InsertJob", 1)
	if _, err := s.EnqueueJob(ctx, NewJob{Kind: "k"}); !errors.Is(err, errInjected) {
		t.Errorf("InsertJob failure: %v", err)
	}
	if _, err := s.ClaimJob(cancelled(), "w", nil, lease); err == nil {
		t.Error("ClaimJob: expected error")
	}
	if err := s.ExtendJobLease(cancelled(), uuid.New(), "w", lease); err == nil {
		t.Error("ExtendJobLease: expected error")
	}
}

func TestJobListener(t *testing.T) {
	s := newStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	l, err := s.ListenJobs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Timeouts are not errors, and the connection stays usable after them.
	for range 2 {
		if err := l.Wait(ctx, 10*time.Millisecond); err != nil {
			t.Fatalf("timeout: %v", err)
		}
	}
	if _, err := s.EnqueueJob(context.Background(), NewJob{Kind: "k"}); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := l.Wait(ctx, 10*time.Second); err != nil || time.Since(start) > 5*time.Second {
		t.Fatalf("notification: %v after %s", err, time.Since(start))
	}
	cancel()
	if err := l.Wait(ctx, time.Second); err == nil {
		t.Fatal("expected an error once the context ended")
	}
	l.Close()

	if _, err := s.ListenJobs(cancelled()); err == nil {
		t.Fatal("ListenJobs: expected error")
	}
	jobsChannel = "not a channel"
	t.Cleanup(func() { jobsChannel = "orq_jobs" })
	if _, err := s.ListenJobs(context.Background()); err == nil {
		t.Fatal("ListenJobs with a bad channel: expected error")
	}
}
