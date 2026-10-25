package engine

import (
	"encoding/json"
	"slices"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
)

// lanes is what card.created and card.moved events tell about lanes.
type lanes struct {
	LaneID     uuid.UUID `json:"lane_id"`
	FromLaneID uuid.UUID `json:"from_lane_id"`
	ToLaneID   uuid.UUID `json:"to_lane_id"`
}

// entered returns the lane a card event put the card in: its lane when it
// was created, or the new lane when it moved to another one.
func entered(event domain.CardEvent) (uuid.UUID, bool) {
	var l lanes
	if json.Unmarshal(event.Payload, &l) != nil {
		return uuid.Nil, false
	}
	switch event.Type {
	case domain.CardCreated:
		return l.LaneID, true
	case domain.CardMoved:
		return l.ToLaneID, l.FromLaneID != l.ToLaneID
	}
	return uuid.Nil, false
}

// matches finds the trigger node of a graph and tells whether it fires for
// the card event. Creating a card in a lane also counts as entering it.
func matches(g domain.Graph, event domain.CardEvent, card domain.Card) (string, domain.TriggerConfig, bool) {
	var cfg domain.TriggerConfig
	i := slices.IndexFunc(g.Nodes, func(n domain.Node) bool { return n.Type == domain.NodeTrigger })
	if i < 0 || json.Unmarshal(g.Nodes[i].Config, &cfg) != nil {
		return "", cfg, false
	}
	if len(cfg.CardTypeIDs) > 0 && !slices.Contains(cfg.CardTypeIDs, card.CardTypeID) {
		return "", cfg, false
	}
	lane, ok := entered(event)
	switch {
	case !ok:
		return "", cfg, false
	case cfg.Event == domain.EventCardCreated:
		ok = event.Type == domain.CardCreated
	case cfg.Event == domain.EventCardEnteredLane:
		ok = cfg.LaneID != nil && *cfg.LaneID == lane
	default:
		ok = false
	}
	return g.Nodes[i].ID, cfg, ok
}
