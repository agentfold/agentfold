// Package worker takes jobs from the queue and runs them.
package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"orquestrator/internal/store"
)

// Queue is where jobs come from. *store.Store implements it.
type Queue interface {
	ClaimJob(ctx context.Context, worker string, capabilities map[string]string, lease time.Duration) (*store.Job, error)
	ExtendJobLease(ctx context.Context, id uuid.UUID, worker string, lease time.Duration) error
	FinishJob(ctx context.Context, id uuid.UUID, worker string, jobErr error) error
}

// Listener wakes the worker when jobs are enqueued. *store.JobListener
// implements it.
type Listener interface {
	Wait(ctx context.Context, timeout time.Duration) error
	Close()
}

// Handler runs one kind of job.
type Handler func(ctx context.Context, payload json.RawMessage) error

// Worker runs jobs one at a time.
type Worker struct {
	Queue Queue
	// Listen starts listening for new jobs until ctx ends.
	Listen func(ctx context.Context) (Listener, error)
	// ID names the worker in job leases.
	ID string
	// Capabilities filter the jobs the worker takes (jobs.requirements).
	Capabilities map[string]string
	Handlers     map[string]Handler
	// Lease is how long a job stays with the worker without a heartbeat.
	Lease time.Duration
	// Beat is how often heartbeats renew the lease, which is also how soon
	// a job canceled meanwhile stops; Lease/3 when zero.
	Beat time.Duration
	// Idle bounds the wait for new jobs, in case a notification is missed.
	Idle time.Duration
	Log  *slog.Logger
}

// Run takes and runs jobs until ctx ends. A job that is running then is
// finished first: shutting down does not interrupt agents.
func (w *Worker) Run(ctx context.Context) error {
	listener, err := w.Listen(ctx)
	if err != nil {
		return err
	}
	defer listener.Close()
	for ctx.Err() == nil {
		job, err := w.Queue.ClaimJob(ctx, w.ID, w.Capabilities, w.Lease)
		if err == nil && job == nil {
			err = listener.Wait(ctx, w.Idle)
		} else if err == nil {
			w.process(ctx, job)
		}
		if err != nil && ctx.Err() == nil {
			return err
		}
	}
	return nil
}

// process runs a job while renewing its lease, then records its result.
// The job is interrupted only if another worker takes it over.
func (w *Worker) process(ctx context.Context, job *store.Job) {
	jobCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	defer cancel()
	stop := make(chan struct{})
	beats := make(chan struct{})
	go func() {
		defer close(beats)
		w.heartbeat(jobCtx, job, cancel, stop)
	}()

	log := w.Log.With("job", job.ID, "kind", job.Kind, "attempt", job.Attempts)
	log.Info("job started")
	err := w.run(jobCtx, job)
	close(stop)
	<-beats
	if finishErr := w.Queue.FinishJob(context.WithoutCancel(ctx), job.ID, w.ID, err); finishErr != nil {
		log.Error("cannot record the job result", "error", finishErr)
	}
	if err != nil {
		log.Error("job failed", "error", err)
		return
	}
	log.Info("job done")
}

func (w *Worker) run(ctx context.Context, job *store.Job) error {
	handler, ok := w.Handlers[job.Kind]
	if !ok {
		return fmt.Errorf("unknown job kind %q", job.Kind)
	}
	return handler(ctx, job.Payload)
}

// heartbeat renews the lease of job until stop closes; losing the lease
// cancels the job.
func (w *Worker) heartbeat(ctx context.Context, job *store.Job, cancel context.CancelFunc, stop <-chan struct{}) {
	beat := w.Beat
	if beat == 0 {
		beat = w.Lease / 3
	}
	ticker := time.NewTicker(beat)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			if err := w.Queue.ExtendJobLease(ctx, job.ID, w.ID, w.Lease); err != nil {
				w.Log.Error("job lease lost, stopping it", "job", job.ID, "error", err)
				cancel()
				return
			}
		}
	}
}
