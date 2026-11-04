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

// GetBMCRecordingArchive returns the zip of a recording. With anonymize, the
// identifying values of the server are replaced, so the archive can be
// published.
func (s *serverService) GetBMCRecordingArchive(ctx context.Context, name string, id string, anonymize bool) (_ io.ReadCloser, size int, _ error) {
	server, err := s.getBMCRecordingServer(ctx, name)
	if err != nil {
		return nil, 0, err
	}

	err = provisioning.ValidateBMCRecordingID(id)
	if err != nil {
		return nil, 0, err
	}

	var rewrite *provisioning.BMCRecordingRewrite
	if anonymize {
		manifest, err := s.bmcRecording.GetByID(ctx, name, id)
		if err != nil {
			return nil, 0, err
		}

		rewrite = provisioning.NewBMCRecordingRewrite(*server, *manifest)
	}

	return s.bmcRecording.GetArchive(ctx, name, id, rewrite)
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

func (s *serverService) checkBMCRecordingServer(ctx context.Context, name string) error {
	_, err := s.getBMCRecordingServer(ctx, name)

	return err
}

// getBMCRecordingServer makes an unknown server fail with not found, even if
// recordings of a deleted server with the same name are left.
func (s *serverService) getBMCRecordingServer(ctx context.Context, name string) (*provisioning.Server, error) {
	if s.bmcRecording == nil {
		return nil, domain.NewErrorf(domain.ErrOperationNotPermitted, "", "Recording the BMC traffic is not available")
	}

	if name == "" {
		return nil, domain.NewErrorf(domain.ErrOperationNotPermitted, "", "Server name cannot be empty")
	}

	server, err := s.repo.GetByName(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("Failed to get server %q by name: %w", name, err)
	}

	return server, nil
}
