package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"orquestrator/internal/store"
)

// fakeQueue hands out its jobs in order and records what happens to them.
type fakeQueue struct {
	mu        sync.Mutex
	jobs      []*store.Job
	claimErr  error
	extendErr error
	finishErr error
	extended  int
	finished  map[uuid.UUID]error
	caps      map[string]string
}

func (q *fakeQueue) ClaimJob(_ context.Context, _ string, caps map[string]string, _ time.Duration) (*store.Job, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.caps = caps
	if q.claimErr != nil {
		return nil, q.claimErr
	}
	if len(q.jobs) == 0 {
		return nil, nil
	}
	job := q.jobs[0]
	q.jobs = q.jobs[1:]
	return job, nil
}

func (q *fakeQueue) ExtendJobLease(context.Context, uuid.UUID, string, time.Duration) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.extended++
	return q.extendErr
}

func (q *fakeQueue) FinishJob(_ context.Context, id uuid.UUID, _ string, jobErr error) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.finished == nil {
		q.finished = map[uuid.UUID]error{}
	}
	q.finished[id] = jobErr
	return q.finishErr
}

// fakeListener ends the worker once the queue is drained, through stop.
type fakeListener struct {
	stop context.CancelFunc
	err  error
}

func (fakeListener) Close() {}

func (l fakeListener) Wait(ctx context.Context, _ time.Duration) error {
	if l.err != nil {
		return l.err
	}
	l.stop()
	<-ctx.Done()
	return ctx.Err()
}

func newWorker(q *fakeQueue, handlers map[string]Handler) (*Worker, context.Context, *bytes.Buffer) {
	ctx, cancel := context.WithCancel(context.Background())
	var log bytes.Buffer
	return &Worker{
		Queue:        q,
		Listen:       listen(fakeListener{stop: cancel}, nil),
		ID:           "w1",
		Capabilities: map[string]string{"host": "h"},
		Handlers:     handlers,
		Lease:        30 * time.Millisecond,
		Idle:         time.Second,
		Log:          slog.New(slog.NewTextHandler(&log, nil)),
	}, ctx, &log
}

func job(kind string) *store.Job {
	return &store.Job{ID: uuid.New(), Kind: kind, Payload: json.RawMessage(`{"x":1}`), Attempts: 1}
}

func TestRunsJobsUntilStopped(t *testing.T) {
	ok, bad, unknown := job("ok"), job("bad"), job("nope")
	q := &fakeQueue{jobs: []*store.Job{ok, bad, unknown}}
	var payloads []string
	w, ctx, log := newWorker(q, map[string]Handler{
		"ok": func(_ context.Context, p json.RawMessage) error {
			payloads = append(payloads, string(p))
			return nil
		},
		"bad": func(context.Context, json.RawMessage) error { return errors.New("boom") },
	})
	if err := w.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if q.finished[ok.ID] != nil || q.finished[bad.ID].Error() != "boom" || !strings.Contains(q.finished[unknown.ID].Error(), `unknown job kind "nope"`) {
		t.Fatalf("finished %v", q.finished)
	}
	if len(payloads) != 1 || payloads[0] != `{"x":1}` || q.caps["host"] != "h" {
		t.Fatalf("payloads %v, caps %v", payloads, q.caps)
	}
	if !strings.Contains(log.String(), "job done") || !strings.Contains(log.String(), "job failed") {
		t.Fatalf("log %s", log)
	}
}

func TestHeartbeats(t *testing.T) {
	long := job("long")
	q := &fakeQueue{jobs: []*store.Job{long}}
	w, ctx, _ := newWorker(q, map[string]Handler{
		"long": func(context.Context, json.RawMessage) error {
			time.Sleep(60 * time.Millisecond)
			return nil
		},
	})
	if err := w.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if q.extended == 0 {
		t.Fatal("the lease was never renewed")
	}
}

func TestLostLeaseStopsTheJob(t *testing.T) {
	q := &fakeQueue{jobs: []*store.Job{job("long")}, extendErr: store.ErrLeaseLost, finishErr: store.ErrLeaseLost}
	w, ctx, log := newWorker(q, map[string]Handler{
		"long": func(ctx context.Context, _ json.RawMessage) error {
			<-ctx.Done()
			return ctx.Err()
		},
	})
	if err := w.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(log.String(), "job lease lost") || !strings.Contains(log.String(), "cannot record the job result") {
		t.Fatalf("log %s", log)
	}
}

func TestShutdownFinishesTheRunningJob(t *testing.T) {
	running := job("slow")
	q := &fakeQueue{jobs: []*store.Job{running}}
	w, _, _ := newWorker(q, nil)
	ctx, cancel := context.WithCancel(context.Background())
	w.Handlers = map[string]Handler{"slow": func(jobCtx context.Context, _ json.RawMessage) error {
		cancel() // shutdown requested mid-job
		time.Sleep(10 * time.Millisecond)
		return jobCtx.Err()
	}}
	if err := w.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if err, ok := q.finished[running.ID]; !ok || err != nil {
		t.Fatalf("the job was interrupted: %v (recorded: %v)", err, ok)
	}
}

func TestQueueErrorsStopTheWorker(t *testing.T) {
	q := &fakeQueue{claimErr: errors.New("db down")}
	w, ctx, _ := newWorker(q, nil)
	if err := w.Run(ctx); err == nil || err.Error() != "db down" {
		t.Fatalf("claim: %v", err)
	}

	w, ctx, _ = newWorker(&fakeQueue{}, nil)
	w.Listen = listen(fakeListener{err: errors.New("listen broke")}, nil)
	if err := w.Run(ctx); err == nil || err.Error() != "listen broke" {
		t.Fatalf("wait: %v", err)
	}

	w.Listen = listen(nil, errors.New("no connection"))
	if err := w.Run(ctx); err == nil || err.Error() != "no connection" {
		t.Fatalf("listen: %v", err)
	}
}

func listen(l Listener, err error) func(context.Context) (Listener, error) {
	return func(context.Context) (Listener, error) { return l, err }
}
