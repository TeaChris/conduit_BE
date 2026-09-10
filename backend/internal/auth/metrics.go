package auth

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	// operationsTotal tracks total auth domain operations.
	operationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "conduit",
			Subsystem: "auth",
			Name:      "operations_total",
			Help:      "Total authentication domain operations.",
		},
		[]string{"operation", "status"},
	)

	// operationDuration tracks auth domain operation duration in seconds.
	operationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "conduit",
			Subsystem: "auth",
			Name:      "operation_duration_seconds",
			Help:      "Authentication domain operation duration in seconds.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"operation"},
	)
)

func init() {
	prometheus.MustRegister(operationsTotal)
	prometheus.MustRegister(operationDuration)
}
