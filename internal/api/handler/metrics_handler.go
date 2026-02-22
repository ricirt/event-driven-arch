package handler

import (
	"net/http"

	"github.com/notifyhub/event-driven-arch/internal/queue"
)

// MetricsHandler serves a human-readable JSON queue snapshot.
// Raw Prometheus metrics (counters, histograms) are available at /metrics
// via promhttp.Handler and are separate from this endpoint.
type MetricsHandler struct {
	q *queue.PriorityQueue
}

func NewMetricsHandler(q *queue.PriorityQueue) *MetricsHandler {
	return &MetricsHandler{q: q}
}

// GetMetrics handles GET /api/v1/metrics
//
// @Summary  Real-time queue depth snapshot
// @Tags     metrics
// @Produce  json
// @Success  200  {object}  map[string]any
// @Router   /api/v1/metrics [get]
func (h *MetricsHandler) GetMetrics(w http.ResponseWriter, r *http.Request) {
	high, normal, low := h.q.Depths()
	respondJSON(w, http.StatusOK, map[string]any{
		"queue_depth": map[string]int{
			"high":   high,
			"normal": normal,
			"low":    low,
			"total":  high + normal + low,
		},
	})
}
