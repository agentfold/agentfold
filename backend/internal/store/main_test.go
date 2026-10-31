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
	_, err := pool.Exec(context.Background(), "TRUNCATE jobs, flow_branches, flow_splits, node_run_output, artifact_messages, artifact_versions, artifacts, questions, agent_sessions, node_runs, flow_runs, mcp_servers, agent_profiles, flow_versions, flows, card_events, cards, card_types, projects, lanes")
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

func (f *failing) SetNodeRunDecision(ctx context.Context, arg sqlcgen.SetNodeRunDecisionParams) (sqlcgen.NodeRun, error) {
	if err := f.hit("SetNodeRunDecision"); err != nil {
		return sqlcgen.NodeRun{}, err
	}
	return f.Querier.SetNodeRunDecision(ctx, arg)
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

func (f *failing) FailOpenNodeRuns(ctx context.Context, flowRunID uuid.UUID) error {
	if err := f.hit("FailOpenNodeRuns"); err != nil {
		return err
	}
	return f.Querier.FailOpenNodeRuns(ctx, flowRunID)
}

func (f *failing) CloseRunQuestions(ctx context.Context, flowRunID uuid.UUID) error {
	if err := f.hit("CloseRunQuestions"); err != nil {
		return err
	}
	return f.Querier.CloseRunQuestions(ctx, flowRunID)
}

func (f *failing) CancelRunJobs(ctx context.Context, flowRunID uuid.UUID) error {
	if err := f.hit("CancelRunJobs"); err != nil {
		return err
	}
	return f.Querier.CancelRunJobs(ctx, flowRunID)
}

func (f *failing) UpsertArtifact(ctx context.Context, arg sqlcgen.UpsertArtifactParams) (sqlcgen.Artifact, error) {
	if err := f.hit("UpsertArtifact"); err != nil {
		return sqlcgen.Artifact{}, err
	}
	return f.Querier.UpsertArtifact(ctx, arg)
}

func (f *failing) LatestArtifactVersion(ctx context.Context, artifactID uuid.UUID) (sqlcgen.ArtifactVersion, error) {
	if err := f.hit("LatestArtifactVersion"); err != nil {
		return sqlcgen.ArtifactVersion{}, err
	}
	return f.Querier.LatestArtifactVersion(ctx, artifactID)
}

func (f *failing) InsertArtifactVersion(ctx context.Context, arg sqlcgen.InsertArtifactVersionParams) (sqlcgen.ArtifactVersion, error) {
	if err := f.hit("InsertArtifactVersion"); err != nil {
		return sqlcgen.ArtifactVersion{}, err
	}
	return f.Querier.InsertArtifactVersion(ctx, arg)
}

func (f *failing) GetArtifactForUpdate(ctx context.Context, id uuid.UUID) (sqlcgen.Artifact, error) {
	if err := f.hit("GetArtifactForUpdate"); err != nil {
		return sqlcgen.Artifact{}, err
	}
	return f.Querier.GetArtifactForUpdate(ctx, id)
}

func (f *failing) GetNodeRun(ctx context.Context, id uuid.UUID) (sqlcgen.NodeRun, error) {
	if err := f.hit("GetNodeRun"); err != nil {
		return sqlcgen.NodeRun{}, err
	}
	return f.Querier.GetNodeRun(ctx, id)
}

func (f *failing) GetFlowRun(ctx context.Context, id uuid.UUID) (sqlcgen.GetFlowRunRow, error) {
	if err := f.hit("GetFlowRun"); err != nil {
		return sqlcgen.GetFlowRunRow{}, err
	}
	return f.Querier.GetFlowRun(ctx, id)
}

func (f *failing) CardHasActiveRun(ctx context.Context, cardID uuid.UUID) (bool, error) {
	if err := f.hit("CardHasActiveRun"); err != nil {
		return false, err
	}
	return f.Querier.CardHasActiveRun(ctx, cardID)
}

func (f *failing) InsertFlowRun(ctx context.Context, arg sqlcgen.InsertFlowRunParams) (sqlcgen.FlowRun, error) {
	if err := f.hit("InsertFlowRun"); err != nil {
		return sqlcgen.FlowRun{}, err
	}
	return f.Querier.InsertFlowRun(ctx, arg)
}

func (f *failing) InsertNodeRun(ctx context.Context, arg sqlcgen.InsertNodeRunParams) (sqlcgen.NodeRun, error) {
	if err := f.hit("InsertNodeRun"); err != nil {
		return sqlcgen.NodeRun{}, err
	}
	return f.Querier.InsertNodeRun(ctx, arg)
}

func (f *failing) GetAgentSession(ctx context.Context, arg sqlcgen.GetAgentSessionParams) (sqlcgen.AgentSession, error) {
	if err := f.hit("GetAgentSession"); err != nil {
		return sqlcgen.AgentSession{}, err
	}
	return f.Querier.GetAgentSession(ctx, arg)
}

func (f *failing) InsertAgentSession(ctx context.Context, arg sqlcgen.InsertAgentSessionParams) (sqlcgen.AgentSession, error) {
	if err := f.hit("InsertAgentSession"); err != nil {
		return sqlcgen.AgentSession{}, err
	}
	return f.Querier.InsertAgentSession(ctx, arg)
}

func (f *failing) InsertArtifactMessage(ctx context.Context, arg sqlcgen.InsertArtifactMessageParams) (sqlcgen.ArtifactMessage, error) {
	if err := f.hit("InsertArtifactMessage"); err != nil {
		return sqlcgen.ArtifactMessage{}, err
	}
	return f.Querier.InsertArtifactMessage(ctx, arg)
}

func (f *failing) OpenArtifactReview(ctx context.Context, arg sqlcgen.OpenArtifactReviewParams) ([]sqlcgen.Artifact, error) {
	if err := f.hit("OpenArtifactReview"); err != nil {
		return nil, err
	}
	return f.Querier.OpenArtifactReview(ctx, arg)
}

func (f *failing) AwaitReviewNodeRun(ctx context.Context, id uuid.UUID) (sqlcgen.NodeRun, error) {
	if err := f.hit("AwaitReviewNodeRun"); err != nil {
		return sqlcgen.NodeRun{}, err
	}
	return f.Querier.AwaitReviewNodeRun(ctx, id)
}

func (f *failing) GetNodeRunByIteration(ctx context.Context, arg sqlcgen.GetNodeRunByIterationParams) (sqlcgen.NodeRun, error) {
	if err := f.hit("GetNodeRunByIteration"); err != nil {
		return sqlcgen.NodeRun{}, err
	}
	return f.Querier.GetNodeRunByIteration(ctx, arg)
}

func (f *failing) ReviewRequest(ctx context.Context, nodeRunID *uuid.UUID) (sqlcgen.ArtifactMessage, error) {
	if err := f.hit("ReviewRequest"); err != nil {
		return sqlcgen.ArtifactMessage{}, err
	}
	return f.Querier.ReviewRequest(ctx, nodeRunID)
}

func (f *failing) FinishNodeRun(ctx context.Context, arg sqlcgen.FinishNodeRunParams) (sqlcgen.NodeRun, error) {
	if err := f.hit("FinishNodeRun"); err != nil {
		return sqlcgen.NodeRun{}, err
	}
	return f.Querier.FinishNodeRun(ctx, arg)
}

func (f *failing) GetCard(ctx context.Context, id uuid.UUID) (sqlcgen.Card, error) {
	if err := f.hit("GetCard"); err != nil {
		return sqlcgen.Card{}, err
	}
	return f.Querier.GetCard(ctx, id)
}

func (f *failing) ListCardsByID(ctx context.Context, ids []uuid.UUID) ([]sqlcgen.Card, error) {
	if err := f.hit("ListCardsByID"); err != nil {
		return nil, err
	}
	return f.Querier.ListCardsByID(ctx, ids)
}

func (f *failing) ListDependentCards(ctx context.Context, id uuid.UUID) ([]sqlcgen.Card, error) {
	if err := f.hit("ListDependentCards"); err != nil {
		return nil, err
	}
	return f.Querier.ListDependentCards(ctx, id)
}

func (f *failing) InsertFlowSplit(ctx context.Context, arg sqlcgen.InsertFlowSplitParams) error {
	if err := f.hit("InsertFlowSplit"); err != nil {
		return err
	}
	return f.Querier.InsertFlowSplit(ctx, arg)
}

func (f *failing) InsertFlowBranch(ctx context.Context, arg sqlcgen.InsertFlowBranchParams) (sqlcgen.FlowBranch, error) {
	if err := f.hit("InsertFlowBranch"); err != nil {
		return sqlcgen.FlowBranch{}, err
	}
	return f.Querier.InsertFlowBranch(ctx, arg)
}

func (f *failing) GetFlowBranch(ctx context.Context, id uuid.UUID) (sqlcgen.FlowBranch, error) {
	if err := f.hit("GetFlowBranch"); err != nil {
		return sqlcgen.FlowBranch{}, err
	}
	return f.Querier.GetFlowBranch(ctx, id)
}

func (f *failing) GetFlowSplitForUpdate(ctx context.Context, id uuid.UUID) (sqlcgen.FlowSplit, error) {
	if err := f.hit("GetFlowSplitForUpdate"); err != nil {
		return sqlcgen.FlowSplit{}, err
	}
	return f.Querier.GetFlowSplitForUpdate(ctx, id)
}

func (f *failing) EndFlowBranch(ctx context.Context, arg sqlcgen.EndFlowBranchParams) (int64, error) {
	if err := f.hit("EndFlowBranch"); err != nil {
		return 0, err
	}
	return f.Querier.EndFlowBranch(ctx, arg)
}

func (f *failing) ListSplitBranches(ctx context.Context, id uuid.UUID) ([]sqlcgen.FlowBranch, error) {
	if err := f.hit("ListSplitBranches"); err != nil {
		return nil, err
	}
	return f.Querier.ListSplitBranches(ctx, id)
}

func (f *failing) SettleFlowSplit(ctx context.Context, id uuid.UUID) error {
	if err := f.hit("SettleFlowSplit"); err != nil {
		return err
	}
	return f.Querier.SettleFlowSplit(ctx, id)
}

func (f *failing) CancelSplitBranches(ctx context.Context, id uuid.UUID) ([]uuid.UUID, error) {
	if err := f.hit("CancelSplitBranches"); err != nil {
		return nil, err
	}
	return f.Querier.CancelSplitBranches(ctx, id)
}

func (f *failing) FailBranchNodeRuns(ctx context.Context, ids []uuid.UUID) error {
	if err := f.hit("FailBranchNodeRuns"); err != nil {
		return err
	}
	return f.Querier.FailBranchNodeRuns(ctx, ids)
}
