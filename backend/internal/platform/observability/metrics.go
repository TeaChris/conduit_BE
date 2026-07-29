package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net/http"
)

var (
	// HTTPRequestDuration tracks HTTP request duration in seconds.
	HTTPRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "conduit",
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request duration in seconds.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"method", "path", "status"},
	)

	// HTTPRequestsTotal tracks total HTTP requests.
	HTTPRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "conduit",
			Name:      "http_requests_total",
			Help:      "Total number of HTTP requests.",
		},
		[]string{"method", "path", "status"},
	)
)

func init() {
	prometheus.MustRegister(HTTPRequestDuration)
	prometheus.MustRegister(HTTPRequestsTotal)
}

// MetricsHandler returns an HTTP handler for Prometheus metrics.
func MetricsHandler() http.Handler {
	return promhttp.Handler()
}
