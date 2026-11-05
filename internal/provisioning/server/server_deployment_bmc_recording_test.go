package server_test

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	adapterMock "github.com/FuturFusion/operations-center/internal/provisioning/adapter/mock"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
	"github.com/FuturFusion/operations-center/internal/util/testing/errassert"
	"github.com/FuturFusion/operations-center/internal/util/testing/queue"
	"github.com/FuturFusion/operations-center/shared/api"
)

type fakeBMCRecordingRepo struct {
	provisioning.BMCRecordingRepo

	t        *testing.T
	mu       sync.Mutex
	openErrs queue.Errs
	opens    int
	sessions []*fakeBMCRecordingSession
}

func (r *fakeBMCRecordingRepo) Open(ctx context.Context, server provisioning.Server, deployment provisioning.ServerDeployment) (provisioning.BMCRecordingSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.opens++

	err := r.openErrs.PopOrNil(r.t)
	if err != nil {
		return nil, err
	}

	session := &fakeBMCRecordingSession{}
	r.sessions = append(r.sessions, session)

	return session, nil
}

func (r *fakeBMCRecordingRepo) openCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.opens
}

// finalized returns the final state of every finalize call of every session.
func (r *fakeBMCRecordingRepo) finalized() []api.ServerDeploymentState {
	r.mu.Lock()
	defer r.mu.Unlock()

	var states []api.ServerDeploymentState
	for _, session := range r.sessions {
		session.mu.Lock()
		for _, deployment := range session.finalized {
			states = append(states, deployment.State)
		}

		session.mu.Unlock()
	}

	return states
}

func (r *fakeBMCRecordingRepo) exchanges() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()

	var exchanges int64
	for _, session := range r.sessions {
		exchanges += session.Stats().Exchanges
	}

	return exchanges
}

type fakeBMCRecordingSession struct {
	mu        sync.Mutex
	exchanges []api.BMCRecordingExchange
	finalized []provisioning.ServerDeployment
}

func (s *fakeBMCRecordingSession) Record(ctx context.Context, exchange api.BMCRecordingExchange) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.exchanges = append(s.exchanges, exchange)
}

func (s *fakeBMCRecordingSession) Stats() provisioning.BMCRecordingStats {
	s.mu.Lock()
	defer s.mu.Unlock()

	return provisioning.BMCRecordingStats{Exchanges: int64(len(s.exchanges))}
}

func (s *fakeBMCRecordingSession) Finalize(ctx context.Context, deployment provisioning.ServerDeployment) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.finalized = append(s.finalized, deployment)

	return nil
}

// bmcRecordingCalls collects the BMC calls by whether they carried a
// recording scope.
type bmcRecordingCalls struct {
	mu       sync.Mutex
	scoped   map[api.ServerDeploymentState]int
	unscoped []string
}

func (c *bmcRecordingCalls) scopedStates() map[api.ServerDeploymentState]int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.scoped
}

func (c *bmcRecordingCalls) unscopedCalls() []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.unscoped
}

// emulateBMCRecording wraps every function of the fake BMC client. A call with
// a recording scope records one exchange, the way the Redfish adapter does.
func emulateBMCRecording(t *testing.T, calls *bmcRecordingCalls) func(client *adapterMock.BMCServerClientPortMock) {
	t.Helper()

	return func(client *adapterMock.BMCServerClientPortMock) {
		value := reflect.ValueOf(client).Elem()
		for i := range value.NumField() {
			field := value.Field(i)
			name, ok := strings.CutSuffix(value.Type().Field(i).Name, "Func")
			if !ok || field.Kind() != reflect.Func || field.IsNil() {
				continue
			}

			original := reflect.ValueOf(field.Interface())
			field.Set(reflect.MakeFunc(field.Type(), func(args []reflect.Value) []reflect.Value {
				ctx, ok := args[0].Interface().(context.Context)
				require.True(t, ok, "the first argument of %s is the context", name)

				scope, scoped := provisioning.BMCRecordingScopeFromContext(ctx)

				calls.mu.Lock()
				if scoped {
					calls.scoped[scope.State]++
				} else {
					calls.unscoped = append(calls.unscoped, name)
				}

				calls.mu.Unlock()

				if scoped {
					scope.Recorder.Record(ctx, api.BMCRecordingExchange{
						DeploymentState: scope.State,
						StepRetries:     scope.Retries,
						Method:          name,
					})
				}

				return original.Call(args)
			}))
		}
	}
}

