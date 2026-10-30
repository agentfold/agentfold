package store

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
)

func TestCreateChildCards(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	f := newFixture(t, s)
	parent := f.card.ID
	child := func(title string, after ...int) domain.ChildCard {
		return domain.ChildCard{
			NewCard: domain.NewCard{CardInput: domain.CardInput{Title: title, CardTypeID: f.cardType.ID, ProjectID: &f.project.ID}, LaneID: f.lanes[1].ID, ParentCardID: &parent},
			After:   after,
		}
	}
	cards, err := s.CreateChildCards(ctx, []domain.ChildCard{child("API"), child("Web")})
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
	if _, err := s.CreateChildCards(ctx, []domain.ChildCard{child("A"), child("B")}); !errors.Is(err, errInjected) {
		t.Fatalf("got %v", err)
	}
	if children, _ := s.ListChildCards(ctx, parent); len(children) != 2 {
		t.Fatalf("a failed creation left cards: %+v", children)
	}
}

func TestChildCardDependencies(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	f := newFixture(t, s)
	parent := f.card.ID
	child := func(title string, deps []uuid.UUID, after ...int) domain.ChildCard {
		return domain.ChildCard{
			NewCard: domain.NewCard{CardInput: domain.CardInput{Title: title, CardTypeID: f.cardType.ID}, LaneID: f.lanes[1].ID, ParentCardID: &parent, DependsOn: deps},
			After:   after,
		}
	}
	// Web depends on API, created after it, and on the parent twice.
	cards, err := s.CreateChildCards(ctx, []domain.ChildCard{child("Web", []uuid.UUID{parent, parent}, 1), child("API", nil)})
	if err != nil {
		t.Fatal(err)
	}
	web, api := cards[0], cards[1]
	if !slices.Equal(web.DependsOn, []uuid.UUID{parent, api.ID}) || len(api.DependsOn) != 0 || api.Position != 0 {
		t.Fatalf("web %+v, api %+v", web, api)
	}
	if got, err := s.GetCard(ctx, web.ID); err != nil || !slices.Equal(got.DependsOn, web.DependsOn) {
		t.Fatalf("GetCard = %+v, %v", got, err)
	}
	dependents, err := s.ListDependentCards(ctx, api.ID)
	if err != nil || len(dependents) != 1 || dependents[0].ID != web.ID {
		t.Fatalf("ListDependentCards = %+v, %v", dependents, err)
	}
	byID, err := s.ListCardsByID(ctx, []uuid.UUID{web.ID, api.ID, web.ID, uuid.New()})
	if err != nil || len(byID) != 2 || byID[0].ID != api.ID {
		t.Fatalf("ListCardsByID = %+v, %v", byID, err)
	}

	// A card others depend on cannot be deleted.
	if err := s.DeleteCard(ctx, api.ID); !errors.Is(err, domain.ErrInUse) {
		t.Fatalf("deleting a dependency: %v", err)
	}
	if err := s.DeleteCard(ctx, web.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteCard(ctx, api.ID); err != nil {
		t.Fatal(err)
	}

	for name, batch := range map[string][]domain.ChildCard{
		"cycle":        {child("A", nil, 1), child("B", nil, 0)},
		"unknown card": {child("A", []uuid.UUID{uuid.New()})},
	} {
		if _, err := s.CreateChildCards(ctx, batch); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	if _, err := s.CreateChildCards(ctx, []domain.ChildCard{child("A", []uuid.UUID{uuid.New()})}); !errors.Is(err, domain.ErrInvalidReference) {
		t.Fatalf("unknown dependency: %v", err)
	}
	if _, err := s.CreateChildCards(cancelled(), []domain.ChildCard{child("A", []uuid.UUID{parent})}); err == nil {
		t.Fatal("cancelled context accepted")
	}
	failOn(s, "ListCardsByID", 1)
	if _, err := s.CreateChildCards(ctx, []domain.ChildCard{child("A", []uuid.UUID{parent})}); !errors.Is(err, errInjected) {
		t.Fatalf("got %v", err)
	}
	failOn(s, "ListDependentCards", 1)
	if err := s.DeleteCard(ctx, parent); !errors.Is(err, errInjected) {
		t.Fatalf("got %v", err)
	}
}

func TestSetCardProperty(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	f := newFixture(t, s)
	if len(f.card.Properties) != 0 || f.card.Properties == nil {
		t.Fatalf("new card properties %v", f.card.Properties)
	}
	if _, err := s.SetCardProperty(ctx, f.card.ID, "dev", "doing"); err != nil {
		t.Fatal(err)
	}
	card, err := s.SetCardProperty(ctx, f.card.ID, "dev", "ok")
	if err != nil || card.Properties["dev"] != "ok" || len(card.Properties) != 1 {
		t.Fatalf("SetCardProperty = %+v, %v", card, err)
	}
	if card, _ = s.SetCardProperty(ctx, f.card.ID, "qa", "ok"); !card.HasProperty("dev", "ok") || !card.HasProperty("qa", "ok") {
		t.Fatalf("properties %v", card.Properties)
	}
	events, _ := s.ListCardEvents(ctx, f.card.ID)
	if last := events[len(events)-1]; last.Type != domain.PropertySet || string(last.Payload) != `{"key": "qa", "value": "ok"}` {
		t.Fatalf("event %+v %s", last, last.Payload)
	}
	if _, err := s.SetCardProperty(ctx, uuid.New(), "dev", "ok"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown card: %v", err)
	}
	failOn(s, "InsertCardEvent", 1)
	if _, err := s.SetCardProperty(ctx, f.card.ID, "dev", "ok"); !errors.Is(err, errInjected) {
		t.Fatalf("got %v", err)
	}
}

func TestWaitingForChildren(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	run, nr := runningNode(t, s)
	if _, err := s.WaitingNodeRun(ctx, run.CardID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("nothing waits yet: %v", err)
	}
	held, err := s.WaitNodeRun(ctx, nr.ID)
	if err != nil || held.Status != domain.NodeWaiting {
		t.Fatalf("WaitNodeRun = %+v, %v", held, err)
	}
	if _, err := s.WaitingNodeRun(ctx, run.CardID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("the run is not waiting yet: %v", err)
	}
	if _, err := s.WaitFlowRun(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	if got, err := s.WaitingNodeRun(ctx, run.CardID); err != nil || got.ID != nr.ID {
		t.Fatalf("WaitingNodeRun = %+v, %v", got, err)
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
	child, err := s.CreateChildCards(ctx, []domain.ChildCard{{NewCard: domain.NewCard{CardInput: domain.CardInput{Title: "c", CardTypeID: card.CardTypeID}, LaneID: card.LaneID, ParentCardID: &card.ID}}})
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

func TestCancelAndRetryCard(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	f := newFixture(t, s)
	card, err := s.CancelCard(ctx, f.card.ID)
	if err != nil || card.CanceledAt == nil {
		t.Fatalf("CancelCard = %+v, %v", card, err)
	}
	if got, _ := s.GetCard(ctx, f.card.ID); got.CanceledAt == nil {
		t.Fatal("the cancel was not stored")
	}
	if card, err = s.RetryCard(ctx, f.card.ID); err != nil || card.CanceledAt != nil {
		t.Fatalf("RetryCard = %+v, %v", card, err)
	}
	var payload json.RawMessage
	if err := pool.QueryRow(ctx, "SELECT payload FROM jobs WHERE kind = 'flow.retry'").Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var job domain.RetryJob
	if json.Unmarshal(payload, &job) != nil || job.CardID != f.card.ID {
		t.Fatalf("job %s", payload)
	}
	if _, err := s.RetryCard(ctx, uuid.New()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown card: %v", err)
	}
	failOn(s, "InsertJob", 1)
	if _, err := s.RetryCard(ctx, f.card.ID); !errors.Is(err, errInjected) {
		t.Fatalf("got %v", err)
	}
}
