// Package domain holds the board entities and their validation rules. It has no I/O.
package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	// ErrNotFound means the referenced entity does not exist.
	ErrNotFound = errors.New("not found")
	// ErrInUse means the entity is still referenced and cannot be deleted.
	ErrInUse = errors.New("in use")
	// ErrConflict means the change clashes with existing data (e.g. a duplicate name).
	ErrConflict = errors.New("conflict")
	// ErrInvalidReference means the change points to an entity that does not exist.
	ErrInvalidReference = errors.New("invalid reference")
	// ErrCardBusy means a flow is running for the card, which holds it in place.
	ErrCardBusy = errors.New("a flow is running for the card")
	// ErrQuestionClosed means the answer already reached the agent.
	ErrQuestionClosed = errors.New("the answer was already delivered to the agent")
)

// ValidationError reports an invalid field value.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// SourceKind is where a project's code comes from.
type SourceKind string

// SourceLocal is a git repository on the worker host.
const SourceLocal SourceKind = "local"

// Lane is a board column.
type Lane struct {
	ID        uuid.UUID
	Name      string
	Color     string
	Position  int
	CreatedAt time.Time
}

// Project is a code base that cards can work on.
type Project struct {
	ID         uuid.UUID
	Name       string
	SourceKind SourceKind
	LocalPath  string
	CreatedAt  time.Time
}

// CardType classifies cards; flow triggers can filter by it.
type CardType struct {
	ID        uuid.UUID
	Name      string
	Color     string
	CreatedAt time.Time
}

// Card is a unit of work on the board.
type Card struct {
	ID           uuid.UUID
	Title        string
	Description  string
	CardTypeID   uuid.UUID
	ProjectID    *uuid.UUID
	ParentCardID *uuid.UUID
	LaneID       uuid.UUID
	Position     int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// CardEventType names an entry of the card timeline.
type CardEventType string

// Card timeline event types.
const (
	CardCreated CardEventType = "card.created"
	CardUpdated CardEventType = "card.updated"
	CardMoved   CardEventType = "card.moved"

	FlowStarted  CardEventType = "flow.started"
	FlowSkipped  CardEventType = "flow.skipped"
	FlowFinished CardEventType = "flow.finished"
	NodeStarted  CardEventType = "node.started"
	NodeFinished CardEventType = "node.finished"

	QuestionAsked    CardEventType = "question.asked"
	QuestionAnswered CardEventType = "question.answered"
	ReviewSubmitted  CardEventType = "review.submitted"
)

// CardEvent is an entry of the card timeline.
type CardEvent struct {
	ID        uuid.UUID
	CardID    uuid.UUID
	Type      CardEventType
	Payload   json.RawMessage
	CreatedAt time.Time
}

// LaneInput holds the editable fields of a lane.
type LaneInput struct {
	Name  string
	Color string
}

// Normalize trims the fields and validates them.
func (in LaneInput) Normalize() (LaneInput, error) {
	name, err := requireText("name", in.Name)
	if err != nil {
		return in, err
	}
	color, err := normalizeColor(in.Color)
	if err != nil {
		return in, err
	}
	return LaneInput{Name: name, Color: color}, nil
}

// CardTypeInput holds the editable fields of a card type.
type CardTypeInput struct {
	Name  string
	Color string
}

// Normalize trims the fields and validates them.
func (in CardTypeInput) Normalize() (CardTypeInput, error) {
	lane, err := LaneInput(in).Normalize()
	return CardTypeInput(lane), err
}

// ProjectInput holds the editable fields of a project.
type ProjectInput struct {
	Name      string
	LocalPath string
}

// Normalize trims the fields and validates them. Whether LocalPath is a git
// repository is checked by the service, since it needs the filesystem.
func (in ProjectInput) Normalize() (ProjectInput, error) {
	name, err := requireText("name", in.Name)
	if err != nil {
		return in, err
	}
	path, err := requireText("local_path", in.LocalPath)
	if err != nil {
		return in, err
	}
	if !strings.HasPrefix(path, "/") {
		return in, &ValidationError{Field: "local_path", Message: "must be an absolute path"}
	}
	return ProjectInput{Name: name, LocalPath: path}, nil
}

// CardInput holds the fields of a card that can be edited after creation.
type CardInput struct {
	Title       string
	Description string
	CardTypeID  uuid.UUID
	ProjectID   *uuid.UUID
}

// Normalize trims the fields and validates them.
func (in CardInput) Normalize() (CardInput, error) {
	title, err := requireText("title", in.Title)
	if err != nil {
		return in, err
	}
	if in.CardTypeID == uuid.Nil {
		return in, &ValidationError{Field: "card_type_id", Message: "is required"}
	}
	in.Title = title
	in.Description = strings.TrimSpace(in.Description)
	return in, nil
}

// NewCard holds the fields used to create a card.
type NewCard struct {
	CardInput
	LaneID       uuid.UUID
	ParentCardID *uuid.UUID
}

// Normalize trims the fields and validates them.
func (in NewCard) Normalize() (NewCard, error) {
	card, err := in.CardInput.Normalize()
	if err != nil {
		return in, err
	}
	if in.LaneID == uuid.Nil {
		return in, &ValidationError{Field: "lane_id", Message: "is required"}
	}
	in.CardInput = card
	return in, nil
}

// ValidatePosition rejects negative positions.
func ValidatePosition(position int) error {
	if position < 0 {
		return &ValidationError{Field: "position", Message: "must not be negative"}
	}
	return nil
}

var colorPattern = regexp.MustCompile(`^#[0-9a-f]{6}$`)

func normalizeColor(color string) (string, error) {
	color = strings.ToLower(strings.TrimSpace(color))
	if color != "" && !colorPattern.MatchString(color) {
		return "", &ValidationError{Field: "color", Message: "must be empty or #rrggbb"}
	}
	return color, nil
}

func requireText(field, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", &ValidationError{Field: field, Message: "is required"}
	}
	return value, nil
}

// EntityID returns the lane id.
func (l Lane) EntityID() uuid.UUID { return l.ID }

// EntityID returns the project id.
func (p Project) EntityID() uuid.UUID { return p.ID }

// EntityID returns the card type id.
func (c CardType) EntityID() uuid.UUID { return c.ID }

// EntityID returns the card id.
func (c Card) EntityID() uuid.UUID { return c.ID }

// EntityID returns the card event id.
func (e CardEvent) EntityID() uuid.UUID { return e.ID }
