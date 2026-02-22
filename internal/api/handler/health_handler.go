package handler

import "net/http"

// HealthHandler serves the liveness probe endpoint.
type HealthHandler struct{}

func NewHealthHandler() *HealthHandler { return &HealthHandler{} }

// Health handles GET /health
//
// @Summary  Liveness probe
// @Tags     system
// @Produce  json
// @Success  200  {object}  map[string]string
// @Router   /health [get]
func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
