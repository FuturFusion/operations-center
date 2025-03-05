package main

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config map[string]*Entity

type Entity struct {
	PluralName             string           `yaml:"plural"`                    // plural form of the entity
	ObjectNamePropertyName string           `yaml:"object-name-property-name"` // Name property of the object
	ObjectType             string           `yaml:"object-type"`               // Go type used for object in model
	ObjectEmbedded         bool             `yaml:"-"`
	OmitProject            bool             `yaml:"omit-project"`            // if omit-project is provided, the entity does not have a relation to a project
	UsesEmbeddedPostType   bool             `yaml:"uses-embedded-post-type"` // if uses-embedded-post-type is provided, the name property is part of an embedded Post type
	ServerResource         bool             `yaml:"server-resource"`         // if server-resource is provided, the resource is bound to a server instead of the whole cluster
	IncusGetAllMethod      string           `yaml:"incus-get-all-method"`    // method of the Incus client to get all the entities, e.g. GetStoragePoolBucketsAllProjects
	IncusGetMethod         string           `yaml:"incus-get-method"`        // method of the Incus client to get the entities, e.g. GetStoragePoolBucketsAllProjects
	ParentName             string           `yaml:"parent"`                  // name of the parent entity, if any
	ParentPluralName       string           `yaml:"parent-plural"`           // plural form of the parent entity
	ParentObjectType       string           `yaml:"parent-object-type"`      // Go type used for object in model for the parent entity
	ExtraAttributes        []ExtraAttribute `yaml:"extra-attributes"`        // TODO: hack in order to add the "type" attribute for storage_volumes, semantically not totally correct approach.
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
