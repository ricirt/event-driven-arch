package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/ricirt/event-driven-arch/internal/domain"
)

func respondJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func respondError(w http.ResponseWriter, status int, msg string) {
	respondJSON(w, status, map[string]string{"error": msg})
}

// mapError translates domain sentinel errors to HTTP status codes.
// All mapping lives here so individual handlers stay concise.
func mapError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		respondError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, domain.ErrConflict),
		errors.Is(err, domain.ErrAlreadyCancelled),
		errors.Is(err, domain.ErrNotCancellable):
		respondError(w, http.StatusConflict, err.Error())
	case errors.Is(err, domain.ErrInvalidChannel),
		errors.Is(err, domain.ErrInvalidPriority),
		errors.Is(err, domain.ErrInvalidContent),
		errors.Is(err, domain.ErrInvalidRecipient),
		errors.Is(err, domain.ErrBatchTooLarge),
		errors.Is(err, domain.ErrBatchEmpty):
		respondError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, domain.ErrQueueFull):
		respondError(w, http.StatusServiceUnavailable, err.Error())
	default:
		respondError(w, http.StatusInternalServerError, "internal server error")
	}
}
