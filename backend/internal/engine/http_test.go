package engine

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"orquestrator/internal/domain"
)

// fakeOpener "opens" by dropping the sealed: prefix, or fails with err.
type fakeOpener struct{ err error }

func (o fakeOpener) Open(sealed []byte) ([]byte, error) {
	return []byte(strings.TrimPrefix(string(sealed), "sealed:")), o.err
}

// fakeDoer answers with resp, or fails with err.
type fakeDoer struct {
	resp *http.Response
	err  error
}

func (d fakeDoer) Do(*http.Request) (*http.Response, error) { return d.resp, d.err }

// errReader fails reads, as a connection dropped mid-body.
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errBoom }

// endpoint is a test server that answers with statuses in order (the last
// one repeats) and records the requests it got.
type endpoint struct {
	*httptest.Server
	statuses []int
	body     string
	requests []*http.Request
	bodies   []string
}

func newEndpoint(t *testing.T, body string, statuses ...int) *endpoint {
	t.Helper()
	ep := &endpoint{statuses: statuses, body: body}
	ep.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		ep.requests, ep.bodies = append(ep.requests, r), append(ep.bodies, string(b))
		status := ep.statuses[min(len(ep.requests), len(ep.statuses))-1]
		w.WriteHeader(status)
		_, _ = io.WriteString(w, ep.body)
	}))
	t.Cleanup(ep.Close)
	return ep
}

// httpEnv is an env whose flow is trigger -> h (http_request with cfg),
// success to end and failed to recover when linked.
func httpEnv(t *testing.T, cfg map[string]any, linkFailed bool) *env {
	t.Helper()
	e := newEnv(t)
	e.engine.HTTP, e.engine.Secrets = &http.Client{}, fakeOpener{}
	edges := []string{"t.next>h", "h.success>end", "h.next>end"}
	if linkFailed {
		edges = append(edges, "h.failed>recover")
	}
	e.addFlow("call", graph{
		nodes:   []string{"t:trigger", "h:http_request", "end:end", "recover:move_card"},
		configs: map[string]any{"t": triggerCfg(domain.EventCardCreated, nil), "h": cfg, "recover": map[string]any{"lane_id": e.doing}},
		edges:   edges,
	})
	return e
}

// runHTTP runs the last queued job, an http.request one.
func (e *env) runHTTP() error {
	e.t.Helper()
	job := e.store.jobs[len(e.store.jobs)-1]
	if job.Kind != domain.JobHTTPRequest {
		e.t.Fatalf("last job %+v", job)
	}
	payload, _ := json.Marshal(job.Payload)
	return e.engine.HandleHTTP(context.Background(), payload)
}

func (e *env) httpNodeRun() domain.NodeRun {
	for _, id := range e.store.order {
		if nr := e.store.nodeRuns[id]; nr.NodeKey == "h" {
			return nr
		}
	}
	e.t.Fatal("no http_request node run")
	return domain.NodeRun{}
}

func (e *env) attempts() []httpAttempt {
	var out []httpAttempt
	for _, ev := range e.store.timeline {
		if ev.Type == domain.HTTPRequested {
			var a httpAttempt
			_ = json.Unmarshal(ev.Payload, &a)
			out = append(out, a)
		}
	}
	return out
}

