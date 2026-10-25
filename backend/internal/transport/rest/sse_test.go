package rest

import (
	"bufio"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"orquestrator/internal/events"
)

// openStream starts a server and subscribes to its event stream. It returns
// a reader over the stream and a function that disconnects the client.
func openStream(t *testing.T, hub *events.Hub, heartbeat time.Duration) (*bufio.Reader, func()) {
	t.Helper()
	srv := httptest.NewServer(NewHandler(newStub(), hub, Options{Heartbeat: heartbeat}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/events", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type %q", ct)
	}
	return bufio.NewReader(resp.Body), cancel
}

// readMessage reads the lines of one SSE message.
func readMessage(t *testing.T, r *bufio.Reader) []string {
	t.Helper()
	var lines []string
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("read: %v (got %q)", err, lines)
		}
		line = strings.TrimSuffix(line, "\n")
		if line == "" {
			return lines
		}
		lines = append(lines, line)
	}
}

func TestEventStreamSendsEvents(t *testing.T) {
	hub := events.NewHub(8)
	r, _ := openStream(t, hub, time.Hour)
	b := newStub()

	hub.Publish(events.Event{Type: "lane.created", ID: id1, Data: b.lane})
	got := readMessage(t, r)
	want := `{"type":"lane.created","id":"` + id1.String() + `","data":` + laneJSON + `}`
	if len(got) != 2 || got[0] != "event: lane.created" || !strings.HasPrefix(got[1], "data: ") {
		t.Fatalf("got %q", got)
	}
	if !reflect.DeepEqual(decode(t, []byte(strings.TrimPrefix(got[1], "data: "))), decode(t, []byte(want))) {
		t.Fatalf("data %s\nwant %s", got[1], want)
	}

	hub.Publish(events.Event{Type: "card.deleted", ID: id2})
	got = readMessage(t, r)
	if got[1] != `data: {"type":"card.deleted","id":"`+id2.String()+`"}` {
		t.Fatalf("got %q", got)
	}
}

func TestEventStreamHeartbeat(t *testing.T) {
	r, _ := openStream(t, events.NewHub(1), 5*time.Millisecond)
	if got := readMessage(t, r); len(got) != 1 || got[0] != ": ping" {
		t.Fatalf("got %q", got)
	}
}

// closedHub hands out subscriptions that have already ended, as the hub
// does for subscribers that fall behind.
type closedHub struct{}

func (closedHub) Subscribe() (<-chan events.Event, func()) {
	ch := make(chan events.Event)
	close(ch)
	return ch, func() {}
}

func TestEventStreamEndsWhenSubscriptionEnds(t *testing.T) {
	rec := httptest.NewRecorder()
	eventStream{hub: closedHub{}, heartbeat: time.Hour}.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/events", http.NoBody))
	if rec.Code != http.StatusOK || rec.Body.Len() != 0 {
		t.Fatalf("status %d, body %q", rec.Code, rec.Body)
	}
}

func TestEventStreamEndsWhenClientLeaves(t *testing.T) {
	_, disconnect := openStream(t, events.NewHub(1), time.Hour)
	disconnect() // srv.Close in the cleanup waits for the handler to return
}

// failingWriter is a ResponseWriter whose flushes start failing after ok
// successful ones.
type failingWriter struct {
	httptest.ResponseRecorder
	ok int
}

func (w *failingWriter) FlushError() error {
	if w.ok == 0 {
		return errors.New("flush failed")
	}
	w.ok--
	return nil
}

func TestEventStreamStopsWhenFlushFails(t *testing.T) {
	for _, ok := range []int{0, 1} {
		hub := events.NewHub(1)
		w := &failingWriter{ResponseRecorder: *httptest.NewRecorder(), ok: ok}
		stream := eventStream{hub: hub, heartbeat: time.Millisecond}
		stream.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/events", http.NoBody))
	}
}
