package user

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	// operationsTotal tracks total user domain operations.
	operationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "conduit",
			Subsystem: "users",
			Name:      "operations_total",
			Help:      "Total user domain operations.",
		},
		[]string{"operation", "status"},
	)

	// operationDuration tracks user domain operation duration in seconds.
	operationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "conduit",
			Subsystem: "users",
			Name:      "operation_duration_seconds",
			Help:      "User domain operation duration in seconds.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"operation"},
	)
)

func init() {
	prometheus.MustRegister(operationsTotal)
	prometheus.MustRegister(operationDuration)
}
