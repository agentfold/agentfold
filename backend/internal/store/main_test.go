package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"orquestrator/internal/store/sqlcgen"
	"orquestrator/migrations"
)

// pool is shared by the tests of this package: one Postgres container per run.
var pool *pgxpool.Pool

func TestMain(m *testing.M) {
	code, err := run(m)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		code = 1
	}
	os.Exit(code)
}

func run(m *testing.M) (int, error) {
	ctx := context.Background()
	ctr, err := postgres.Run(ctx, "postgres:18-alpine",
		postgres.WithDatabase("orquestrator"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		return 0, fmt.Errorf("start postgres: %w", err)
	}
	defer func() { _ = testcontainers.TerminateContainer(ctr) }()

	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return 0, err
	}
	pool, err = pgxpool.New(ctx, dsn)
	if err != nil {
		return 0, err
	}
	defer pool.Close()

	db := stdlib.OpenDBFromPool(pool)
	defer func() { _ = db.Close() }()
	if err := migrations.Up(ctx, db); err != nil {
		return 0, fmt.Errorf("migrate: %w", err)
	}
	return m.Run(), nil
}

// newStore returns a Store over an empty database.
func newStore(t *testing.T) *Store {
	t.Helper()
	_, err := pool.Exec(context.Background(), "TRUNCATE jobs, review_comments, questions, agent_sessions, node_runs, flow_runs, mcp_servers, agent_profiles, flow_versions, flows, card_events, cards, card_types, projects, lanes")
	if err != nil {
		t.Fatal(err)
	}
	return New(pool)
}

// cancelled returns a context that is already cancelled, which makes every
// database call fail.
func cancelled() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

var errInjected = errors.New("injected")

// failing wraps the transaction querier and fails the n-th call (1-based) of
// one method.
type failing struct {
	sqlcgen.Querier
	method string
	n      int
	calls  map[string]int
}

// failOn makes the transactions of s fail on the n-th call of method.
func failOn(s *Store, method string, n int) {
	s.txQuerier = func(tx pgx.Tx) sqlcgen.Querier {
		return &failing{Querier: sqlcgen.New(tx), method: method, n: n, calls: map[string]int{}}
	}
}

func (f *failing) hit(method string) error {
	f.calls[method]++
	if method == f.method && f.calls[method] == f.n {
		return errInjected
	}
	return nil
}

func (f *failing) CountLanes(ctx context.Context) (int32, error) {
	if err := f.hit("CountLanes"); err != nil {
		return 0, err
	}
	return f.Querier.CountLanes(ctx)
}

func (f *failing) GetLaneForUpdate(ctx context.Context, id uuid.UUID) (sqlcgen.Lane, error) {
	if err := f.hit("GetLaneForUpdate"); err != nil {
		return sqlcgen.Lane{}, err
	}
	return f.Querier.GetLaneForUpdate(ctx, id)
}

func (f *failing) ShiftLanes(ctx context.Context, arg sqlcgen.ShiftLanesParams) error {
	if err := f.hit("ShiftLanes"); err != nil {
		return err
	}
	return f.Querier.ShiftLanes(ctx, arg)
}

func (f *failing) SetLanePosition(ctx context.Context, arg sqlcgen.SetLanePositionParams) (sqlcgen.Lane, error) {
	if err := f.hit("SetLanePosition"); err != nil {
		return sqlcgen.Lane{}, err
	}
	return f.Querier.SetLanePosition(ctx, arg)
}

func (f *failing) DeleteLane(ctx context.Context, id uuid.UUID) (int64, error) {
	if err := f.hit("DeleteLane"); err != nil {
		return 0, err
	}
	return f.Querier.DeleteLane(ctx, id)
}

func (f *failing) CountCardsInLane(ctx context.Context, laneID uuid.UUID) (int32, error) {
	if err := f.hit("CountCardsInLane"); err != nil {
		return 0, err
	}
	return f.Querier.CountCardsInLane(ctx, laneID)
}

