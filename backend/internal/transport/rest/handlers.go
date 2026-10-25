package rest

import (
	"context"

	"orquestrator/internal/domain"
	"orquestrator/internal/transport/rest/openapi"
)

// handlers implements the generated strict server over the board use cases.
type handlers struct {
	board Board
}

var _ openapi.StrictServerInterface = handlers{}

func laneInput(b *openapi.LaneInput) domain.LaneInput {
	return domain.LaneInput{Name: b.Name, Color: deref(b.Color)}
}

func cardTypeInput(b *openapi.CardTypeInput) domain.CardTypeInput {
	return domain.CardTypeInput{Name: b.Name, Color: deref(b.Color)}
}

func cardInput(b *openapi.CardInput) domain.CardInput {
	return domain.CardInput{
		Title:       b.Title,
		Description: deref(b.Description),
		CardTypeID:  b.CardTypeId,
		ProjectID:   b.ProjectId,
	}
}

// Lanes

func (h handlers) ListLanes(ctx context.Context, _ openapi.ListLanesRequestObject) (openapi.ListLanesResponseObject, error) {
	lanes, err := h.board.ListLanes(ctx)
	return openapi.ListLanes200JSONResponse(mapAll(lanes, toLane)), err
}

func (h handlers) CreateLane(ctx context.Context, req openapi.CreateLaneRequestObject) (openapi.CreateLaneResponseObject, error) {
	lane, err := h.board.CreateLane(ctx, laneInput(req.Body))
	return openapi.CreateLane201JSONResponse(toLane(lane)), err
}

func (h handlers) GetLane(ctx context.Context, req openapi.GetLaneRequestObject) (openapi.GetLaneResponseObject, error) {
	lane, err := h.board.GetLane(ctx, req.Id)
	return openapi.GetLane200JSONResponse(toLane(lane)), err
}

func (h handlers) UpdateLane(ctx context.Context, req openapi.UpdateLaneRequestObject) (openapi.UpdateLaneResponseObject, error) {
	lane, err := h.board.UpdateLane(ctx, req.Id, laneInput(req.Body))
	return openapi.UpdateLane200JSONResponse(toLane(lane)), err
}

func (h handlers) MoveLane(ctx context.Context, req openapi.MoveLaneRequestObject) (openapi.MoveLaneResponseObject, error) {
	lane, err := h.board.MoveLane(ctx, req.Id, req.Body.Position)
	return openapi.MoveLane200JSONResponse(toLane(lane)), err
}

func (h handlers) DeleteLane(ctx context.Context, req openapi.DeleteLaneRequestObject) (openapi.DeleteLaneResponseObject, error) {
	return openapi.DeleteLane204Response{}, h.board.DeleteLane(ctx, req.Id)
}

// Projects

func (h handlers) ListProjects(ctx context.Context, _ openapi.ListProjectsRequestObject) (openapi.ListProjectsResponseObject, error) {
	projects, err := h.board.ListProjects(ctx)
	return openapi.ListProjects200JSONResponse(mapAll(projects, toProject)), err
}

func (h handlers) CreateProject(ctx context.Context, req openapi.CreateProjectRequestObject) (openapi.CreateProjectResponseObject, error) {
	project, err := h.board.CreateProject(ctx, domain.ProjectInput{Name: req.Body.Name, LocalPath: req.Body.LocalPath})
	return openapi.CreateProject201JSONResponse(toProject(project)), err
}

func (h handlers) GetProject(ctx context.Context, req openapi.GetProjectRequestObject) (openapi.GetProjectResponseObject, error) {
	project, err := h.board.GetProject(ctx, req.Id)
	return openapi.GetProject200JSONResponse(toProject(project)), err
}

func (h handlers) UpdateProject(ctx context.Context, req openapi.UpdateProjectRequestObject) (openapi.UpdateProjectResponseObject, error) {
	project, err := h.board.UpdateProject(ctx, req.Id, domain.ProjectInput{Name: req.Body.Name, LocalPath: req.Body.LocalPath})
	return openapi.UpdateProject200JSONResponse(toProject(project)), err
}

func (h handlers) DeleteProject(ctx context.Context, req openapi.DeleteProjectRequestObject) (openapi.DeleteProjectResponseObject, error) {
	return openapi.DeleteProject204Response{}, h.board.DeleteProject(ctx, req.Id)
}

