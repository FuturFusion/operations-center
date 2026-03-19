package lifecycle

import (
	"crypto/tls"

	"github.com/maniartech/signals"

	"github.com/FuturFusion/operations-center/shared/api"
	apisystem "github.com/FuturFusion/operations-center/shared/api/system"
)

var (
	ServerCertificateUpdateSignal = signals.NewSync[tls.Certificate]()

	NetworkUpdateSignal = signals.NewSync[apisystem.Network]()

	SecurityUpdateSignal                    = signals.NewSync[apisystem.Security]()
	SecurityTrustedHTTPSProxiesUpdateSignal = signals.NewSync[[]string]()
	SecurityACMEUpdateSignal                = signals.NewSync[apisystem.SecurityACME]()

	UpdatesValidateSignal = signals.NewSync[apisystem.Updates]()
	UpdatesUpdateSignal   = signals.NewSync[apisystem.Updates]()

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
