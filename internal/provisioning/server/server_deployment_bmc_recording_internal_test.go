package server

import (
	"context"
	"crypto/tls"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	repoMock "github.com/FuturFusion/operations-center/internal/provisioning/repo/mock"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
	"github.com/FuturFusion/operations-center/shared/api"
)

type testBMCRecordingSession struct {
	provisioning.BMCRecorder

	stats       provisioning.BMCRecordingStats
	finalizeErr error
	finalized   []provisioning.ServerDeployment
}

func (s *testBMCRecordingSession) Stats() provisioning.BMCRecordingStats {
	return s.stats
}

func (s *testBMCRecordingSession) Finalize(ctx context.Context, deployment provisioning.ServerDeployment) error {
	s.finalized = append(s.finalized, deployment)

	return s.finalizeErr
}

func TestServerService_ensureBMCRecordingSession(t *testing.T) {
	t.Parallel()

	startedAt := time.Date(2026, 10, 27, 8, 15, 0, 0, time.UTC)
	currentID := provisioning.BMCRecordingID(startedAt)
	earlierID := provisioning.BMCRecordingID(startedAt.Add(-time.Hour))

	noSessions := func(*testBMCRecordingSession) map[string]bmcRecordingSession {
		return map[string]bmcRecordingSession{}
	}

	sessionWithID := func(id string) func(*testBMCRecordingSession) map[string]bmcRecordingSession {
		return func(session *testBMCRecordingSession) map[string]bmcRecordingSession {
			return map[string]bmcRecordingSession{"one": {id: id, session: session}}
		}
	}

	tests := []struct {
		name        string
		sessions    func(session *testBMCRecordingSession) map[string]bmcRecordingSession
		finalizeErr error
		openErr     error

		wantOpens      int
		wantSessionIDs map[string]string
		wantFinalized  []provisioning.ServerDeployment
	}{
		{
			name:     "success - opens the recording",
			sessions: noSessions,

			wantOpens:      1,
			wantSessionIDs: map[string]string{"one": currentID},
		},
		{
			name:     "success - keeps the open recording",
			sessions: sessionWithID(currentID),

			wantOpens:      0,
			wantSessionIDs: map[string]string{"one": currentID},
		},
		{
			name:        "success - replaces the recording of an earlier deployment",
			sessions:    sessionWithID(earlierID),
			finalizeErr: boom.Error,

			wantOpens:      1,
			wantSessionIDs: map[string]string{"one": currentID},
			wantFinalized:  []provisioning.ServerDeployment{{}},
		},
		{
			name:     "error - open fails",
			sessions: noSessions,
			openErr:  boom.Error,

			wantOpens:      1,
			wantSessionIDs: map[string]string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			repo := &repoMock.BMCRecordingRepoMock{
				OpenFunc: func(ctx context.Context, server provisioning.Server, deployment provisioning.ServerDeployment) (provisioning.BMCRecordingSession, error) {
					return &testBMCRecordingSession{}, tc.openErr
				},
			}

			existing := &testBMCRecordingSession{finalizeErr: tc.finalizeErr}

			svc := New(nil, nil, nil, nil, nil, nil, nil, tls.Certificate{}, WithBMCRecordingRepo(repo))
			svc.bmcRecordingSessions = tc.sessions(existing)

			server := provisioning.Server{
				Name: "one",
				StatusInternal: provisioning.ServerStatusInternal{
					Deployment: &provisioning.ServerDeployment{
						State:        api.ServerDeploymentStateApplyBIOS,
						StartedAt:    startedAt,
						BMCRecording: &provisioning.ServerDeploymentBMCRecording{ID: currentID},
					},
				},
			}

			// Run test
			svc.ensureBMCRecordingSession(t.Context(), slog.Default(), server)

			// Assert
			require.Len(t, repo.OpenCalls(), tc.wantOpens, "the recording is only opened, if it is not open yet")

			ids := map[string]string{}
			for name, session := range svc.bmcRecordingSessions {
				ids[name] = session.id
			}

			require.Equal(t, tc.wantSessionIDs, ids, "the open recordings")
			require.Equal(t, tc.wantFinalized, existing.finalized, "the recording of an earlier deployment is finalized")
		})
	}
}

