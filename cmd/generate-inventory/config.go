package main

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config map[string]*Entity

type Entity struct {
	PluralName             string           `yaml:"plural"`                    // plural form of the entity
	RequiresExtension      string           `yaml:"requires-extension"`        // If provided, the incus API needs to support the given extension in order to support this resoruce type
	ObjectNamePropertyName string           `yaml:"object-name-property-name"` // Name property of the object
	ObjectDisplayName      string           `yaml:"obect-display-name"`        // Display name of the object
	ObjectType             string           `yaml:"object-type"`               // Go type used for object in model
	ObjectEmbedded         bool             `yaml:"-"`
	OmitProject            bool             `yaml:"omit-project"`              // if omit-project is provided, the entity does not have a relation to a project
	UsesEmbeddedPostType   bool             `yaml:"uses-embedded-post-type"`   // if uses-embedded-post-type is provided, the name property is part of an embedded Post type
	ServerIDByLocation     bool             `yaml:"server-id-by-location"`     // if a resource has a location, it can be used to bind it to a server
	IsServerIDOptional     bool             `yaml:"is-server-id-optional"`     // if a resource only optionally has a location
	IsServerIDNotDefining  bool             `yaml:"is-server-id-not-defining"` // if a resource has a server_id, but it should not be considered part of the defining unique key
	IncusGetAllMethod      string           `yaml:"incus-get-all-method"`      // method of the Incus client to get all the entities, e.g. GetStoragePoolBucketsAllProjects
	IncusGetMethod         string           `yaml:"incus-get-method"`          // method of the Incus client to get the entities, e.g. GetStoragePoolBucketsAllProjects
	ParentName             string           `yaml:"parent"`                    // name of the parent entity, if any
	ParentPluralName       string           `yaml:"parent-plural"`             // plural form of the parent entity
	ParentObjectType       string           `yaml:"parent-object-type"`        // Go type used for object in model for the parent entity
	ExtraAttributes        []ExtraAttribute `yaml:"extra-attributes"`          // TODO: hack in order to add the "type" attribute for storage_volumes, semantically not totally correct approach.
	HasSyncFilter          bool             `yaml:"has-sync-filter"`           // if a resource has a sync filter, it is passed to the service. The implementation needs to be provided separately.
	HasParentFilter        bool             `yaml:"has-parent-filter"`         // if a resource has a parent filter, it is passed to the service. The implementation needs to be provided separately.
}

type ExtraAttribute struct {
	Name           string `yaml:"name"`
	Type           string `yaml:"type"`
	TestDummyValue string `yaml:"test-dummy-value"`
}

func (c *Config) LoadConfig(path string) error {
	contents, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	return yaml.Unmarshal(contents, c)
}
