// Package metrics holds the metrics registry of the daemon and the components
// that gate groups of metrics.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Namespace is the prefix of all metrics owned by Operations Center.
const Namespace = "operations_center"

// registry is an own registry instead of the default one, so the collectors
// of the runtime component can be gated.
var registry = prometheus.NewRegistry()

// Registerer returns the registerer for the metrics of the daemon.
func Registerer() prometheus.Registerer {
	return registry
}

// Gatherer returns the gatherer for the metrics of the daemon.
func Gatherer() prometheus.Gatherer {
	return registry
}