func TestServerService_settleBMCRecording(t *testing.T) {
	t.Parallel()

	const id = "20261027T081500Z"

	stats := provisioning.BMCRecordingStats{Exchanges: 3, Bytes: 300, TruncatedExchanges: 1}
	settled := &provisioning.ServerDeploymentBMCRecording{ID: id, Exchanges: 3, Bytes: 300, TruncatedExchanges: 1}

	deployment := func(state api.ServerDeploymentState, recording *provisioning.ServerDeploymentBMCRecording) *provisioning.Server {
		return &provisioning.Server{
			Name: "one",
			StatusInternal: provisioning.ServerStatusInternal{
				Deployment: &provisioning.ServerDeployment{
					State:        state,
					BMCRecording: recording,
				},
			},
		}
	}

	tests := []struct {
		name   string
		server *provisioning.Server
		getErr error

		wantUpdates     int
		wantFinalized   []provisioning.ServerDeployment
		wantSessions    int
		wantDeleteCalls int
	}{
		{
			name:   "success - active deployment gets the counts",
			server: deployment(api.ServerDeploymentStateApplyBIOS, &provisioning.ServerDeploymentBMCRecording{ID: id}),

			wantUpdates:  1,
			wantSessions: 1,
		},
		{
			name:   "success - unchanged counts are not written",
			server: deployment(api.ServerDeploymentStateApplyBIOS, settled),

			wantSessions: 1,
		},
		{
			name:   "success - ended deployment finalizes the recording",
			server: deployment(api.ServerDeploymentStateCompleted, settled),

			wantFinalized: []provisioning.ServerDeployment{{State: api.ServerDeploymentStateCompleted, BMCRecording: settled}},
		},
		{
			name:   "success - session of an earlier deployment is closed",
			server: deployment(api.ServerDeploymentStateApplyBIOS, &provisioning.ServerDeploymentBMCRecording{ID: "20261027T091500Z"}),

			wantFinalized: []provisioning.ServerDeployment{{}},
		},
		{
			name:   "success - session without deployment is closed",
			server: &provisioning.Server{Name: "one"},

			wantFinalized: []provisioning.ServerDeployment{{}},
		},
		{
			name:   "success - deleted server removes the recordings",
			getErr: domain.ErrNotFound,

			wantFinalized:   []provisioning.ServerDeployment{{}},
			wantDeleteCalls: 1,
		},
		{
			name:   "error - get server fails",
			getErr: boom.Error,

			wantSessions: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			serverRepo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return tc.server, tc.getErr
				},
				UpdateFunc: func(ctx context.Context, server provisioning.Server) error {
					return nil
				},
			}

			recordingRepo := &repoMock.BMCRecordingRepoMock{
				DeleteAllFunc: func(ctx context.Context, serverName string) error {
					return nil
				},
			}

			session := &testBMCRecordingSession{stats: stats}

			svc := New(serverRepo, nil, nil, nil, nil, nil, nil, tls.Certificate{}, WithBMCRecordingRepo(recordingRepo))
			svc.bmcRecordingSessions["one"] = bmcRecordingSession{id: id, session: session}

			// Run test
			svc.settleBMCRecording(t.Context(), slog.Default(), "one")

			// Assert
			require.Len(t, serverRepo.UpdateCalls(), tc.wantUpdates, "the counts are only written when they change")
			require.Equal(t, tc.wantFinalized, session.finalized, "the recording is finalized once it is no longer needed")
			require.Len(t, svc.bmcRecordingSessions, tc.wantSessions, "a finalized session is dropped")
			require.Len(t, recordingRepo.DeleteAllCalls(), tc.wantDeleteCalls, "the recordings of a deleted server are removed")
		})
	}
}

func TestServerService_deleteBMCRecordings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		session      *testBMCRecordingSession
		deployment   *provisioning.ServerDeployment
		deleteAllErr error

		wantFinalized []provisioning.ServerDeployment
	}{
		{
			name:       "success - finalizes the open recording",
			session:    &testBMCRecordingSession{},
			deployment: &provisioning.ServerDeployment{State: api.ServerDeploymentStateWaitInstall},

			wantFinalized: []provisioning.ServerDeployment{{State: api.ServerDeploymentStateWaitInstall}},
		},
		{
			name:    "success - finalizes the open recording without a deployment",
			session: &testBMCRecordingSession{finalizeErr: boom.Error},

			wantFinalized: []provisioning.ServerDeployment{{}},
		},
		{
			name:         "error - delete fails",
			session:      &testBMCRecordingSession{},
			deleteAllErr: boom.Error,

			wantFinalized: []provisioning.ServerDeployment{{}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			repo := &repoMock.BMCRecordingRepoMock{
				DeleteAllFunc: func(ctx context.Context, serverName string) error {
					return tc.deleteAllErr
				},
			}

			svc := New(nil, nil, nil, nil, nil, nil, nil, tls.Certificate{}, WithBMCRecordingRepo(repo))
			svc.bmcRecordingSessions["one"] = bmcRecordingSession{id: "20261027T081500Z", session: tc.session}

			// Run test
			svc.deleteBMCRecordings(t.Context(), "one", tc.deployment)

			// Assert
			require.Equal(t, tc.wantFinalized, tc.session.finalized, "the open recording is closed before it is deleted")
			require.Empty(t, svc.bmcRecordingSessions, "the session is dropped")
			require.Len(t, repo.DeleteAllCalls(), 1, "the recordings are deleted")
		})
	}
}