func (f *failing) InsertCardEvent(ctx context.Context, arg sqlcgen.InsertCardEventParams) (sqlcgen.CardEvent, error) {
	if err := f.hit("InsertCardEvent"); err != nil {
		return sqlcgen.CardEvent{}, err
	}
	return f.Querier.InsertCardEvent(ctx, arg)
}

func (f *failing) ShiftCards(ctx context.Context, arg sqlcgen.ShiftCardsParams) error {
	if err := f.hit("ShiftCards"); err != nil {
		return err
	}
	return f.Querier.ShiftCards(ctx, arg)
}

func (f *failing) PlaceCard(ctx context.Context, arg sqlcgen.PlaceCardParams) (sqlcgen.Card, error) {
	if err := f.hit("PlaceCard"); err != nil {
		return sqlcgen.Card{}, err
	}
	return f.Querier.PlaceCard(ctx, arg)
}

func (f *failing) NextFlowVersion(ctx context.Context, flowID uuid.UUID) (int32, error) {
	if err := f.hit("NextFlowVersion"); err != nil {
		return 0, err
	}
	return f.Querier.NextFlowVersion(ctx, flowID)
}

func (f *failing) InsertFlowVersion(ctx context.Context, arg sqlcgen.InsertFlowVersionParams) (sqlcgen.FlowVersion, error) {
	if err := f.hit("InsertFlowVersion"); err != nil {
		return sqlcgen.FlowVersion{}, err
	}
	return f.Querier.InsertFlowVersion(ctx, arg)
}

func (f *failing) InsertJob(ctx context.Context, arg sqlcgen.InsertJobParams) (sqlcgen.Job, error) {
	if err := f.hit("InsertJob"); err != nil {
		return sqlcgen.Job{}, err
	}
	return f.Querier.InsertJob(ctx, arg)
}

func (f *failing) InsertQuestion(ctx context.Context, arg sqlcgen.InsertQuestionParams) (sqlcgen.Question, error) {
	if err := f.hit("InsertQuestion"); err != nil {
		return sqlcgen.Question{}, err
	}
	return f.Querier.InsertQuestion(ctx, arg)
}

func (f *failing) GetNodeRunForUpdate(ctx context.Context, id uuid.UUID) (sqlcgen.NodeRun, error) {
	if err := f.hit("GetNodeRunForUpdate"); err != nil {
		return sqlcgen.NodeRun{}, err
	}
	return f.Querier.GetNodeRunForUpdate(ctx, id)
}

func (f *failing) ListUndeliveredQuestions(ctx context.Context, nodeRunID uuid.UUID) ([]sqlcgen.Question, error) {
	if err := f.hit("ListUndeliveredQuestions"); err != nil {
		return nil, err
	}
	return f.Querier.ListUndeliveredQuestions(ctx, nodeRunID)
}

func (f *failing) SetFlowRunStatus(ctx context.Context, arg sqlcgen.SetFlowRunStatusParams) (sqlcgen.FlowRun, error) {
	if err := f.hit("SetFlowRunStatus"); err != nil {
		return sqlcgen.FlowRun{}, err
	}
	return f.Querier.SetFlowRunStatus(ctx, arg)
}

func (f *failing) InsertReviewComment(ctx context.Context, arg sqlcgen.InsertReviewCommentParams) (sqlcgen.ReviewComment, error) {
	if err := f.hit("InsertReviewComment"); err != nil {
		return sqlcgen.ReviewComment{}, err
	}
	return f.Querier.InsertReviewComment(ctx, arg)
}

func (f *failing) DeliverQuestions(ctx context.Context, nodeRunID uuid.UUID) error {
	if err := f.hit("DeliverQuestions"); err != nil {
		return err
	}
	return f.Querier.DeliverQuestions(ctx, nodeRunID)
}

func (f *failing) QueueNodeRun(ctx context.Context, id uuid.UUID) error {
	if err := f.hit("QueueNodeRun"); err != nil {
		return err
	}
	return f.Querier.QueueNodeRun(ctx, id)
}

func (f *failing) DeleteReviewComments(ctx context.Context, nodeRunID uuid.UUID) error {
	if err := f.hit("DeleteReviewComments"); err != nil {
		return err
	}
	return f.Querier.DeleteReviewComments(ctx, nodeRunID)
}
