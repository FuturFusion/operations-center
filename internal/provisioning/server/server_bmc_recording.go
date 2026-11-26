package server

import (
	"context"
	"fmt"
	"io"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/shared/api"
)

func (s *serverService) GetBMCRecordings(ctx context.Context, name string) ([]api.BMCRecordingManifest, error) {
	err := s.checkBMCRecordingServer(ctx, name)
	if err != nil {
		return nil, err
	}

	return s.bmcRecording.GetAll(ctx, name)
}

func (s *serverService) GetBMCRecording(ctx context.Context, name string, id string) (*api.BMCRecordingManifest, error) {
	err := s.checkBMCRecording(ctx, name, id)
	if err != nil {
		return nil, err
	}

	return s.bmcRecording.GetByID(ctx, name, id)
}

// GetBMCRecordingArchive returns the zip of a recording. Anonymize is not
// implemented yet, the archive is always returned as recorded.
func (s *serverService) GetBMCRecordingArchive(ctx context.Context, name string, id string, anonymize bool) (_ io.ReadCloser, size int, _ error) {
	err := s.checkBMCRecording(ctx, name, id)
	if err != nil {
		return nil, 0, err
	}

	return s.bmcRecording.GetArchive(ctx, name, id, nil)
}

func (s *serverService) GetBMCRecordingFile(ctx context.Context, name string, id string, filename string) (_ io.ReadCloser, size int64, _ error) {
	err := s.checkBMCRecording(ctx, name, id)
	if err != nil {
		return nil, 0, err
	}

	return s.bmcRecording.GetFile(ctx, name, id, filename)
}

func (s *serverService) DeleteBMCRecording(ctx context.Context, name string, id string) error {
	err := s.checkBMCRecording(ctx, name, id)
	if err != nil {
		return err
	}

	return s.bmcRecording.DeleteByID(ctx, name, id)
}

func (s *serverService) checkBMCRecording(ctx context.Context, name string, id string) error {
	err := s.checkBMCRecordingServer(ctx, name)
	if err != nil {
		return err
	}

	return provisioning.ValidateBMCRecordingID(id)
}

// checkBMCRecordingServer makes an unknown server fail with not found, even if
// recordings of a deleted server with the same name are left.
func (s *serverService) checkBMCRecordingServer(ctx context.Context, name string) error {
	if s.bmcRecording == nil {
		return domain.NewErrorf(domain.ErrOperationNotPermitted, "", "Recording the BMC traffic is not available")
	}

	if name == "" {
		return domain.NewErrorf(domain.ErrOperationNotPermitted, "", "Server name cannot be empty")
	}

	_, err := s.repo.GetByName(ctx, name)
	if err != nil {
		return fmt.Errorf("Failed to get server %q by name: %w", name, err)
	}

	return nil
}
