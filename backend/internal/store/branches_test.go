package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
)

// split runs a Split node key in branch parent and opens its branches.
func split(t *testing.T, s *Store, run domain.FlowRun, parent *domain.Branch, key string, names ...string) map[string]domain.Branch {
	t.Helper()
	ctx := context.Background()
	nr, err := s.CreateBranchNodeRun(ctx, run.ID, parent, key, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	branches, err := s.StartSplit(ctx, nr, parent, names)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]domain.Branch{}
	for _, b := range branches {
		out[b.Name] = b
	}
	return out
}

func first(bs []domain.Branch) bool {
	for _, b := range bs {
		if b.Status == domain.BranchArrived {
			return true
		}
	}
	return false
}

func TestBranches(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	run, _ := publishedRun(t, s)
	outer := split(t, s, run, nil, "s", "a", "b")
	a := outer["a"]
	if a.Label != "s.a" || a.SplitKey != "s" || a.Status != domain.BranchActive || a.ParentID != nil {
		t.Fatalf("branch %+v", a)
	}
	inner := split(t, s, run, &a, "in", "x", "y")
	x := inner["x"]
	if x.Label != "s.a/in.x" || *x.ParentID != a.ID {
		t.Fatalf("nested branch %+v", x)
	}
	open, err := s.CreateBranchNodeRun(ctx, run.ID, &x, "h", 1, nil)
	if err != nil || *open.BranchID != x.ID || open.Branch != "s.a/in.x" {
		t.Fatalf("node run %+v, %v", open, err)
	}

	// b arriving first settles the outer split: a and the branches nested
	// in it are canceled, and their open node runs fail.
	st, err := s.SettleBranch(ctx, outer["b"].ID, domain.BranchEnd{Status: domain.BranchArrived, Output: json.RawMessage(`{"v":1}`)}, first)
	if err != nil || !st.Settled || st.Parent != nil || len(st.Branches) != 2 {
		t.Fatalf("settlement %+v, %v", st, err)
	}
	if st.Branches[0].Status != domain.BranchCanceled || st.Branches[1].Status != domain.BranchArrived || string(st.Branches[1].Output) != `{"v": 1}` {
		t.Fatalf("branches %+v", st.Branches)
	}
	if got, _ := s.GetBranch(ctx, x.ID); got.Status != domain.BranchCanceled {
		t.Fatalf("nested branch %+v", got)
	}
	if got, _ := s.GetNodeRun(ctx, open.ID); got.Status != domain.NodeFailed || got.Error != "ramo cancelado" {
		t.Fatalf("open node run %+v", got)
	}
	// Ending a branch of a settled split, or one that ended, does nothing.
	if st, err := s.SettleBranch(ctx, a.ID, domain.BranchEnd{Status: domain.BranchDone}, first); err != nil || st.Settled {
		t.Fatalf("settled split %+v, %v", st, err)
	}
	if st, err := s.SettleBranch(ctx, x.ID, domain.BranchEnd{Status: domain.BranchFailed, Error: "boom"}, first); err != nil || st.Settled {
		t.Fatalf("canceled branch %+v, %v", st, err)
	}

	// A nested split that settles reports the branch it ran in.
	other := split(t, s, run, nil, "s2", "p", "q")
	p := other["p"]
	nested := split(t, s, run, &p, "in2", "r1", "r2")
	st, err = s.SettleBranch(ctx, nested["r1"].ID, domain.BranchEnd{Status: domain.BranchFailed, Error: "boom"}, first)
	if err != nil || st.Settled || len(st.Branches) != 2 || st.Branches[0].Error != "boom" {
		t.Fatalf("unsettled %+v, %v", st, err)
	}
	st, err = s.SettleBranch(ctx, nested["r2"].ID, domain.BranchEnd{Status: domain.BranchArrived}, first)
	if err != nil || !st.Settled || st.Parent == nil || st.Parent.ID != p.ID {
		t.Fatalf("nested settlement %+v, %v", st, err)
	}

	if _, err := s.GetBranch(ctx, uuid.New()); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("missing branch: %v", err)
	}
	if _, err := s.SettleBranch(ctx, uuid.New(), domain.BranchEnd{Status: domain.BranchDone}, first); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("settling a missing branch: %v", err)
	}
}

func TestBranchErrors(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		method string
		n      int
	}{
		{"InsertFlowSplit", 1}, {"InsertFlowBranch", 1},
		{"GetFlowBranch", 1}, {"GetFlowSplitForUpdate", 1}, {"EndFlowBranch", 1}, {"ListSplitBranches", 1},
		{"SettleFlowSplit", 1}, {"CancelSplitBranches", 1}, {"FailBranchNodeRuns", 1}, {"ListSplitBranches", 2},
		{"GetFlowBranch", 2},
	} {
		t.Run(fmt.Sprintf("%s %d", tc.method, tc.n), func(t *testing.T) {
			s := newStore(t)
			run, _ := publishedRun(t, s)
			outer := split(t, s, run, nil, "s", "a", "b")
			a := outer["a"]
			nested := split(t, s, run, &a, "in", "x", "y")
			failOn(s, tc.method, tc.n)
			var err error
			if tc.method == "InsertFlowSplit" || tc.method == "InsertFlowBranch" {
				nr, _ := s.CreateNodeRun(ctx, run.ID, "s3", 1, nil)
				_, err = s.StartSplit(ctx, nr, nil, []string{"a"})
			} else {
				_, err = s.SettleBranch(ctx, nested["x"].ID, domain.BranchEnd{Status: domain.BranchArrived}, first)
			}
			if !errors.Is(err, errInjected) {
				t.Fatalf("got %v", err)
			}
		})
	}
}
