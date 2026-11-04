package provisioning

import (
	"context"
	"io"

	"github.com/FuturFusion/operations-center/shared/api"
)

// BMCRecordingRepo stores the recordings of the BMC exchanges of deployments.
type BMCRecordingRepo interface {
	// Open creates the recording or reopens it for appending after a restart.
	Open(ctx context.Context, server Server, deployment ServerDeployment) (BMCRecordingSession, error)
	GetAll(ctx context.Context, serverName string) ([]api.BMCRecordingManifest, error)
	GetByID(ctx context.Context, serverName string, id string) (*api.BMCRecordingManifest, error)
	// GetArchive returns a zip of the recording. A nil rewrite returns it as recorded.
	GetArchive(ctx context.Context, serverName string, id string, rewrite *BMCRecordingRewrite) (_ io.ReadCloser, size int, _ error)
	GetFile(ctx context.Context, serverName string, id string, filename string) (_ io.ReadCloser, size int64, _ error)
	DeleteByID(ctx context.Context, serverName string, id string) error
	DeleteAll(ctx context.Context, serverName string) error
}

// BMCRecordingSession is an open recording.
type BMCRecordingSession interface {
	BMCRecorder

	// Finalize writes the manifest with the outcome and closes the recording.
	Finalize(ctx context.Context, deployment ServerDeployment) error
}
