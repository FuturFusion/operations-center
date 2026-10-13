package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"

	"github.com/FuturFusion/operations-center/internal/version"
)

var componentRuntime = RegisterComponent("runtime")

// runtimeRegistry is a registry of its own, so all metrics of the runtime
// component are gated at once.
var runtimeRegistry = prometheus.NewRegistry()

var startTime = time.Now()

func init() {
	buildInfo := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: Namespace,
		Name:      "build_info",
		Help:      "Build information of Operations Center. The value is always 1.",
	}, []string{"version"})
	buildInfo.WithLabelValues(version.Version).Set(1)

	uptime := prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Namespace: Namespace,
		Name:      "uptime_seconds",
		Help:      "Time in seconds since the start of Operations Center.",
	}, func() float64 {
		return time.Since(startTime).Seconds()
	})

	runtimeRegistry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		collectors.NewBuildInfoCollector(),
		buildInfo,
		uptime,
	)

	registry.MustRegister(GatedCollector(componentRuntime, runtimeRegistry))
}

// RuntimeRegisterer returns the registerer for the metrics of the runtime
// component.
func RuntimeRegisterer() prometheus.Registerer {
	return runtimeRegistry
}
