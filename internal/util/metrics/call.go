package metrics

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// The services, repositories and adapters share these vectors. The component
// label tells them apart, so the metric names never collide.
var (
	componentCallsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: Namespace,
		Subsystem: "component",
		Name:      "calls_total",
		Help:      "Number of method calls of services, repositories and adapters.",
	}, []string{"component", "method"})

	componentCallErrorsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: Namespace,
		Subsystem: "component",
		Name:      "call_errors_total",
		Help:      "Number of method calls of services, repositories and adapters that returned an error.",
	}, []string{"component", "method", "retryable"})

	componentCallDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: Namespace,
		Subsystem: "component",
		Name:      "call_duration_seconds",
		Help:      "Duration in seconds of method calls of services, repositories and adapters.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"component", "method"})
)

// callSeriesMu orders recording against deleting. Without it, a call that ends
// after its component is disabled adds the deleted series again.
var callSeriesMu sync.RWMutex

func init() {
	registry.MustRegister(
		componentCallsTotal,
		componentCallErrorsTotal,
		componentCallDuration,
	)
}

// ObserveCall records the count, the errors and the duration of a method call
// of the component c. The call started at start and returned err. Pass nil for
// a method without an error return. It records nothing if c is disabled.
func ObserveCall(c *Component, method string, start time.Time, err error) {
	duration := time.Since(start)

	callSeriesMu.RLock()
	defer callSeriesMu.RUnlock()

	if !c.Enabled() {
		return
	}

	componentCallsTotal.WithLabelValues(c.name, method).Inc()
	componentCallDuration.WithLabelValues(c.name, method).Observe(duration.Seconds())

	if err != nil {
		componentCallErrorsTotal.WithLabelValues(c.name, method, RetryableLabel(err)).Inc()
	}
}

// deleteCallSeries removes the series of the component with the given name,
// so a disabled component exposes no stale series.
func deleteCallSeries(name string) {
	labels := prometheus.Labels{"component": name}

	componentCallsTotal.DeletePartialMatch(labels)
	componentCallErrorsTotal.DeletePartialMatch(labels)
	componentCallDuration.DeletePartialMatch(labels)
}
