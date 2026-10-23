package supportbundle

import (
	"context"
	"encoding/json"
	"io"

	incusosapi "github.com/lxc/incus-os/incus-osd/api"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/shared/api"
)

type SupportBundleService interface {
	// Create returns the name and the content of a zip archive holding the
	// diagnostic data of the resource selected by req.
	Create(ctx context.Context, req api.SupportBundlePost) (filename string, _ io.ReadCloser, _ error)
}

type SupportBundleClientPort interface {
	Ping(ctx context.Context, endpoint provisioning.Endpoint) error
	IncusClient(ctx context.Context, endpoint provisioning.Endpoint) (provisioning.InstanceServer, error)
	GetOS(ctx context.Context, server provisioning.Server) (json.RawMessage, error)
	GetOSApplicationNames(ctx context.Context, server provisioning.Server) ([]string, error)
	GetOSApplication(ctx context.Context, server provisioning.Server, name string) (json.RawMessage, error)
	GetSystem(ctx context.Context, server provisioning.Server, resource string) (map[string]any, error)
	GetOSDebugLog(ctx context.Context, server provisioning.Server, entries int) ([]map[string]any, error)
	GetOSDebugKernel(ctx context.Context, server provisioning.Server) (incusosapi.DebugKernel, error)
	GetOSDebugProcesses(ctx context.Context, server provisioning.Server) (string, error)
	GetOSApplicationDebugLogs(ctx context.Context, server provisioning.Server, application string) (io.ReadCloser, error)
	GetInstanceLogfile(ctx context.Context, server provisioning.Server, project string, name string, filename string) (io.ReadCloser, error)
	GetInstanceConsoleLog(ctx context.Context, server provisioning.Server, project string, name string) (io.ReadCloser, error)
}

type ProvisioningClusterService interface {
	GetByName(ctx context.Context, name string) (*provisioning.Cluster, error)
}

type ProvisioningServerService interface {
	GetByName(ctx context.Context, name string) (*provisioning.Server, error)
	GetAllWithFilter(ctx context.Context, filter provisioning.ServerFilter) (provisioning.Servers, error)
}
