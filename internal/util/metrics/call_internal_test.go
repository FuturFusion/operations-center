package metrics

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
	"github.com/FuturFusion/operations-center/internal/util/testing/callseries"
)

func TestObserveCall(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		component string
		enabled   bool
		err       error

		want map[string]float64
	}{
		{
			name:      "success",
			component: "test.observe_call.success",
			enabled:   true,
			err:       nil,

			want: map[string]float64{
				"operations_center_component_calls_total{method=Get}":           1,
				"operations_center_component_call_duration_seconds{method=Get}": 1,
			},
		},
		{
			name:      "plain error",
			component: "test.observe_call.plain_error",
			enabled:   true,
			err:       boom.Error,

			want: map[string]float64{
				"operations_center_component_calls_total{method=Get}":                       1,
				"operations_center_component_call_duration_seconds{method=Get}":             1,
				"operations_center_component_call_errors_total{method=Get,retryable=false}": 1,
			},
		},
		{
			name:      "retryable error",
			component: "test.observe_call.retryable_error",
			enabled:   true,
			err:       fmt.Errorf("outer: %w", domain.NewRetryableErr(boom.Error)),

			want: map[string]float64{
				"operations_center_component_calls_total{method=Get}":                      1,
				"operations_center_component_call_duration_seconds{method=Get}":            1,
				"operations_center_component_call_errors_total{method=Get,retryable=true}": 1,
			},
		},
		{
			name:      "disabled",
			component: "test.observe_call.disabled",
			enabled:   false,
			err:       boom.Error,

			want: map[string]float64{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			component := RegisterComponent(tc.component)
			// Set the flag directly. SetEnabledComponents would disable the
			// components of the other parallel cases.
			component.enabled.Store(tc.enabled)
			t.Cleanup(func() { deleteCallSeries(tc.component) })

			// Run test
			ObserveCall(component, "Get", time.Now(), tc.err)

			// Assert
			require.Equal(t, tc.want, callseries.Gather(t, Gatherer(), tc.component), "ObserveCall records the call, its duration and its error only for an enabled component")
		})
	}
}

func TestSetEnabledComponents_deletesCallSeries(t *testing.T) {
	tests := []struct {
		name    string
		enabled []string

		want map[string]float64
	}{
		{
			name:    "still enabled",
			enabled: []string{"test.delete_call_series"},

			want: map[string]float64{
				"operations_center_component_calls_total{method=Get}":                       1,
				"operations_center_component_call_duration_seconds{method=Get}":             1,
				"operations_center_component_call_errors_total{method=Get,retryable=false}": 1,
			},
		},
		{
			name:    "disabled",
			enabled: nil,

			want: map[string]float64{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			component := RegisterComponent("test.delete_call_series")
			t.Cleanup(func() { SetEnabledComponents(nil) })

			SetEnabledComponents([]string{"test.delete_call_series"})
			ObserveCall(component, "Get", time.Now(), boom.Error)

			// Run test
			SetEnabledComponents(tc.enabled)

			// Assert
			require.Equal(t, tc.want, callseries.Gather(t, Gatherer(), "test.delete_call_series"), "only a component that becomes disabled loses its call series")
		})
	}
}
