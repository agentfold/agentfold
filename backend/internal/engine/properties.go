package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
	"orquestrator/internal/events"
)

// setProperty sets the property of the node on the card. The cards waiting
// for it (its parent, the cards that depend on it) may settle.
func (e *Engine) setProperty(ctx context.Context, r *run, node domain.Node) (domain.NodeResult, error) {
	var cfg domain.SetPropertyConfig
	if err := json.Unmarshal(node.Config, &cfg); err != nil {
		return failed("configuração inválida: " + err.Error()), nil
	}
	card, err := e.Store.SetCardProperty(ctx, r.CardID, strings.TrimSpace(cfg.Key), strings.TrimSpace(cfg.Value))
	if err != nil {
		return domain.NodeResult{}, err
	}
	e.Pub.Publish(events.Event{Type: "card.updated", ID: card.ID, Data: card})
	if err := e.wakeWaiting(ctx, card); err != nil {
		return domain.NodeResult{}, err
	}
	return domain.NodeResult{Status: domain.NodeDone, OutputHandle: "next"}, nil
}

// wakeWaiting settles what may wait for card: the WaitChildren node of its
// parent and the CheckProperty nodes of the cards that depend on it.
func (e *Engine) wakeWaiting(ctx context.Context, card domain.Card) error {
	dependents, err := e.Store.ListDependentCards(ctx, card.ID)
	if err != nil {
		return err
	}
	errs := []error{e.wakeParent(ctx, card)}
	for _, d := range dependents {
		errs = append(errs, e.settle(ctx, d.ID))
	}
	return errors.Join(errs...)
}

// dependency is a card a card depends on, as CheckProperty reports it.
type dependency struct {
	ID        uuid.UUID `json:"id"`
	Title     string    `json:"title"`
	Satisfied bool      `json:"satisfied"`
}

// propertyResult settles a CheckProperty node from the cards the card
// depends on: satisfied when every one has the property, else pending, or
// nil (still waiting) with wait on.
func (e *Engine) propertyResult(ctx context.Context, cardID uuid.UUID, node domain.Node) (*domain.NodeResult, error) {
	var cfg domain.CheckPropertyConfig
	if err := json.Unmarshal(node.Config, &cfg); err != nil {
		r := failed("configuração inválida: " + err.Error())
		return &r, nil
	}
	card, err := e.Store.GetCard(ctx, cardID)
	if err != nil {
		return nil, err
	}
	deps, err := e.Store.ListCardsByID(ctx, card.DependsOn)
	if err != nil {
		return nil, err
	}
	key, value := strings.TrimSpace(cfg.Key), strings.TrimSpace(cfg.Value)
	report := make([]dependency, len(deps))
	var pending []string
	for i, d := range deps {
		report[i] = dependency{ID: d.ID, Title: d.Title, Satisfied: d.HasProperty(key, value)}
		if !report[i].Satisfied {
			pending = append(pending, fmt.Sprintf("%q (%s)", d.Title, d.ID))
		}
	}
	result := domain.NodeResult{Status: domain.NodeDone, OutputHandle: "satisfied"}
	msg := fmt.Sprintf("Todas as dependências têm %s=%s.", key, value)
	if len(pending) > 0 {
		if cfg.Wait {
			return nil, nil
		}
		result.OutputHandle = "pending"
		msg = fmt.Sprintf("Dependências sem %s=%s: %s.", key, value, strings.Join(pending, ", "))
	}
	result.Output, _ = json.Marshal(map[string]any{"result": msg, "dependencies": report}) // plain values
	return &result, nil
}
