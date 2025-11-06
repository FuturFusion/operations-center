package provisioning

import "github.com/maniartech/signals"

func (s clusterService) GetClusterUpdateSignal() signals.Signal[ClusterUpdateMessage] {
	return s.clusterUpdateSignal
}
