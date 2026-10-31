package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
	"orquestrator/internal/store"
)

// Doer sends HTTP requests. *http.Client implements it.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Opener decrypts the sealed values of secret headers. *crypto.Box
// implements it.
type Opener interface {
	Open(sealed []byte) ([]byte, error)
}

// queueHTTP queues an attempt of the request of an http_request node run.
func (e *Engine) queueHTTP(ctx context.Context, nodeRunID uuid.UUID, attempt int, delay time.Duration) error {
	// MaxAttempts 2: a worker that died mid-request leaves the attempt to another.
	_, err := e.Store.EnqueueJob(ctx, store.NewJob{
		Kind: domain.JobHTTPRequest, Payload: domain.HTTPJob{NodeRunID: nodeRunID, Attempt: attempt}, MaxAttempts: 2, Delay: delay,
	})
	return err
}

// fireAndForget tells whether an http_request node goes on without waiting.
func fireAndForget(node domain.Node) bool {
	var cfg domain.HTTPRequestConfig
	_ = json.Unmarshal(node.Config, &cfg) // validated when published
	return cfg.FireAndForget()
}

// HandleHTTP runs http.request jobs: it makes an attempt of the request of
// an http_request node run. In wait mode the node leaves by success, tries
// again later, or leaves by failed when it should not or has no attempts
// left; a run that ended meanwhile drops the attempt. Firing and
// forgetting, the node went on already and the attempt is only recorded.
func (e *Engine) HandleHTTP(ctx context.Context, payload json.RawMessage) error {
	var p domain.HTTPJob
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("http.request payload: %w", err)
	}
	nr, err := e.Store.GetNodeRun(ctx, p.NodeRunID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil // the card was deleted meanwhile
	}
	if err != nil {
		return err
	}
	fr, err := e.Store.GetFlowRun(ctx, nr.FlowRunID)
	if err != nil {
		return err
	}
	version, err := e.Store.GetFlowVersion(ctx, fr.FlowVersionID)
	if err != nil {
		return err
	}
	node, _ := findNode(version.Graph, nr.NodeKey) // the run entered it
	var cfg domain.HTTPRequestConfig
	_ = json.Unmarshal(node.Config, &cfg) // validated when published
	pending := nr.Status == domain.NodeQueued || nr.Status == domain.NodeRunning
	if !cfg.FireAndForget() && (!fr.Status.Active() || !pending) {
		return nil // canceled meanwhile
	}
	data, err := e.templateData(ctx, fr, nr.Input)
	if err != nil {
		return err
	}

	res := e.call(ctx, cfg, data)
	attempt := httpAttempt{
		FlowRunID: fr.ID, NodeKey: nr.NodeKey, Attempt: p.Attempt, MaxAttempts: cfg.Attempts(),
		Method: cfg.Method, URL: res.url, Status: res.status, Error: res.err, DurationMS: res.duration.Milliseconds(),
	}
	ok := res.err == "" && cfg.Succeeded(res.status)
	retry := !ok && p.Attempt < cfg.Attempts() && (res.status == 0 || domain.Retryable(res.status))
	if retry {
		attempt.RetryInSeconds = int(cfg.RetryDelay(p.Attempt).Seconds())
	}
	if err := e.timeline(ctx, fr.CardID, domain.HTTPRequested, attempt); err != nil || cfg.FireAndForget() {
		return err
	}
	if retry {
		return e.queueHTTP(ctx, nr.ID, p.Attempt+1, cfg.RetryDelay(p.Attempt))
	}
	result := domain.NodeResult{Status: domain.NodeDone, OutputHandle: "success", Output: res.output()}
	if !ok {
		result = domain.NodeResult{Status: domain.NodeFailed, OutputHandle: "failed", Output: res.output(), Error: res.failure(p.Attempt)}
	}
	if _, err := e.Store.FinishNodeRun(ctx, nr.ID, result); err != nil {
		return err
	}
	return e.continueFrom(ctx, nr.ID)
}

