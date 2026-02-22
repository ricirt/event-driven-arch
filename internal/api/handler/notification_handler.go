package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	apimw "github.com/ricirt/event-driven-arch/internal/api/middleware"
	"github.com/ricirt/event-driven-arch/internal/domain"
	"github.com/ricirt/event-driven-arch/internal/service"
)

// NotificationHandler handles single-notification CRUD endpoints.
type NotificationHandler struct {
	svc    *service.NotificationService
	logger *zap.Logger
}

func NewNotificationHandler(svc *service.NotificationService, logger *zap.Logger) *NotificationHandler {
	return &NotificationHandler{svc: svc, logger: logger}
}

// Create handles POST /api/v1/notifications
//
// @Summary     Create a notification
// @Tags        notifications
// @Accept      json
// @Produce     json
// @Param       X-Idempotency-Key  header    string                          false  "Idempotency key"
// @Param       body               body      domain.CreateNotificationRequest true   "Notification payload"
// @Success     201                {object}  domain.Notification
// @Success     200                {object}  domain.Notification              "Duplicate: returned existing notification"
// @Failure     422                {object}  map[string]string
// @Failure     503                {object}  map[string]string
// @Router      /api/v1/notifications [post]
func (h *NotificationHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req domain.CreateNotificationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	idempotencyKey := r.Header.Get("X-Idempotency-Key")
	n, isDuplicate, err := h.svc.Create(r.Context(), req, idempotencyKey)
	if err != nil {
		h.logger.Warn("create notification failed",
			zap.String("correlation_id", apimw.GetCorrelationID(r.Context())),
			zap.Error(err),
		)
		mapError(w, err)
		return
	}

	status := http.StatusCreated
	if isDuplicate {
		status = http.StatusOK
	}
	respondJSON(w, status, n)
}

// GetByID handles GET /api/v1/notifications/{id}
//
// @Summary  Get a notification by ID
// @Tags     notifications
// @Produce  json
// @Param    id   path      string  true  "Notification UUID"
// @Success  200  {object}  domain.Notification
// @Failure  404  {object}  map[string]string
// @Router   /api/v1/notifications/{id} [get]
func (h *NotificationHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	n, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		mapError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, n)
}

// List handles GET /api/v1/notifications
//
// @Summary  List notifications with filtering and pagination
// @Tags     notifications
// @Produce  json
// @Param    status   query     string  false  "Filter by status"
// @Param    channel  query     string  false  "Filter by channel"
// @Param    from     query     string  false  "Created after (RFC3339)"
// @Param    to       query     string  false  "Created before (RFC3339)"
// @Param    page     query     int     false  "Page number (default 1)"
// @Param    limit    query     int     false  "Items per page (default 20, max 100)"
// @Success  200      {object}  map[string]any
// @Router   /api/v1/notifications [get]
func (h *NotificationHandler) List(w http.ResponseWriter, r *http.Request) {
	filter := parseListFilter(r)
	notifications, total, err := h.svc.List(r.Context(), filter)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list notifications")
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"data":  notifications,
		"total": total,
		"page":  filter.Page,
		"limit": filter.Limit,
	})
}

// Cancel handles DELETE /api/v1/notifications/{id}
//
// @Summary  Cancel a pending notification
// @Tags     notifications
// @Param    id   path      string  true  "Notification UUID"
// @Success  204
// @Failure  404  {object}  map[string]string
// @Failure  409  {object}  map[string]string
// @Router   /api/v1/notifications/{id} [delete]
func (h *NotificationHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.svc.Cancel(r.Context(), id); err != nil {
		mapError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func parseListFilter(r *http.Request) domain.ListFilter {
	q := r.URL.Query()
	filter := domain.ListFilter{Page: 1, Limit: 20}

	if p, err := strconv.Atoi(q.Get("page")); err == nil && p > 0 {
		filter.Page = p
	}
	if l, err := strconv.Atoi(q.Get("limit")); err == nil && l > 0 && l <= 100 {
		filter.Limit = l
	}
	if s := q.Get("status"); s != "" {
		st := domain.Status(s)
		filter.Status = &st
	}
	if ch := q.Get("channel"); ch != "" {
		c := domain.Channel(ch)
		filter.Channel = &c
	}
	if f := q.Get("from"); f != "" {
		if t, err := time.Parse(time.RFC3339, f); err == nil {
			filter.From = &t
		}
	}
	if to := q.Get("to"); to != "" {
		if t, err := time.Parse(time.RFC3339, to); err == nil {
			filter.To = &t
		}
	}
	return filter
}