func TestHTTPRequestSucceeds(t *testing.T) {
	ep := newEndpoint(t, `{"ok":true}`, http.StatusCreated)
	e := httpEnv(t, map[string]any{
		"method": "POST", "url": " " + ep.URL + "/cards/{{card.id}}?t={{card.title}} ",
		"body": `{"title":"{{card.title}}"}`,
		"headers": []map[string]any{
			{"name": "Authorization", "secret": true, "sealed": []byte("sealed:Bearer s3cret")},
			{"name": "X-Card", "value": "{{card.id}}"},
		},
	}, false)
	e.card.Title = `a "b"`
	e.store.cards[e.card.ID] = e.card
	e.trigger(e.created())

	job := e.store.jobs[0]
	if job.Kind != domain.JobHTTPRequest || job.MaxAttempts != 2 || job.Delay != 0 || job.Payload.(domain.HTTPJob).Attempt != 1 {
		t.Fatalf("job %+v", job)
	}
	if err := e.runHTTP(); err != nil {
		t.Fatal(err)
	}
	r := ep.requests[0]
	if r.Method != "POST" || r.URL.Path != "/cards/"+e.card.ID.String() || r.URL.Query().Get("t") != `a "b"` {
		t.Fatalf("request %s %s", r.Method, r.URL)
	}
	if r.Header.Get("Authorization") != "Bearer s3cret" || r.Header.Get("X-Card") != e.card.ID.String() || r.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("headers %v", r.Header)
	}
	if ep.bodies[0] != `{"title":"a \"b\""}` {
		t.Fatalf("body %s", ep.bodies[0])
	}
	nr := e.httpNodeRun()
	if nr.Status != domain.NodeDone || nr.OutputHandle != "success" || string(nr.Output) != `{"output":{"body":{"ok":true},"status":201}}` {
		t.Fatalf("node run %+v (%s)", nr, nr.Output)
	}
	if run := e.onlyRun(); run.Status != domain.RunDone {
		t.Fatalf("run %+v", run)
	}
	e.wantTimeline("flow.started", "node.started h", "http.requested h", "node.finished h done",
		"node.started end", "node.finished end done", "flow.finished done")
	if a := e.attempts()[0]; a.Attempt != 1 || a.MaxAttempts != 1 || a.Status != 201 || a.Method != "POST" || !strings.HasPrefix(a.URL, ep.URL+"/cards/") {
		t.Fatalf("attempt %+v", a)
	}
}

func TestHTTPRequestRetries(t *testing.T) {
	ep := newEndpoint(t, "", http.StatusServiceUnavailable, http.StatusTooManyRequests, http.StatusOK)
	e := httpEnv(t, map[string]any{
		"method": "GET", "url": ep.URL, "max_attempts": 3, "retry_delay_seconds": 5, "backoff": "exponential",
		"headers": []map[string]any{{"name": "Content-Type", "value": "text/plain"}},
	}, false)
	e.trigger(e.created())
	for range 2 {
		if err := e.runHTTP(); err != nil {
			t.Fatal(err)
		}
	}
	delays := []time.Duration{e.store.jobs[1].Delay, e.store.jobs[2].Delay}
	if !slices.Equal(delays, []time.Duration{5 * time.Second, 10 * time.Second}) || e.store.jobs[2].Payload.(domain.HTTPJob).Attempt != 3 {
		t.Fatalf("jobs %+v", e.store.jobs)
	}
	if nr := e.httpNodeRun(); nr.Status != domain.NodeQueued {
		t.Fatalf("the node left while retrying: %+v", nr)
	}
	if err := e.runHTTP(); err != nil {
		t.Fatal(err)
	}
	if nr := e.httpNodeRun(); nr.OutputHandle != "success" || string(nr.Output) != `{"output":{"body":null,"status":200}}` {
		t.Fatalf("node run %+v (%s)", nr, nr.Output)
	}
	got := e.attempts()
	if len(got) != 3 || got[0].RetryInSeconds != 5 || got[1].RetryInSeconds != 10 || got[2].RetryInSeconds != 0 || got[1].Status != 429 {
		t.Fatalf("attempts %+v", got)
	}
	if ep.requests[0].Header.Get("Content-Type") != "text/plain" {
		t.Fatal("the content type was replaced")
	}
}

