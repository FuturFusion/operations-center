package inventory

import (
	"time"

	incusapi "github.com/lxc/incus/v6/shared/api"
)

type Instance struct {
	ID          int
	ClusterID   int
	ServerID    int
	ProjectName string
	Name        string
	Object      incusapi.InstanceFull
	LastUpdated time.Time
}

func (s Instance) Validate() error {
	return nil
}

type Instances []Instance
