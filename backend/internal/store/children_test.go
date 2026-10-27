package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
)

func TestCreateChildCards(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	f := newFixture(t, s)
	parent := f.card.ID
	child := func(title string) domain.NewCard {
		return domain.NewCard{CardInput: domain.CardInput{Title: title, CardTypeID: f.cardType.ID, ProjectID: &f.project.ID}, LaneID: f.lanes[1].ID, ParentCardID: &parent}
	}
	cards, err := s.CreateChildCards(ctx, []domain.NewCard{child("API"), child("Web")})
	if err != nil || len(cards) != 2 || cards[1].Position != 1 || *cards[0].ParentCardID != parent {
		t.Fatalf("CreateChildCards = %+v, %v", cards, err)
	}
	children, err := s.ListChildCards(ctx, parent)
	if err != nil || len(children) != 2 || children[0].Title != "API" {
		t.Fatalf("ListChildCards = %+v, %v", children, err)
	}
	var jobs int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM jobs WHERE kind = 'flow.trigger'").Scan(&jobs); err != nil || jobs != 3 {
		t.Fatalf("trigger jobs %d, %v", jobs, err)
	}

	// The second card failing leaves none.
	failOn(s, "CountCardsInLane", 2)
	if _, err := s.CreateChildCards(ctx, []domain.NewCard{child("A"), child("B")}); !errors.Is(err, errInjected) {
		t.Fatalf("got %v", err)
	}
	if children, _ := s.ListChildCards(ctx, parent); len(children) != 2 {
		t.Fatalf("a failed creation left cards: %+v", children)
	}
}

func TestWaitingForChildren(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	run, nr := runningNode(t, s)
	if _, err := s.WaitingChildrenNodeRun(ctx, run.CardID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("nothing waits yet: %v", err)
	}
	held, err := s.WaitNodeRun(ctx, nr.ID)
	if err != nil || held.Status != domain.NodeWaiting {
		t.Fatalf("WaitNodeRun = %+v, %v", held, err)
	}
	if _, err := s.WaitingChildrenNodeRun(ctx, run.CardID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("the run is not waiting yet: %v", err)
	}
	if _, err := s.WaitFlowRun(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	if got, err := s.WaitingChildrenNodeRun(ctx, run.CardID); err != nil || got.ID != nr.ID {
		t.Fatalf("WaitingChildrenNodeRun = %+v, %v", got, err)
	}

	result := domain.NodeResult{Status: domain.NodeDone, OutputHandle: "all_done", Output: json.RawMessage(`{"children":[]}`)}
	settled, ok, err := s.SettleWait(ctx, nr.ID, result)
	if err != nil || !ok || settled.Status != domain.NodeDone || settled.OutputHandle != "all_done" {
		t.Fatalf("SettleWait = %+v, %v, %v", settled, ok, err)
	}
	if fr, _ := s.GetFlowRun(ctx, run.ID); fr.Status != domain.RunRunning {
		t.Fatalf("run %+v", fr)
	}
	if _, ok, err := s.SettleWait(ctx, nr.ID, result); ok || err != nil {
		t.Fatalf("settling twice: %v, %v", ok, err)
	}
	if _, err := s.WaitNodeRun(ctx, nr.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("holding a finished node: %v", err)
	}
	if _, _, err := s.SettleWait(ctx, uuid.New(), result); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown node run: %v", err)
	}
}

func TestSettleWaitFailures(t *testing.T) {
	for _, tc := range []struct {
		method string
		// running leaves the flow run running, so it cannot be resumed.
		running bool
		want    error
	}{
		{method: "GetNodeRunForUpdate", want: errInjected},
		{method: "SetFlowRunStatus", want: errInjected},
		{running: true, want: domain.ErrNotFound},
		{method: "FinishNodeRun", want: errInjected},
	} {
		s := newStore(t)
		ctx := context.Background()
		run, nr := runningNode(t, s)
		if _, err := s.WaitNodeRun(ctx, nr.ID); err != nil {
			t.Fatal(err)
		}
		if !tc.running {
			if _, err := s.WaitFlowRun(ctx, run.ID); err != nil {
				t.Fatal(err)
			}
		}
		if tc.method != "" {
			failOn(s, tc.method, 1)
		}
		if _, ok, err := s.SettleWait(ctx, nr.ID, domain.NodeResult{Status: domain.NodeDone}); ok || !errors.Is(err, tc.want) {
			t.Fatalf("%s: %v, %v", tc.method, ok, err)
		}
	}
}

func TestCancelingAChildRunWakesTheParent(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	run, nr := runningNode(t, s)
	if _, err := s.WaitNodeRun(ctx, nr.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CancelFlowRun(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetNodeRun(ctx, nr.ID); got.Status != domain.NodeFailed {
		t.Fatalf("a waiting node survived the cancel: %+v", got)
	}

	// The same for a child card.
	s = newStore(t)
	run, _ = runningNode(t, s)
	card, _ := s.GetCard(ctx, run.CardID)
	child, err := s.CreateChildCards(ctx, []domain.NewCard{{CardInput: domain.CardInput{Title: "c", CardTypeID: card.CardTypeID}, LaneID: card.LaneID, ParentCardID: &card.ID}})
	if err != nil {
		t.Fatal(err)
	}
	version, err := s.GetFlowVersion(ctx, run.FlowVersionID)
	if err != nil {
		t.Fatal(err)
	}
	childRun, _, err := s.StartFlowRun(ctx, version, child[0].ID, domain.EventCardCreated)
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"GetCard", "InsertJob"} {
		failOn(s, method, 1)
		if _, err := s.CancelFlowRun(ctx, childRun.ID); !errors.Is(err, errInjected) {
			t.Fatalf("%s: got %v", method, err)
		}
	}
	s.txQuerier = New(pool).txQuerier
	if _, err := s.CancelFlowRun(ctx, childRun.ID); err != nil {
		t.Fatal(err)
	}
	var payload json.RawMessage
	if err := pool.QueryRow(ctx, "SELECT payload FROM jobs WHERE kind = 'flow.child_changed'").Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var job domain.ChildChangedJob
	if json.Unmarshal(payload, &job) != nil || job.CardID != child[0].ID {
		t.Fatalf("job %s", payload)
	}
}
