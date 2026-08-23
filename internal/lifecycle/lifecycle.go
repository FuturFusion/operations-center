package lifecycle

import (
	"crypto/tls"

	"github.com/google/uuid"
	"github.com/lxc/incus-os/incus-osd/api/images"
	"github.com/maniartech/signals"

	"github.com/FuturFusion/operations-center/shared/api"
	apisystem "github.com/FuturFusion/operations-center/shared/api/system"
)

var (
	ServerCertificateUpdateSignal = signals.NewSync[tls.Certificate]()

	NetworkUpdateSignal = signals.NewSync[apisystem.Network]()

	SecurityValidateSignal                  = signals.NewSync[apisystem.Security]()
	SecurityUpdateSignal                    = signals.NewSync[apisystem.Security]()
	SecurityTrustedHTTPSProxiesUpdateSignal = signals.NewSync[[]string]()
	SecurityACMEUpdateSignal                = signals.NewSync[apisystem.SecurityACME]()

	SettingsValidateSignal = signals.NewSync[apisystem.Settings]()
	SettingsUpdateSignal   = signals.NewSync[apisystem.Settings]()

	UpdatesValidateSignal = signals.NewSync[apisystem.Updates]()
	UpdatesUpdateSignal   = signals.NewSync[apisystem.Updates]()

	ClusterUpdateSignal = signals.NewSync[ClusterUpdateMessage]()

	ServerLifecycleSignal = signals.NewSync[ServerLifecycleMessage]()

	BMCVirtualMediaSignal = signals.NewSync[BMCVirtualMediaMessage]()
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

type BMCVirtualMediaOperation string

const (
	BMCVirtualMediaOperationPreAttach BMCVirtualMediaOperation = "pre-attach"
	BMCVirtualMediaOperationAttach    BMCVirtualMediaOperation = "attach"
	BMCVirtualMediaOperationDetach    BMCVirtualMediaOperation = "detach"
)

// BMCVirtualMediaMessage reports, that installation media is about to be
// attached to, has been attached to or has been detached from a virtual media
// device of a server via its BMC.
type BMCVirtualMediaMessage struct {
	Operation      BMCVirtualMediaOperation
	Server         string
	VirtualMediaID string
	TokenUUID      uuid.UUID
	Seed           string
	ImageType      api.ImageType
	Architecture   images.UpdateFileArchitecture
	Channel        string
}