// httpResult is how an attempt went: the response status and body, or why
// there was none.
type httpResult struct {
	url      string
	status   int
	body     json.RawMessage
	err      string
	duration time.Duration
}

// output is what the node hands on: the status and the body, as JSON when
// it is JSON and as text otherwise.
func (r httpResult) output() json.RawMessage {
	out, _ := json.Marshal(map[string]any{ // plain values: marshals
		"output": map[string]any{"status": r.status, "body": r.body},
	})
	return out
}

// failure is the error of the node run when the attempt was its last.
func (r httpResult) failure(attempts int) string {
	msg := fmt.Sprintf("a chamada respondeu %d", r.status)
	if r.err != "" {
		msg = "a chamada falhou: " + r.err
	}
	if attempts > 1 {
		msg += fmt.Sprintf(" (%d tentativas)", attempts)
	}
	return msg
}

// call makes the request of cfg, with what its placeholders read.
func (e *Engine) call(ctx context.Context, cfg domain.HTTPRequestConfig, data domain.TemplateData) httpResult {
	res := httpResult{url: domain.RenderTemplate(strings.TrimSpace(cfg.URL), data, domain.EscapeURL)}
	req, err := e.request(ctx, cfg, data, res.url)
	if err != nil {
		res.err = err.Error()
		return res
	}
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout())
	defer cancel()
	start := time.Now()
	resp, err := e.HTTP.Do(req.WithContext(ctx))
	if err == nil {
		defer func() { _ = resp.Body.Close() }()
		var body []byte
		body, err = io.ReadAll(io.LimitReader(resp.Body, domain.MaxHTTPResponseBody))
		res.status, res.body = resp.StatusCode, responseBody(body)
	}
	res.duration = time.Since(start)
	if err != nil {
		res.status, res.err = 0, err.Error()
	}
	return res
}

// request builds the request of cfg; a JSON content type is the default
// when it has a body.
func (e *Engine) request(ctx context.Context, cfg domain.HTTPRequestConfig, data domain.TemplateData, url string) (*http.Request, error) {
	var body io.Reader
	if cfg.Body != "" {
		body = strings.NewReader(domain.RenderTemplate(cfg.Body, data, domain.EscapeJSON))
	}
	req, err := http.NewRequestWithContext(ctx, cfg.Method, url, body)
	if err != nil {
		return nil, err
	}
	for _, h := range cfg.Headers {
		value := h.Value
		if h.Secret {
			plain, err := e.Secrets.Open(h.Sealed)
			if err != nil {
				return nil, fmt.Errorf("o header %s: %w", h.Name, err)
			}
			value = string(plain)
		}
		req.Header.Set(h.Name, domain.RenderTemplate(value, data, domain.EscapeHeader))
	}
	if body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

// responseBody is the body as JSON when it is JSON, as a JSON string
// otherwise; null when empty.
func responseBody(body []byte) json.RawMessage {
	trimmed := bytes.TrimSpace(body)
	switch {
	case len(trimmed) == 0:
		return json.RawMessage("null")
	case json.Valid(trimmed):
		return trimmed
	}
	text, _ := json.Marshal(string(body)) // strings marshal
	return text
}

// httpAttempt is the timeline entry of an attempt.
type httpAttempt struct {
	FlowRunID   uuid.UUID `json:"flow_run_id"`
	NodeKey     string    `json:"node_key"`
	Attempt     int       `json:"attempt"`
	MaxAttempts int       `json:"max_attempts"`
	Method      string    `json:"method"`
	URL         string    `json:"url"`
	Status      int       `json:"status,omitempty"`
	Error       string    `json:"error,omitempty"`
	DurationMS  int64     `json:"duration_ms"`
	// RetryInSeconds is when the next attempt goes, if there is one.
	RetryInSeconds int `json:"retry_in_seconds,omitempty"`
}
