// Package metrics exposes Prometheus counters and histograms for the hub.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics bundles the application's Prometheus collectors. Each instance owns
// its own registry so multiple servers (e.g. in tests) can coexist.
type Metrics struct {
	Registry *prometheus.Registry

	HTTPRequestsTotal *prometheus.CounterVec
	HTTPDuration      *prometheus.HistogramVec
	FlowRunsTotal     *prometheus.CounterVec
	TaskTotal         *prometheus.CounterVec
}

// New registers and returns the application metrics on a fresh registry.
func New() *Metrics {
	reg := prometheus.NewRegistry()
	factory := promauto.With(reg)

	return &Metrics{
		Registry: reg,
		HTTPRequestsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total HTTP requests processed, partitioned by method, path and status.",
		}, []string{"method", "path", "status"}),
		HTTPDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "path"}),
		FlowRunsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "flow_runs_total",
			Help: "Total flow executions, partitioned by status.",
		}, []string{"status"}),
		TaskTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "task_total",
			Help: "Total tasks created, partitioned by status.",
		}, []string{"status"}),
	}
}