func TestServerService_DeploymentControlLoopRecordsBMC(t *testing.T) {
	tests := []struct {
		name              string
		worldOptions      []func(*bmcWorld)
		openErrs          queue.Errs
		cancelAt          api.ServerDeploymentState
		cancelSkipCleanup bool

		wantState        api.ServerDeploymentState
		wantOpens        int
		wantFinalized    []api.ServerDeploymentState
		wantScopedStates []api.ServerDeploymentState
		wantUnscoped     []string
	}{
		{
			name: "success - the full deployment is recorded",

			wantState:     api.ServerDeploymentStateCompleted,
			wantOpens:     1,
			wantFinalized: []api.ServerDeploymentState{api.ServerDeploymentStateCompleted},
			wantScopedStates: []api.ServerDeploymentState{
				api.ServerDeploymentStateRefreshBMCData,
				api.ServerDeploymentStateApplyBIOS,
				api.ServerDeploymentStateAttachMedia,
				api.ServerDeploymentStateWaitInstall,
			},
			wantUnscoped: []string{"GetData"},
		},
		{
			name:     "success - opening the recording is retried on the next step",
			openErrs: queue.Errs{boom.Error},

			wantState:     api.ServerDeploymentStateCompleted,
			wantOpens:     2,
			wantFinalized: []api.ServerDeploymentState{api.ServerDeploymentStateCompleted},
			wantScopedStates: []api.ServerDeploymentState{
				api.ServerDeploymentStateApplyBIOS,
				api.ServerDeploymentStateAttachMedia,
			},
			wantUnscoped: []string{"GetData", "GetData"},
		},
		{
			name: "success - a failed deployment is finalized",
			worldOptions: []func(*bmcWorld){
				func(world *bmcWorld) {
					world.powerOffErrs = queue.Errs{
						domain.NewRetryableErr(boom.Error),
						domain.NewRetryableErr(boom.Error),
						domain.NewRetryableErr(boom.Error),
						domain.NewRetryableErr(boom.Error),
					}
				},
			},

			wantState:        api.ServerDeploymentStateFailed,
			wantOpens:        1,
			wantFinalized:    []api.ServerDeploymentState{api.ServerDeploymentStateFailed},
			wantScopedStates: []api.ServerDeploymentState{api.ServerDeploymentStatePowerOffBIOS},
			wantUnscoped:     []string{"GetData"},
		},
		{
			name:     "success - a cancelled deployment is finalized",
			cancelAt: api.ServerDeploymentStateWaitPowerOffBIOS,

			wantState:        api.ServerDeploymentStateCancelled,
			wantOpens:        1,
			wantFinalized:    []api.ServerDeploymentState{api.ServerDeploymentStateCancelled},
			wantScopedStates: []api.ServerDeploymentState{api.ServerDeploymentStateCancel},
			wantUnscoped:     []string{"GetData"},
		},
		{
			name:              "success - a cancelled deployment without clean up is finalized",
			cancelAt:          api.ServerDeploymentStateWaitPowerOffBIOS,
			cancelSkipCleanup: true,

			wantState:        api.ServerDeploymentStateCancelled,
			wantOpens:        1,
			wantFinalized:    []api.ServerDeploymentState{api.ServerDeploymentStateCancelled},
			wantScopedStates: []api.ServerDeploymentState{api.ServerDeploymentStatePowerOffBIOS},
			wantUnscoped:     []string{"GetData"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()

			// Setup
			calls := &bmcRecordingCalls{scoped: map[api.ServerDeploymentState]int{}}
			recordings := &fakeBMCRecordingRepo{t: t, openErrs: tc.openErrs}

			w := setupDeploymentWorld(t, ctx, deploymentWorldConfig{
				forceReboot:   true,
				resolution:    deploymentTestResolution(),
				worldOptions:  tc.worldOptions,
				bmcRecording:  recordings,
				wrapBMCClient: emulateBMCRecording(t, calls),
			})

			request := deploymentTestRequest(w.tokenUUID)
			request.RecordBMC = true

			err := w.service.DeployByName(ctx, worldServerName, request)
			require.NoError(t, err)

			cancel := func(t *testing.T, ctx context.Context, svc provisioning.ServerService, server provisioning.Server) {
				t.Helper()

				if server.StatusInternal.Deployment.State == tc.cancelAt {
					require.NoError(t, svc.CancelDeploymentByName(ctx, worldServerName, tc.cancelSkipCleanup))
				}
			}

			// Run test
			server := driveDeployment(t, ctx, w, false, cancel)

			// Assert
			deployment := server.StatusInternal.Deployment

			require.Equal(t, tc.wantState, deployment.State)
			require.Equal(t, tc.wantOpens, recordings.openCount(), "the recording is opened once per service")
			require.Equal(t, tc.wantFinalized, recordings.finalized(), "the recording is finalized once with the final state")
			require.Equal(t, tc.wantUnscoped, calls.unscopedCalls(), "only the BMC calls outside of a deployment step are not recorded")

			for _, state := range tc.wantScopedStates {
				require.Positive(t, calls.scopedStates()[state], "the BMC calls of state %q are recorded", state)
			}

			require.NotNil(t, deployment.BMCRecording, "the deployment keeps the state of the recording")
			require.Equal(t, provisioning.BMCRecordingID(deployment.StartedAt), deployment.BMCRecording.ID)
			require.Positive(t, deployment.BMCRecording.Exchanges, "the deployment keeps the counts of the recording")
			require.Equal(t, recordings.exchanges(), deployment.BMCRecording.Exchanges, "the deployment keeps the final counts")
		})
	}
}

