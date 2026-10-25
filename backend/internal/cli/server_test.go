package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"orquestrator/internal/crypto"
)

// dsn points to the Postgres started for this package; each test that needs
// a database gets its own (see freshDatabase).
var dsn string

func TestMain(m *testing.M) {
	ctx := context.Background()
	ctr, err := postgres.Run(ctx, "postgres:18-alpine",
		postgres.WithDatabase("orquestrator"),
		postgres.BasicWaitStrategies(),
	)
	if err == nil {
		dsn, err = ctr.ConnectionString(ctx, "sslmode=disable")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "start postgres:", err)
		os.Exit(1)
	}
	code := m.Run()
	_ = testcontainers.TerminateContainer(ctr)
	os.Exit(code)
}

// freshDatabase creates an empty database, runs setup statements in it and
// returns its URL.
func freshDatabase(t *testing.T, setup ...string) string {
	t.Helper()
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close(ctx) }()
	name := "t" + strings.ReplaceAll(strings.ToLower(t.Name()), "/", "_")
	if _, err := admin.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)"); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	url := strings.Replace(dsn, "/orquestrator?", "/"+name+"?", 1)

	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(ctx) }()
	for _, stmt := range setup {
		if _, err := conn.Exec(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	return url
}

// syncBuffer is a bytes.Buffer safe for a writer goroutine and a reader.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func run(args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = Run(context.Background(), args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestCommandErrors(t *testing.T) {
	tests := []struct {
		name, url, addr string
		args            []string
		want            string
	}{
		{"migrate args", "", "", []string{"migrate", "x"}, "unexpected arguments"},
		{"server args", "", "", []string{"server", "x"}, "unexpected arguments"},
		{"no database url", "", "", []string{"migrate"}, "ORQ_DATABASE_URL is not set"},
		{"malformed database url", "postgres://%zz", "", []string{"server"}, "ORQ_DATABASE_URL:"},
		{"unreachable database", "postgres://u:p@127.0.0.1:1/db?connect_timeout=2", "", []string{"migrate"}, "connect to database"},
		{"bad address", dsn, "127.0.0.1:not-a-port", []string{"server"}, "not-a-port"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(envSecretKey, crypto.NewKey())
			t.Setenv(envDatabaseURL, tt.url)
			t.Setenv(envAddr, tt.addr)
			code, _, stderr := run(tt.args...)
			if code != ExitError || !strings.Contains(stderr, tt.want) {
				t.Fatalf("code %d, stderr %q; want %q", code, stderr, tt.want)
			}
		})
	}
}

func TestSecretKeyErrors(t *testing.T) {
	t.Setenv(envSecretKey, "")
	if code, _, stderr := run("server"); code != ExitError || !strings.Contains(stderr, "ORQ_SECRET_KEY is not set") {
		t.Fatalf("missing key: %d %q", code, stderr)
	}
	t.Setenv(envSecretKey, "short")
	if code, _, stderr := run("server"); code != ExitError || !strings.Contains(stderr, "ORQ_SECRET_KEY: the secret key must be") {
		t.Fatalf("bad key: %d %q", code, stderr)
	}
}

func TestMigrateFailure(t *testing.T) {
	t.Setenv(envDatabaseURL, freshDatabase(t, "CREATE TABLE lanes (id int)"))
	if code, _, stderr := run("migrate"); code != ExitError || !strings.Contains(stderr, "lanes") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}

func TestServeReportsListenerErrors(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_ = ln.Close()
	if err := serve(context.Background(), ln, http.NotFoundHandler()); err == nil {
		t.Fatal("expected an error from a closed listener")
	}
}

// TestServerEndToEnd migrates a database, starts the server on a random port,
// uses the API and stops the server by cancelling its context.
func TestServerEndToEnd(t *testing.T) {
	t.Setenv(envSecretKey, crypto.NewKey())
	t.Setenv(envDatabaseURL, freshDatabase(t))
	if code, stdout, stderr := run("migrate"); code != ExitOK || stdout != "migrations applied\n" {
		t.Fatalf("migrate: code %d, stdout %q, stderr %q", code, stdout, stderr)
	}

	t.Setenv(envAddr, "127.0.0.1:0")
	ctx, cancel := context.WithCancel(context.Background())
	var stdout, stderr syncBuffer
	done := make(chan int)
	go func() { done <- Run(ctx, []string{"server"}, &stdout, &stderr) }()

	base := waitListening(t, &stdout)
	events := openEvents(t, base)

	lane := post(t, base+"/api/lanes", `{"name":"Todo"}`)
	cardType := post(t, base+"/api/card-types", `{"name":"task"}`)
	card := post(t, base+"/api/cards", fmt.Sprintf(`{"title":"First","lane_id":%q,"card_type_id":%q}`, lane["id"], cardType["id"]))
	if card["position"] != 0.0 || card["project_id"] != nil {
		t.Fatalf("unexpected card %v", card)
	}
	for _, want := range []string{"event: lane.created", "event: card_type.created", "event: card.created"} {
		if got := nextEvent(t, events); got != want {
			t.Fatalf("event %q, want %q", got, want)
		}
	}

	cancel()
	select {
	case code := <-done:
		if code != ExitOK {
			t.Fatalf("server exit code %d, stderr %q", code, stderr.String())
		}
	case <-time.After(15 * time.Second):
		t.Fatal("server did not stop")
	}
}

func waitListening(t *testing.T, stdout *syncBuffer) string {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if out := stdout.String(); strings.HasPrefix(out, "listening on ") {
			return strings.TrimSpace(strings.TrimPrefix(out, "listening on "))
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("server did not start: %q", stdout.String())
	return ""
}

func post(t *testing.T, url, body string) map[string]any {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST %s: %d %s", url, resp.StatusCode, data)
	}
	var v map[string]any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

// openEvents subscribes to the event stream; the stream is closed when the
// server stops.
func openEvents(t *testing.T, base string) chan string {
	t.Helper()
	resp, err := http.Get(base + "/api/events") //nolint:bodyclose // closed by the reader goroutine below
	if err != nil {
		t.Fatal(err)
	}
	lines := make(chan string, 16)
	go func() {
		defer func() { _ = resp.Body.Close() }()
		data := make([]byte, 4096)
		var pending string
		for {
			n, err := resp.Body.Read(data)
			pending += string(data[:n])
			for {
				i := strings.Index(pending, "\n")
				if i < 0 {
					break
				}
				lines <- pending[:i]
				pending = pending[i+1:]
			}
			if err != nil {
				close(lines)
				return
			}
		}
	}()
	return lines
}

// nextEvent returns the next "event:" line of the stream.
func nextEvent(t *testing.T, lines chan string) string {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case line, ok := <-lines:
			if !ok {
				t.Fatal("stream closed")
			}
			if strings.HasPrefix(line, "event: ") {
				return line
			}
		case <-timeout:
			t.Fatal("no event")
		}
	}
}
