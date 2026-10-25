package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"orquestrator/internal/crypto"
	"orquestrator/internal/events"
	"orquestrator/internal/gitrepo"
	"orquestrator/internal/service"
	"orquestrator/internal/store"
	"orquestrator/internal/transport/rest"
	"orquestrator/migrations"
)

// Environment variables read by the subcommands.
const (
	envDatabaseURL = "ORQ_DATABASE_URL"
	envAddr        = "ORQ_ADDR"
	envSecretKey   = "ORQ_SECRET_KEY" //nolint:gosec // a variable name, not a credential
)

// defaultAddr listens on loopback only: the API has no authentication yet.
const defaultAddr = "127.0.0.1:8080"

const (
	eventBuffer     = 64
	sseHeartbeat    = 15 * time.Second
	relayRetry      = 5 * time.Second
	shutdownTimeout = 10 * time.Second
)

var errUnexpectedArgs = errors.New("unexpected arguments")

// connect opens the database named by ORQ_DATABASE_URL and checks it answers.
func connect(ctx context.Context) (*pgxpool.Pool, error) {
	url := os.Getenv(envDatabaseURL)
	if url == "" {
		return nil, fmt.Errorf("%s is not set", envDatabaseURL)
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", envDatabaseURL, err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	return pool, nil
}

// secretBox opens the key in ORQ_SECRET_KEY, which seals agent credentials
// and MCP secrets.
func secretBox() (*crypto.Box, error) {
	key := os.Getenv(envSecretKey)
	if key == "" {
		return nil, fmt.Errorf("%s is not set (generate one with `orquestrator secret-key`)", envSecretKey)
	}
	box, err := crypto.NewBox(key)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", envSecretKey, err)
	}
	return box, nil
}

func runMigrate(ctx context.Context, args []string, stdout, _ io.Writer) error {
	if len(args) > 0 {
		return errUnexpectedArgs
	}
	pool, err := connect(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	db := stdlib.OpenDBFromPool(pool)
	defer func() { _ = db.Close() }()
	if err := migrations.Up(ctx, db); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(stdout, "migrations applied")
	return nil
}

func runServer(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) > 0 {
		return errUnexpectedArgs
	}
	addr := os.Getenv(envAddr)
	if addr == "" {
		addr = defaultAddr
	}
	box, err := secretBox()
	if err != nil {
		return err
	}
	pool, err := connect(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	log := slog.New(slog.NewTextHandler(stderr, nil))
	st := store.New(pool)
	hub := events.NewHub(eventBuffer)
	board := service.New(st, gitrepo.Inspector{}, hub, box)
	handler := rest.NewHandler(board, hub, rest.Options{Log: log, Heartbeat: sseHeartbeat})
	relay := &events.Relay{
		Listen: func(ctx context.Context) (events.Changes, error) { return st.ListenChanges(ctx) },
		Load:   board.Entity,
		Hub:    hub,
		Retry:  relayRetry,
		Log:    log,
	}
	relayCtx, stopRelay := context.WithCancel(ctx)
	defer stopRelay()
	go relay.Run(relayCtx)

	ln, err := new(net.ListenConfig).Listen(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "listening on http://%s\n", ln.Addr())
	return serve(ctx, ln, handler)
}

// serve answers HTTP requests on ln until ctx is cancelled, then shuts down.
// Request contexts derive from ctx, so open event streams end too.
func serve(ctx context.Context, ln net.Listener, handler http.Handler) error {
	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		<-ctx.Done()
		sctx, scancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
		defer scancel()
		_ = srv.Shutdown(sctx)
	}()
	if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