func TestHTTPRequestFails(t *testing.T) {
	t.Run("out of attempts", func(t *testing.T) {
		ep := newEndpoint(t, "down", http.StatusBadGateway)
		e := httpEnv(t, map[string]any{"method": "GET", "url": ep.URL, "max_attempts": 2, "retry_delay_seconds": 1}, true)
		e.trigger(e.created())
		for range 2 {
			if err := e.runHTTP(); err != nil {
				t.Fatal(err)
			}
		}
		nr := e.httpNodeRun()
		if nr.Status != domain.NodeFailed || nr.OutputHandle != "failed" || nr.Error != "a chamada respondeu 502 (2 tentativas)" ||
			string(nr.Output) != `{"output":{"body":"down","status":502}}` {
			t.Fatalf("node run %+v (%s)", nr, nr.Output)
		}
		if e.store.cards[e.card.ID].LaneID != e.doing {
			t.Fatal("the failed output was not followed")
		}
	})
	t.Run("not worth retrying", func(t *testing.T) {
		ep := newEndpoint(t, "", http.StatusNotFound)
		e := httpEnv(t, map[string]any{"method": "GET", "url": ep.URL, "max_attempts": 5}, false)
		e.trigger(e.created())
		if err := e.runHTTP(); err != nil {
			t.Fatal(err)
		}
		if nr := e.httpNodeRun(); nr.Error != "a chamada respondeu 404" || len(e.store.jobs) != 1 {
			t.Fatalf("node run %+v, jobs %d", nr, len(e.store.jobs))
		}
		if run := e.onlyRun(); run.Status != domain.RunFailed || run.Error != "a chamada respondeu 404" {
			t.Fatalf("run %+v", run)
		}
	})
	t.Run("network error", func(t *testing.T) {
		e := httpEnv(t, map[string]any{"method": "GET", "url": "http://x.test", "max_attempts": 2}, false)
		e.engine.HTTP = fakeDoer{err: errBoom}
		e.trigger(e.created())
		if err := e.runHTTP(); err != nil {
			t.Fatal(err)
		}
		if len(e.store.jobs) != 2 || e.store.jobs[1].Delay != domain.DefaultRetryDelay {
			t.Fatalf("network errors are retried: %+v", e.store.jobs)
		}
		if err := e.runHTTP(); err != nil {
			t.Fatal(err)
		}
		if nr := e.httpNodeRun(); !strings.HasPrefix(nr.Error, "a chamada falhou: ") || !strings.HasSuffix(nr.Error, "boom (2 tentativas)") {
			t.Fatalf("node run %+v", nr)
		}
	})
	t.Run("body cut", func(t *testing.T) {
		e := httpEnv(t, map[string]any{"method": "GET", "url": "http://x.test"}, false)
		e.engine.HTTP = fakeDoer{resp: &http.Response{StatusCode: 200, Body: io.NopCloser(errReader{})}}
		e.trigger(e.created())
		if err := e.runHTTP(); err != nil {
			t.Fatal(err)
		}
		if nr := e.httpNodeRun(); nr.Error != "a chamada falhou: boom" || e.attempts()[0].Status != 0 {
			t.Fatalf("node run %+v", nr)
		}
	})
	t.Run("bad request", func(t *testing.T) {
		e := httpEnv(t, map[string]any{"method": "GET", "url": "http://a b"}, false)
		e.trigger(e.created())
		if err := e.runHTTP(); err != nil {
			t.Fatal(err)
		}
		if nr := e.httpNodeRun(); !strings.HasPrefix(nr.Error, "a chamada falhou: ") {
			t.Fatalf("node run %+v", nr)
		}
	})
	t.Run("sealed header", func(t *testing.T) {
		e := httpEnv(t, map[string]any{"method": "GET", "url": "http://x.test",
			"headers": []map[string]any{{"name": "Token", "secret": true, "sealed": []byte("x")}}}, false)
		e.engine.Secrets = fakeOpener{err: errBoom}
		e.trigger(e.created())
		if err := e.runHTTP(); err != nil {
			t.Fatal(err)
		}
		if nr := e.httpNodeRun(); nr.Error != "a chamada falhou: o header Token: boom" {
			t.Fatalf("node run %+v", nr)
		}
	})
}

