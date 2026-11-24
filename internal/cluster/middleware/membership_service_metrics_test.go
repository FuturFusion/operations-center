package middleware_test

import (
	"context"
	"testing"

	"github.com/cowsql/go-cowsql/cluster"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/cluster/middleware"
	"github.com/FuturFusion/operations-center/internal/cluster/mock"
	"github.com/FuturFusion/operations-center/internal/util/metrics"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
	"github.com/FuturFusion/operations-center/internal/util/testing/callseries"
)

// The generated metrics decorators share one template. MembershipService has
// a method of every kind: with an error return, with a value return and void.
func TestMembershipServiceWithMetrics(t *testing.T) {
	const component = "cluster.membership_service"

	tests := []struct {
		name    string
		enabled []string
		call    func(svc middleware.MembershipServiceWithMetrics)

		wantBaseCalls func(base *mock.MembershipServiceMock) int
		want          map[string]float64
	}{
		{
			name:    "error return enabled",
			enabled: []string{component},
			call: func(svc middleware.MembershipServiceWithMetrics) {
				_ = svc.DetermineNodeID(context.Background(), "address")
			},

			wantBaseCalls: func(base *mock.MembershipServiceMock) int { return len(base.DetermineNodeIDCalls()) },
			want: map[string]float64{
				"operations_center_component_calls_total{method=DetermineNodeID}":                       1,
				"operations_center_component_call_duration_seconds{method=DetermineNodeID}":             1,
				"operations_center_component_call_errors_total{method=DetermineNodeID,retryable=false}": 1,
			},
		},
		{
			name:    "error return disabled",
			enabled: nil,
			call: func(svc middleware.MembershipServiceWithMetrics) {
				_ = svc.DetermineNodeID(context.Background(), "address")
			},

			wantBaseCalls: func(base *mock.MembershipServiceMock) int { return len(base.DetermineNodeIDCalls()) },
			want:          map[string]float64{},
		},
		{
			name:    "value return enabled",
			enabled: []string{component},
			call: func(svc middleware.MembershipServiceWithMetrics) {
				_ = svc.Gateway()
			},

			wantBaseCalls: func(base *mock.MembershipServiceMock) int { return len(base.GatewayCalls()) },
			want: map[string]float64{
				"operations_center_component_calls_total{method=Gateway}":           1,
				"operations_center_component_call_duration_seconds{method=Gateway}": 1,
			},
		},
		{
			name:    "value return disabled",
			enabled: nil,
			call: func(svc middleware.MembershipServiceWithMetrics) {
				_ = svc.Gateway()
			},

			wantBaseCalls: func(base *mock.MembershipServiceMock) int { return len(base.GatewayCalls()) },
			want:          map[string]float64{},
		},
		{
			name:    "void enabled",
			enabled: []string{component},
			call: func(svc middleware.MembershipServiceWithMetrics) {
				svc.StartLeaderTasks()
			},

			wantBaseCalls: func(base *mock.MembershipServiceMock) int { return len(base.StartLeaderTasksCalls()) },
			want: map[string]float64{
				"operations_center_component_calls_total{method=StartLeaderTasks}":           1,
				"operations_center_component_call_duration_seconds{method=StartLeaderTasks}": 1,
			},
		},
		{
			name:    "void disabled",
			enabled: nil,
			call: func(svc middleware.MembershipServiceWithMetrics) {
				svc.StartLeaderTasks()
			},

			wantBaseCalls: func(base *mock.MembershipServiceMock) int { return len(base.StartLeaderTasksCalls()) },
			want:          map[string]float64{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			base := &mock.MembershipServiceMock{
				DetermineNodeIDFunc: func(ctx context.Context, address string) error {
					return boom.Error
				},
				GatewayFunc: func() cluster.Gateway {
					return nil
				},
				StartLeaderTasksFunc: func() {},
			}

			svc := middleware.NewMembershipServiceWithMetrics(base)

			metrics.SetEnabledComponents(tc.enabled)
			t.Cleanup(func() { metrics.SetEnabledComponents(nil) })

			// Run test
			tc.call(svc)

			// Assert
			require.Equal(t, 1, tc.wantBaseCalls(base), "the decorator calls the base method exactly once")
			require.Equal(t, tc.want, callseries.Gather(t, metrics.Gatherer(), component), "only an enabled component records call series")
		})
	}
}
