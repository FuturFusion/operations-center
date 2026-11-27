package callseries

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
)

// Gather returns the call series of the component from gatherer. The key is
// the metric name with all labels except the component label. The value is
// the counter value or the sample count of the histogram.
func Gather(t *testing.T, gatherer prometheus.Gatherer, component string) map[string]float64 {
	t.Helper()

	families, err := gatherer.Gather()
	require.NoError(t, err)

	series := map[string]float64{}
	for _, family := range families {
		for _, metric := range family.GetMetric() {
			labels, ok := labelsOf(metric, component)
			if !ok {
				continue
			}

			series[family.GetName()+"{"+strings.Join(labels, ",")+"}"] = valueOf(family.GetType(), metric)
		}
	}

	return series
}

// labelsOf returns the labels of metric without the component label. It
// returns false if metric does not belong to component.
func labelsOf(metric *dto.Metric, component string) ([]string, bool) {
	labels := []string{}
	belongs := false
	for _, label := range metric.GetLabel() {
		if label.GetName() == "component" {
			belongs = label.GetValue() == component
			continue
		}

		labels = append(labels, label.GetName()+"="+label.GetValue())
	}

	return labels, belongs
}

func valueOf(metricType dto.MetricType, metric *dto.Metric) float64 {
	switch metricType {
	case dto.MetricType_HISTOGRAM:
		return float64(metric.GetHistogram().GetSampleCount())

	default:
		return metric.GetCounter().GetValue()
	}
}
