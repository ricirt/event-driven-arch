package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"github.com/ricirt/event-driven-arch/internal/api/handler"
	apimw "github.com/ricirt/event-driven-arch/internal/api/middleware"
	"github.com/ricirt/event-driven-arch/internal/queue"
	"github.com/ricirt/event-driven-arch/internal/service"
)

// NewRouter wires the chi router, attaches all middleware, and registers
// every route. It is the single source of truth for the HTTP surface area.
func NewRouter(
	svc *service.NotificationService,
	q *queue.PriorityQueue,
	reg prometheus.Gatherer,
	logger *zap.Logger,
) http.Handler {
	r := chi.NewRouter()

	// --- global middleware (applied to every route) ---
	r.Use(chimw.Recoverer)         // recover panics, return 500
	r.Use(chimw.RealIP)            // trust X-Forwarded-For / X-Real-IP
	r.Use(chimw.RequestSize(1<<20)) // 1 MB max request body
	r.Use(apimw.CorrelationID)     // X-Correlation-ID inject / echo
	r.Use(apimw.RequestLogger(logger))

	// --- handler instances ---
	nh := handler.NewNotificationHandler(svc, logger)
	bh := handler.NewBatchHandler(svc, logger)
	mh := handler.NewMetricsHandler(q)
	hh := handler.NewHealthHandler()

	// --- routes ---
	r.Get("/health", hh.Health)

	// Raw Prometheus scrape endpoint (for Prometheus server / Grafana)
	r.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))

	r.Route("/api/v1", func(r chi.Router) {
		// Notifications — note: /batch must be registered before /{id}
		// so chi does not treat the literal string "batch" as an ID.
		r.Post("/notifications/batch", bh.CreateBatch)
		r.Post("/notifications", nh.Create)
		r.Get("/notifications", nh.List)
		r.Get("/notifications/{id}", nh.GetByID)
		r.Delete("/notifications/{id}", nh.Cancel)

		// Batches
		r.Get("/batches/{id}", bh.GetBatch)

		// JSON metrics snapshot
		r.Get("/metrics", mh.GetMetrics)
	})

	return r
}
