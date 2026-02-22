package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/ricirt/event-driven-arch/internal/domain"
	"github.com/ricirt/event-driven-arch/internal/service"
)

// BatchHandler handles batch-level endpoints.
type BatchHandler struct {
	svc    *service.NotificationService
	logger *zap.Logger
}

func NewBatchHandler(svc *service.NotificationService, logger *zap.Logger) *BatchHandler {
	return &BatchHandler{svc: svc, logger: logger}
}

// CreateBatch handles POST /api/v1/notifications/batch
//
// @Summary  Create up to 1000 notifications in a single request
// @Tags     batches
// @Accept   json
// @Produce  json
// @Param    body  body      domain.CreateBatchRequest  true  "Batch payload"
// @Success  201   {object}  domain.Batch
// @Failure  422   {object}  map[string]string
// @Router   /api/v1/notifications/batch [post]
func (h *BatchHandler) CreateBatch(w http.ResponseWriter, r *http.Request) {
	var req domain.CreateBatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	batch, err := h.svc.CreateBatch(r.Context(), req.Notifications)
	if err != nil {
		h.logger.Warn("create batch failed", zap.Error(err))
		mapError(w, err)
		return
	}

	respondJSON(w, http.StatusCreated, batch)
}

// GetBatch handles GET /api/v1/batches/{id}
//
// @Summary  Get a batch and its notifications
// @Tags     batches
// @Produce  json
// @Param    id   path      string  true  "Batch UUID"
// @Success  200  {object}  map[string]any
// @Failure  404  {object}  map[string]string
// @Router   /api/v1/batches/{id} [get]
func (h *BatchHandler) GetBatch(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	batch, notifications, err := h.svc.GetBatch(r.Context(), id)
	if err != nil {
		mapError(w, err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"batch":         batch,
		"notifications": notifications,
	})
}
