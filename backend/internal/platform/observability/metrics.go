package observability

import (
	"slices"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics owns a private Prometheus registry and bounded-label instruments.
type Metrics struct {
	registry *prometheus.Registry
	requests *prometheus.CounterVec
}

// NewMetrics creates instruments without touching prometheus.DefaultRegisterer.
func NewMetrics() *Metrics {
	registry := prometheus.NewRegistry()
	requests := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "scyg_http_requests_total", Help: "Completed HTTP requests."}, []string{"method", "route", "status_class"})
	registry.MustRegister(requests)
	return &Metrics{registry: registry, requests: requests}
}

// Registry returns this lifecycle's private gatherer/registerer.
func (metrics *Metrics) Registry() *prometheus.Registry { return metrics.registry }

// ObserveRequest increments a request counter after mapping labels to bounded sets.
func (metrics *Metrics) ObserveRequest(method, route, statusClass string) {
	metrics.requests.WithLabelValues(bounded(method, []string{"GET", "POST", "PATCH", "DELETE"}), bounded(route, []string{"articles", "article-types", "tags", "health", "docs"}), bounded(statusClass, []string{"2xx", "3xx", "4xx", "5xx"})).Inc()
}

func bounded(value string, allowed []string) string {
	if slices.Contains(allowed, value) {
		return value
	}
	return "other"
}
