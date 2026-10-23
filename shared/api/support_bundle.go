package api

import (
	"fmt"
)

// SupportBundleType is the kind of resource a support bundle is created for.
type SupportBundleType string

const (
	SupportBundleTypeCluster  SupportBundleType = "cluster"
	SupportBundleTypeProject  SupportBundleType = "project"
	SupportBundleTypeInstance SupportBundleType = "instance"
	SupportBundleTypeServer   SupportBundleType = "server"
)

var SupportBundleTypes = map[SupportBundleType]struct{}{
	SupportBundleTypeCluster:  {},
	SupportBundleTypeProject:  {},
	SupportBundleTypeInstance: {},
	SupportBundleTypeServer:   {},
}

func (t SupportBundleType) String() string {
	return string(t)
}

// MarshalText implements the encoding.TextMarshaler interface.
func (t SupportBundleType) MarshalText() ([]byte, error) {
	return []byte(t), nil
}

// UnmarshalText implements the encoding.TextUnmarshaler interface.
func (t *SupportBundleType) UnmarshalText(text []byte) error {
	_, ok := SupportBundleTypes[SupportBundleType(text)]
	if !ok {
		return fmt.Errorf("%q is not a valid support bundle type", string(text))
	}

	*t = SupportBundleType(text)

	return nil
}

// SupportBundlePost selects the affected resource a support bundle is created for.
//
// swagger:model
type SupportBundlePost struct {
	// Type of the affected resource, one of cluster, project, instance or server.
	// Example: instance
	Type SupportBundleType `json:"type" yaml:"type"`

	// Cluster holds the name of the cluster, required for the types cluster, project and instance.
	// Example: cluster01
	Cluster string `json:"cluster,omitempty" yaml:"cluster,omitempty"`

	// Project holds the name of the project, required for the types project and instance.
	// Example: default
	Project string `json:"project,omitempty" yaml:"project,omitempty"`

	// Name holds the name of the instance, required for the type instance.
	// Example: c1
	Name string `json:"name,omitempty" yaml:"name,omitempty"`

	// Server holds the name of the server, required for the type server.
	// Example: server01
	Server string `json:"server,omitempty" yaml:"server,omitempty"`
}
