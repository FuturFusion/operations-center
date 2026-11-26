// Package taskmetrics reads the metrics of background tasks in tests.
package taskmetrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

// Counts holds the counters of one background task.
type Counts struct {
	ErrorsHard      int
	ErrorsRetryable int
	DurationCount   uint64
}

// Sub returns the change from before to c. Tests use it because the counters
// are global and keep their values between test runs.
func (c Counts) Sub(before Counts) Counts {
	return Counts{
		ErrorsHard:      c.ErrorsHard - before.ErrorsHard,
		ErrorsRetryable: c.ErrorsRetryable - before.ErrorsRetryable,
		DurationCount:   c.DurationCount - before.DurationCount,
	}
}

// Series is the state of the series of one background task after a scrape.
// Exposed is false if the task exposes no series. LastSuccess and Interval
// are nil if the task does not expose the gauge.
type Series struct {
	Counts

	Exposed     bool
	LastSuccess *float64
	Interval    *float64
}

// Gather scrapes g and returns the series of the task with the given name.
func Gather(t *testing.T, g prometheus.Gatherer, name string) Series {
	t.Helper()

	families, err := g.Gather()
	require.NoError(t, err)

	var series Series

	for _, family := range families {
		for _, m := range family.GetMetric() {
			labels := map[string]string{}
			for _, l := range m.GetLabel() {
				labels[l.GetName()] = l.GetValue()
			}

			if labels["task"] != name {
				continue
			}

			series.Exposed = true

			switch family.GetName() {
			case "operations_center_task_errors_total":
				if labels["retryable"] == "true" {
					series.ErrorsRetryable = int(m.GetCounter().GetValue())
				} else {
					series.ErrorsHard = int(m.GetCounter().GetValue())
				}

			case "operations_center_task_duration_seconds":
				series.DurationCount = m.GetHistogram().GetSampleCount()

			case "operations_center_task_last_success_timestamp_seconds":
				series.LastSuccess = new(m.GetGauge().GetValue())

			case "operations_center_task_interval_seconds":
				series.Interval = new(m.GetGauge().GetValue())
			}
		}
	}

	return series
}