func TestHTTPRequestFireAndForget(t *testing.T) {
	ep := newEndpoint(t, "", http.StatusInternalServerError)
	e := httpEnv(t, map[string]any{"method": "POST", "url": ep.URL, "mode": "fire_and_forget", "max_attempts": 3}, false)
	e.trigger(e.created())
	if run := e.onlyRun(); run.Status != domain.RunDone {
		t.Fatalf("the run waited: %+v", run)
	}
	if err := e.runHTTP(); err != nil {
		t.Fatal(err)
	}
	if len(ep.requests) != 1 || len(e.store.jobs) != 1 {
		t.Fatalf("fire and forget tries once: %d requests, %d jobs", len(ep.requests), len(e.store.jobs))
	}
	if nr := e.httpNodeRun(); nr.OutputHandle != "next" || nr.Status != domain.NodeDone {
		t.Fatalf("node run %+v", nr)
	}
	if a := e.attempts(); len(a) != 1 || a[0].Status != 500 || a[0].MaxAttempts != 1 {
		t.Fatalf("attempts %+v", a)
	}
}

func TestHTTPRequestOfAnEndedRun(t *testing.T) {
	ep := newEndpoint(t, "", http.StatusOK)
	e := httpEnv(t, map[string]any{"method": "GET", "url": ep.URL}, false)
	e.trigger(e.created())
	run := e.onlyRun()
	run.Status = domain.RunCanceled
	e.store.runs[run.ID] = run
	if err := e.runHTTP(); err != nil || len(ep.requests) != 0 {
		t.Fatalf("canceled run: %v, %d requests", err, len(ep.requests))
	}

	e = httpEnv(t, map[string]any{"method": "GET", "url": ep.URL}, false)
	e.trigger(e.created())
	nr := e.httpNodeRun()
	nr.Status = domain.NodeFailed
	e.store.nodeRuns[nr.ID] = nr
	if err := e.runHTTP(); err != nil || len(ep.requests) != 0 {
		t.Fatalf("ended node run: %v, %d requests", err, len(ep.requests))
	}
}

func TestHTTPRequestErrors(t *testing.T) {
	if err := (&Engine{}).HandleHTTP(context.Background(), json.RawMessage(`[`)); err == nil {
		t.Error("bad payload accepted")
	}
	ep := newEndpoint(t, "", http.StatusServiceUnavailable)
	cfg := map[string]any{"method": "GET", "url": ep.URL, "max_attempts": 2}
	for _, name := range []string{"GetNodeRun", "GetFlowRun", "GetFlowVersion", "GetCard", "ListNodeRuns", "AddCardEvent:http.requested", "EnqueueJob"} {
		t.Run(name, func(t *testing.T) {
			e := httpEnv(t, cfg, false)
			e.trigger(e.created())
			e.store.errs[name] = errBoom
			if err := e.runHTTP(); !errors.Is(err, errBoom) {
				t.Fatalf("got %v", err)
			}
		})
	}
	t.Run("FinishNodeRun", func(t *testing.T) {
		e := httpEnv(t, map[string]any{"method": "GET", "url": ep.URL}, false)
		e.trigger(e.created())
		e.store.errs["FinishNodeRun"] = errBoom
		if err := e.runHTTP(); !errors.Is(err, errBoom) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("deleted card", func(t *testing.T) {
		e := httpEnv(t, cfg, false)
		e.trigger(e.created())
		e.store.errs["GetNodeRun"] = domain.ErrNotFound
		if err := e.runHTTP(); err != nil {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("queueing the first attempt", func(t *testing.T) {
		e := httpEnv(t, cfg, false)
		e.store.errs["EnqueueJob"] = errBoom
		if err := e.engine.HandleTrigger(context.Background(), e.created()); !errors.Is(err, errBoom) {
			t.Fatalf("got %v", err)
		}
	})
}

func TestResponseBody(t *testing.T) {
	for body, want := range map[string]string{"": "null", " \n": "null", ` {"a":1} `: `{"a":1}`, "oi": `"oi"`, "{bad": `"{bad"`} {
		if got := string(responseBody([]byte(body))); got != want {
			t.Errorf("%q = %s, want %s", body, got, want)
		}
	}
}
