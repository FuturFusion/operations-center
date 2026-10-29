package server

import (
	"context"
	"log/slog"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/util/logger"
)

// bmcRecordingSession is an open recording together with the ID of the
// recording, so a session of an earlier deployment is not reused.
type bmcRecordingSession struct {
	id      string
	session provisioning.BMCRecordingSession
}

func (s *serverService) lookupBMCRecordingSession(name string) (bmcRecordingSession, bool) {
	s.deploymentControlLoopMu.Lock()
	defer s.deploymentControlLoopMu.Unlock()

	session, ok := s.bmcRecordingSessions[name]

	return session, ok
}

func (s *serverService) storeBMCRecordingSession(name string, session bmcRecordingSession) {
	s.deploymentControlLoopMu.Lock()
	defer s.deploymentControlLoopMu.Unlock()

	s.bmcRecordingSessions[name] = session
}

func (s *serverService) dropBMCRecordingSession(name string) (bmcRecordingSession, bool) {
	s.deploymentControlLoopMu.Lock()
	defer s.deploymentControlLoopMu.Unlock()

	session, ok := s.bmcRecordingSessions[name]
	delete(s.bmcRecordingSessions, name)

	return session, ok
}

// ensureBMCRecordingSession opens the recording of the deployment, if it is not
// open yet. This also reopens it after a restart of the daemon. A failure is
// only logged, since a recording must never fail a deployment. The next step
// tries again.
func (s *serverService) ensureBMCRecordingSession(ctx context.Context, log *slog.Logger, server provisioning.Server) {
	if s.bmcRecording == nil {
		return
	}

	deployment := server.StatusInternal.Deployment
	id := provisioning.BMCRecordingID(deployment.StartedAt)

	existing, ok := s.lookupBMCRecordingSession(server.Name)
	if ok && existing.id == id {
		return
	}

	if ok {
		s.dropBMCRecordingSession(server.Name)

		log.WarnContext(ctx, "Closing BMC recording of an earlier deployment", slog.String("recording_id", existing.id))

		err := existing.session.Finalize(ctx, provisioning.ServerDeployment{})
		if err != nil {
			log.ErrorContext(ctx, "Failed to finalize BMC recording", slog.String("recording_id", existing.id), logger.Err(err))
		}
	}

	session, err := s.bmcRecording.Open(ctx, server, *deployment)
	if err != nil {
		log.ErrorContext(ctx, "Failed to open BMC recording", slog.String("recording_id", id), logger.Err(err))
		return
	}

	s.storeBMCRecordingSession(server.Name, bmcRecordingSession{
		id:      id,
		session: session,
	})
}

// settleBMCRecording keeps the counts of the recording with the deployment and
// finalizes the recording, once the deployment has ended. Every terminal
// transition happens within a deployment step, so this catches all of them.
// Failures are only logged, since a recording must never fail a deployment.
func (s *serverService) settleBMCRecording(ctx context.Context, log *slog.Logger, name string) {
	entry, ok := s.lookupBMCRecordingSession(name)
	if !ok {
		return
	}

	server, err := s.repo.GetByName(ctx, name)
	if err != nil {
		log.ErrorContext(ctx, "Failed to get server for BMC recording", logger.Err(err))
		return
	}

	deployment := server.StatusInternal.Deployment
	if deployment == nil || provisioning.BMCRecordingID(deployment.StartedAt) != entry.id {
		return
	}

	stats := entry.session.Stats()
	recording := provisioning.ServerDeploymentBMCRecording{
		ID:                 entry.id,
		Exchanges:          stats.Exchanges,
		Bytes:              stats.Bytes,
		TruncatedExchanges: stats.TruncatedExchanges,
	}

	if deployment.BMCRecording == nil || *deployment.BMCRecording != recording {
		err = s.updateDeployment(ctx, name, func(deployment *provisioning.ServerDeployment) {
			deployment.BMCRecording = &recording
		})
		if err != nil {
			log.ErrorContext(ctx, "Failed to update BMC recording counts", logger.Err(err))
		}

		deployment.BMCRecording = &recording
	}

	if deployment.IsActive() {
		return
	}

	s.dropBMCRecordingSession(name)

	err = entry.session.Finalize(ctx, *deployment)
	if err != nil {
		log.ErrorContext(ctx, "Failed to finalize BMC recording", slog.String("recording_id", entry.id), logger.Err(err))
	}
}

// deleteBMCRecordings removes the recordings of a deleted server. A failure is
// only logged, since the server is gone already.
func (s *serverService) deleteBMCRecordings(ctx context.Context, name string, deployment *provisioning.ServerDeployment) {
	if s.bmcRecording == nil {
		return
	}

	entry, ok := s.dropBMCRecordingSession(name)
	if ok {
		finalDeployment := provisioning.ServerDeployment{}
		if deployment != nil {
			finalDeployment = *deployment
		}

		err := entry.session.Finalize(ctx, finalDeployment)
		if err != nil {
			slog.ErrorContext(ctx, "Failed to finalize BMC recording", slog.String("name", name), slog.String("recording_id", entry.id), logger.Err(err))
		}
	}

	err := s.bmcRecording.DeleteAll(ctx, name)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to delete BMC recordings", slog.String("name", name), logger.Err(err))
	}
}
