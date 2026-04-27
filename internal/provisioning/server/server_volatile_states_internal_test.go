package server

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
)

func Test_volatileServerStates_retryCount(t *testing.T) {
	tests := []struct {
		name    string
		servers map[string]volatileServerState

		wantOK      bool
		wantRetries int
	}{
		{
			name:    "success",
			servers: map[string]volatileServerState{},

			wantRetries: 0,
		},
		{
			name: "success - 2nd try",
			servers: map[string]volatileServerState{
				"one": {
					inFlightOperation:   operationEvacuation,
					operationRetryCount: 1,
				},
			},

			wantRetries: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := volatileServerStates{
				mu:      sync.Mutex{},
				servers: tc.servers,
			}

			retries := v.retryCount("one")

			require.Equal(t, tc.wantRetries, retries)
		})
	}
}

func Test_volatileServerStates_start(t *testing.T) {
	tests := []struct {
		name    string
		servers map[string]volatileServerState

		wantOK      bool
		wantRetries int
	}{
		{
			name:    "success",
			servers: map[string]volatileServerState{},

			wantOK:      true,
			wantRetries: 1,
		},
		{
			name: "operation ongoing",
			servers: map[string]volatileServerState{
				"one": {
					inFlightOperation: operationRestore,
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := volatileServerStates{
				mu:      sync.Mutex{},
				servers: tc.servers,
			}

			ok := v.start("one", operationEvacuation)

			require.Equal(t, tc.wantOK, ok)
		})
	}
}

func Test_volatileServerStates_done(t *testing.T) {
	tests := []struct {
		name    string
		servers map[string]volatileServerState
		err     error

		wantInFlightOperation operation
		wantLastErr           error
	}{
		{
			name: "success",
			servers: map[string]volatileServerState{
				"one": {
					inFlightOperation: operationEvacuation,
				},
			},
			err: boom.Error,

			wantInFlightOperation: operationNone,
			wantLastErr:           boom.Error,
		},
		{
			name:    "empty state - wrong operation",
			servers: map[string]volatileServerState{},

			wantInFlightOperation: operationNone,
		},
		{
			name: "wrong operation",
			servers: map[string]volatileServerState{
				"one": {
					inFlightOperation: operationRestore,
				},
			},

			wantInFlightOperation: operationRestore,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := volatileServerStates{
				mu:      sync.Mutex{},
				servers: tc.servers,
			}

			v.done("one", operationEvacuation, tc.err)

			v.mu.Lock()
			require.Equal(t, tc.wantInFlightOperation, v.servers["one"].inFlightOperation)
			v.mu.Unlock()

			require.Equal(t, tc.wantLastErr, v.lastErr("one"))
		})
	}
}

func Test_volatileServerStates_reset(t *testing.T) {
	tests := []struct {
		name    string
		servers map[string]volatileServerState
		err     error
	}{
		{
			name: "success",
			servers: map[string]volatileServerState{
				"one": {
					inFlightOperation: operationEvacuation,
				},
			},
		},
		{
			name:    "empty state - wrong operation",
			servers: map[string]volatileServerState{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := volatileServerStates{
				mu:      sync.Mutex{},
				servers: tc.servers,
			}

			v.reset("one", operationEvacuation)

			v.mu.Lock()
			require.Equal(t, operationNone, v.servers["one"].inFlightOperation)
			require.Equal(t, 0, v.servers["one"].operationRetryCount)
			v.mu.Unlock()

			require.NoError(t, v.lastErr("one"))
		})
	}
}
