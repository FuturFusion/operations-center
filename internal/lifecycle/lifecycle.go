package lifecycle

import (
	"crypto/tls"

	"github.com/maniartech/signals"

	"github.com/FuturFusion/operations-center/shared/api"
)

var (
	ServerCertificateUpdateSignal = signals.NewSync[tls.Certificate]()

	NetworkUpdateSignal = signals.NewSync[api.SystemNetwork]()

	SecurityUpdateSignal                    = signals.NewSync[api.SystemSecurity]()
	SecurityTrustedHTTPSProxiesUpdateSignal = signals.NewSync[[]string]()
	SecurityACMEUpdateSignal                = signals.NewSync[api.SystemSecurityACME]()

	UpdatesValidateSignal = signals.NewSync[api.SystemUpdates]()
	UpdatesUpdateSignal   = signals.NewSync[api.SystemUpdates]()

	ClusterUpdateSignal = signals.NewSync[ClusterUpdateMessage]()

	ServerLifecycleSignal = signals.NewSync[ServerLifecycleMessage]()
)

type ClusterUpdateMessage struct {
	Operation ClusterUpdateOperation
	Name      string
	OldName   string
}

type ClusterUpdateOperation string

const (
	ClusterUpdateOperationCreate ClusterUpdateOperation = "create"
	ClusterUpdateOperationDelete ClusterUpdateOperation = "delete"
	ClusterUpdateOperationRename ClusterUpdateOperation = "rename"
)

type ServerLifecycleMessage struct {
	Server            string
	Cluster           *string
	ServerUpdateState api.ServerUpdateState
}
