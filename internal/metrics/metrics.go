package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/notifyhub/event-driven-arch/internal/domain"
)

// Metrics groups all Prometheus instruments used across the application.
// Registered once at startup via New(); passed by pointer wherever needed.
type Metrics struct {
	NotificationsSent   *prometheus.CounterVec
	NotificationsFailed *prometheus.CounterVec
	NotificationLatency *prometheus.HistogramVec
	QueueDepthHigh      prometheus.Gauge
	QueueDepthNormal    prometheus.Gauge
	QueueDepthLow       prometheus.Gauge
}

// New registers all instruments with the given Prometheus registerer and
// returns the populated Metrics struct.
// Using a custom registry (instead of prometheus.DefaultRegisterer) keeps
// tests isolated and avoids global state.
func New(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		NotificationsSent: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "notifications_sent_total",
			Help: "Total number of successfully delivered notifications.",
		}, []string{"channel"}),

		NotificationsFailed: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "notifications_failed_total",
			Help: "Total number of permanently failed notifications (retries exhausted).",
		}, []string{"channel"}),

		NotificationLatency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "notification_processing_seconds",
			Help:    "End-to-end processing latency from dequeue to provider ack.",
			Buckets: prometheus.DefBuckets,
		}, []string{"channel"}),

		QueueDepthHigh: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "queue_depth_high",
			Help: "Current number of items in the high-priority queue.",
		}),
		QueueDepthNormal: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "queue_depth_normal",
			Help: "Current number of items in the normal-priority queue.",
		}),
		QueueDepthLow: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "queue_depth_low",
			Help: "Current number of items in the low-priority queue.",
		}),
	}

	reg.MustRegister(
		m.NotificationsSent,
		m.NotificationsFailed,
		m.NotificationLatency,
		m.QueueDepthHigh,
		m.QueueDepthNormal,
		m.QueueDepthLow,
	)

	return m
}

// WorkerHooks returns the metric callback functions expected by worker.MetricHooks.
// Centralises the prometheus observation calls so worker.go stays import-free.
func (m *Metrics) WorkerHooks() (
	onSent func(domain.Channel, time.Duration),
	onFailed func(domain.Channel),
) {
	onSent = func(ch domain.Channel, latency time.Duration) {
		m.NotificationsSent.WithLabelValues(string(ch)).Inc()
		m.NotificationLatency.WithLabelValues(string(ch)).Observe(latency.Seconds())
	}
	onFailed = func(ch domain.Channel) {
		m.NotificationsFailed.WithLabelValues(string(ch)).Inc()
	}
	return
}
