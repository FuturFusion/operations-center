package server

import (
	"context"
	"crypto/tls"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	repoMock "github.com/FuturFusion/operations-center/internal/provisioning/repo/mock"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
	"github.com/FuturFusion/operations-center/shared/api"
)

type testBMCRecordingSession struct {
	provisioning.BMCRecorder

	finalizeErr error
	finalized   []provisioning.ServerDeployment
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

	tests := []struct {
		name     string
		existing map[string]bmcRecordingSession
		openErr  error

		wantOpens      int
		wantSessionIDs map[string]string
		wantFinalized  []provisioning.ServerDeployment
	}{
		{
			name:     "success - opens the recording",
			existing: map[string]bmcRecordingSession{},

			wantOpens:      1,
			wantSessionIDs: map[string]string{"one": currentID},
		},
		{
			name: "success - keeps the open recording",
			existing: map[string]bmcRecordingSession{
				"one": {id: currentID, session: &testBMCRecordingSession{}},
			},

			wantOpens:      0,
			wantSessionIDs: map[string]string{"one": currentID},
		},
		{
			name: "success - replaces the recording of an earlier deployment",
			existing: map[string]bmcRecordingSession{
				"one": {id: earlierID, session: &testBMCRecordingSession{finalizeErr: boom.Error}},
			},

			wantOpens:      1,
			wantSessionIDs: map[string]string{"one": currentID},
			wantFinalized:  []provisioning.ServerDeployment{{}},
		},
		{
			name:     "error - open fails",
			existing: map[string]bmcRecordingSession{},
			openErr:  boom.Error,

			wantOpens:      1,
			wantSessionIDs: map[string]string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			existing, hasExisting := tc.existing["one"]

			repo := &repoMock.BMCRecordingRepoMock{
				OpenFunc: func(ctx context.Context, server provisioning.Server, deployment provisioning.ServerDeployment) (provisioning.BMCRecordingSession, error) {
					if tc.openErr != nil {
						return nil, tc.openErr
					}

					return &testBMCRecordingSession{}, nil
				},
			}

			svc := New(nil, nil, nil, nil, nil, nil, nil, tls.Certificate{}, WithBMCRecordingRepo(repo))
			svc.bmcRecordingSessions = tc.existing

			server := provisioning.Server{
				Name: "one",
				StatusInternal: provisioning.ServerStatusInternal{
					Deployment: &provisioning.ServerDeployment{
						State:     api.ServerDeploymentStateApplyBIOS,
						StartedAt: startedAt,
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

			if hasExisting {
				require.Equal(t, tc.wantFinalized, existing.session.(*testBMCRecordingSession).finalized, "the recording of an earlier deployment is finalized")
			}
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