func TestServerService_DeploymentControlLoopDoesNotRecordBMCUnlessRequested(t *testing.T) {
	ctx := t.Context()

	// Setup
	calls := &bmcRecordingCalls{scoped: map[api.ServerDeploymentState]int{}}
	recordings := &fakeBMCRecordingRepo{t: t}

	w := setupDeploymentWorld(t, ctx, deploymentWorldConfig{
		forceReboot:   true,
		resolution:    deploymentTestResolution(),
		bmcRecording:  recordings,
		wrapBMCClient: emulateBMCRecording(t, calls),
	})

	err := w.service.DeployByName(ctx, worldServerName, deploymentTestRequest(w.tokenUUID))
	require.NoError(t, err)

	// Run test
	server := driveDeployment(t, ctx, w, false)

	// Assert
	require.Equal(t, api.ServerDeploymentStateCompleted, server.StatusInternal.Deployment.State)
	require.Zero(t, recordings.openCount(), "no recording is opened")
	require.Empty(t, calls.scopedStates(), "no BMC call carries a recording scope")
	require.NotEmpty(t, calls.unscopedCalls(), "the BMC is called")
	require.Nil(t, server.StatusInternal.Deployment.BMCRecording, "the deployment has no recording")
}

func TestServerService_DeployByNameRecordBMCWithoutRepo(t *testing.T) {
	ctx := t.Context()

	// Setup
	w := setupDeploymentWorld(t, ctx, deploymentWorldConfig{
		forceReboot: true,
		resolution:  deploymentTestResolution(),
	})

	request := deploymentTestRequest(w.tokenUUID)
	request.RecordBMC = true

	// Run test
	err := w.service.DeployByName(ctx, worldServerName, request)

	// Assert
	errassert.OperationNotPermittedError(t, err)

	server, err := w.repo.GetByName(ctx, worldServerName)
	require.NoError(t, err)
	require.Nil(t, server.StatusInternal.Deployment, "no deployment is started")
}
