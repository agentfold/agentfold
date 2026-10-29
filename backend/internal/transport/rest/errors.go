package rest

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"orquestrator/internal/domain"
	"orquestrator/internal/transport/rest/openapi"
)

// writeJSON writes v with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v) // the client may be gone; nothing left to do
}

// badRequest answers requests the generated code could not decode.
func badRequest(w http.ResponseWriter, _ *http.Request, err error) {
	writeJSON(w, http.StatusBadRequest, openapi.Error{Code: openapi.BadRequest, Message: err.Error()})
}

// errorWriter answers failed operations with the status of their domain error.
func errorWriter(log *slog.Logger) func(http.ResponseWriter, *http.Request, error) {
	return func(w http.ResponseWriter, r *http.Request, err error) {
		var verr *domain.ValidationError
		var gerr *domain.GraphError
		switch {
		case errors.As(err, &gerr):
			problems := toProblems(gerr.Problems)
			writeJSON(w, http.StatusUnprocessableEntity, openapi.Error{
				Code: openapi.InvalidGraph, Message: "the draft has problems", Problems: &problems,
			})
		case errors.As(err, &verr):
			writeJSON(w, http.StatusUnprocessableEntity, openapi.Error{
				Code: openapi.Validation, Message: verr.Error(), Field: &verr.Field,
			})
		case errors.Is(err, domain.ErrNotFound):
			writeJSON(w, http.StatusNotFound, openapi.Error{Code: openapi.NotFound, Message: err.Error()})
		case errors.Is(err, domain.ErrConflict):
			writeJSON(w, http.StatusConflict, openapi.Error{Code: openapi.Conflict, Message: err.Error()})
		case errors.Is(err, domain.ErrCardBusy):
			writeJSON(w, http.StatusConflict, openapi.Error{Code: openapi.CardBusy, Message: err.Error()})
		case errors.Is(err, domain.ErrQuestionClosed):
			writeJSON(w, http.StatusConflict, openapi.Error{Code: openapi.QuestionClosed, Message: err.Error()})
		case errors.Is(err, domain.ErrReviewClosed):
			writeJSON(w, http.StatusConflict, openapi.Error{Code: openapi.ReviewClosed, Message: err.Error()})
		case errors.Is(err, domain.ErrInUse):
			writeJSON(w, http.StatusConflict, openapi.Error{Code: openapi.InUse, Message: err.Error()})
		case errors.Is(err, domain.ErrInvalidReference):
			writeJSON(w, http.StatusUnprocessableEntity, openapi.Error{Code: openapi.InvalidReference, Message: err.Error()})
		default:
			log.ErrorContext(r.Context(), "request failed", "method", r.Method, "path", r.URL.Path, "error", err)
			writeJSON(w, http.StatusInternalServerError, openapi.Error{Code: openapi.Internal, Message: "internal error"})
		}
	}
}
