package domain

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

func fieldOf(t *testing.T, err error) string {
	t.Helper()
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("got %v, want a ValidationError", err)
	}
	return verr.Field
}

func TestValidationErrorMessage(t *testing.T) {
	err := &ValidationError{Field: "name", Message: "is required"}
	if err.Error() != "name: is required" {
		t.Fatalf("got %q", err.Error())
	}
}

func TestLaneInput(t *testing.T) {
	got, err := LaneInput{Name: "  Todo ", Color: " #AABBCC "}.Normalize()
	if err != nil || got != (LaneInput{Name: "Todo", Color: "#aabbcc"}) {
		t.Fatalf("got %+v, %v", got, err)
	}
	if _, err := (LaneInput{Name: " "}).Normalize(); fieldOf(t, err) != "name" {
		t.Fatal("blank name accepted")
	}
	if _, err := (LaneInput{Name: "a", Color: "blue"}).Normalize(); fieldOf(t, err) != "color" {
		t.Fatal("bad color accepted")
	}
}

func TestCardTypeInput(t *testing.T) {
	got, err := CardTypeInput{Name: " bug ", Color: ""}.Normalize()
	if err != nil || got != (CardTypeInput{Name: "bug"}) {
		t.Fatalf("got %+v, %v", got, err)
	}
	if _, err := (CardTypeInput{}).Normalize(); fieldOf(t, err) != "name" {
		t.Fatal("blank name accepted")
	}
}

func TestProjectInput(t *testing.T) {
	got, err := ProjectInput{Name: " web ", LocalPath: " /src/web "}.Normalize()
	if err != nil || got != (ProjectInput{Name: "web", LocalPath: "/src/web"}) {
		t.Fatalf("got %+v, %v", got, err)
	}
	tests := map[string]ProjectInput{
		"name":       {LocalPath: "/x"},
		"local_path": {Name: "x"},
	}
	for field, in := range tests {
		if _, err := in.Normalize(); fieldOf(t, err) != field {
			t.Errorf("%s not validated", field)
		}
	}
	if _, err := (ProjectInput{Name: "x", LocalPath: "src"}).Normalize(); fieldOf(t, err) != "local_path" {
		t.Fatal("relative path accepted")
	}
}

func TestCardInputs(t *testing.T) {
	typeID, laneID := uuid.New(), uuid.New()
	got, err := NewCard{
		CardInput: CardInput{Title: " t ", Description: " d ", CardTypeID: typeID},
		LaneID:    laneID,
	}.Normalize()
	if err != nil || got.Title != "t" || got.Description != "d" || got.LaneID != laneID {
		t.Fatalf("got %+v, %v", got, err)
	}

	tests := map[string]NewCard{
		"title":        {CardInput: CardInput{CardTypeID: typeID}, LaneID: laneID},
		"card_type_id": {CardInput: CardInput{Title: "t"}, LaneID: laneID},
		"lane_id":      {CardInput: CardInput{Title: "t", CardTypeID: typeID}},
	}
	for field, in := range tests {
		if _, err := in.Normalize(); fieldOf(t, err) != field {
			t.Errorf("%s not validated", field)
		}
	}
}

func TestValidatePosition(t *testing.T) {
	if err := ValidatePosition(0); err != nil {
		t.Fatal(err)
	}
	if fieldOf(t, ValidatePosition(-1)) != "position" {
		t.Fatal("negative position accepted")
	}
}

func TestEntityID(t *testing.T) {
	id := uuid.New()
	ids := []uuid.UUID{
		Lane{ID: id}.EntityID(),
		Project{ID: id}.EntityID(),
		CardType{ID: id}.EntityID(),
		Card{ID: id}.EntityID(),
		CardEvent{ID: id}.EntityID(),
		FlowRun{ID: id}.EntityID(),
	}
	for i, got := range ids {
		if got != id {
			t.Errorf("entity %d: got %s", i, got)
		}
	}
}

func TestRunStatusActive(t *testing.T) {
	for status, want := range map[RunStatus]bool{
		RunRunning: true, RunWaiting: true, RunDone: false, RunFailed: false, RunCanceled: false,
	} {
		if status.Active() != want {
			t.Errorf("%s: Active() = %v", status, !want)
		}
	}
}