// Card types

func (h handlers) ListCardTypes(ctx context.Context, _ openapi.ListCardTypesRequestObject) (openapi.ListCardTypesResponseObject, error) {
	types, err := h.board.ListCardTypes(ctx)
	return openapi.ListCardTypes200JSONResponse(mapAll(types, toCardType)), err
}

func (h handlers) CreateCardType(ctx context.Context, req openapi.CreateCardTypeRequestObject) (openapi.CreateCardTypeResponseObject, error) {
	cardType, err := h.board.CreateCardType(ctx, cardTypeInput(req.Body))
	return openapi.CreateCardType201JSONResponse(toCardType(cardType)), err
}

func (h handlers) GetCardType(ctx context.Context, req openapi.GetCardTypeRequestObject) (openapi.GetCardTypeResponseObject, error) {
	cardType, err := h.board.GetCardType(ctx, req.Id)
	return openapi.GetCardType200JSONResponse(toCardType(cardType)), err
}

func (h handlers) UpdateCardType(ctx context.Context, req openapi.UpdateCardTypeRequestObject) (openapi.UpdateCardTypeResponseObject, error) {
	cardType, err := h.board.UpdateCardType(ctx, req.Id, cardTypeInput(req.Body))
	return openapi.UpdateCardType200JSONResponse(toCardType(cardType)), err
}

func (h handlers) DeleteCardType(ctx context.Context, req openapi.DeleteCardTypeRequestObject) (openapi.DeleteCardTypeResponseObject, error) {
	return openapi.DeleteCardType204Response{}, h.board.DeleteCardType(ctx, req.Id)
}

// Cards

func (h handlers) ListCards(ctx context.Context, _ openapi.ListCardsRequestObject) (openapi.ListCardsResponseObject, error) {
	cards, err := h.board.ListCards(ctx)
	return openapi.ListCards200JSONResponse(mapAll(cards, toCard)), err
}

func (h handlers) CreateCard(ctx context.Context, req openapi.CreateCardRequestObject) (openapi.CreateCardResponseObject, error) {
	b := req.Body
	card, err := h.board.CreateCard(ctx, domain.NewCard{
		CardInput: domain.CardInput{
			Title:       b.Title,
			Description: deref(b.Description),
			CardTypeID:  b.CardTypeId,
			ProjectID:   b.ProjectId,
		},
		LaneID:       b.LaneId,
		ParentCardID: b.ParentCardId,
	})
	return openapi.CreateCard201JSONResponse(toCard(card)), err
}

func (h handlers) GetCard(ctx context.Context, req openapi.GetCardRequestObject) (openapi.GetCardResponseObject, error) {
	card, err := h.board.GetCard(ctx, req.Id)
	return openapi.GetCard200JSONResponse(toCard(card)), err
}

func (h handlers) UpdateCard(ctx context.Context, req openapi.UpdateCardRequestObject) (openapi.UpdateCardResponseObject, error) {
	card, err := h.board.UpdateCard(ctx, req.Id, cardInput(req.Body))
	return openapi.UpdateCard200JSONResponse(toCard(card)), err
}

func (h handlers) MoveCard(ctx context.Context, req openapi.MoveCardRequestObject) (openapi.MoveCardResponseObject, error) {
	card, err := h.board.MoveCard(ctx, req.Id, req.Body.LaneId, req.Body.Position)
	return openapi.MoveCard200JSONResponse(toCard(card)), err
}

func (h handlers) DeleteCard(ctx context.Context, req openapi.DeleteCardRequestObject) (openapi.DeleteCardResponseObject, error) {
	return openapi.DeleteCard204Response{}, h.board.DeleteCard(ctx, req.Id)
}

func (h handlers) ListChildCards(ctx context.Context, req openapi.ListChildCardsRequestObject) (openapi.ListChildCardsResponseObject, error) {
	cards, err := h.board.ListChildCards(ctx, req.Id)
	return openapi.ListChildCards200JSONResponse(mapAll(cards, toCard)), err
}

func (h handlers) ListCardEvents(ctx context.Context, req openapi.ListCardEventsRequestObject) (openapi.ListCardEventsResponseObject, error) {
	events, err := h.board.ListCardEvents(ctx, req.Id)
	return openapi.ListCardEvents200JSONResponse(mapAll(events, toCardEvent)), err
}
