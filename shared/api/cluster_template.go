package api

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// ClusterTemplatePost defines a template, which can be used to form a cluster
// of servers running Hypervisor OS.
//
// swagger:model
type ClusterTemplatePost struct {
	ClusterTemplatePut `yaml:",inline"`

	// A human-friendly name for this cluster config template.
	// Example: MyTemplate
	Name string `json:"name" yaml:"name"`
}

// ClusterTemplatePut represents the fields available for update.
//
// swagger:model
type ClusterTemplatePut struct {
	// Description of the cluster config template.
	// Example: Cluster configuration for production clusters.
	Description string `json:"description" yaml:"description"`

	// ServiceConfigTemplate represents a template the service config for cluster
	// creation.
	// It contains contains the configuration for each service, which should be
	// configured on Hypervisor OS.
	// Operations Center is simply passing forward the settings to Hypervisor OS.
	// For details about the configuration settings available refer to the service
	// API definitions in https://github.com/lxc/incus-os/tree/main/incus-osd/api.
	ServiceConfigTemplate string `json:"service_config_template" yaml:"service_config_template"`

	// ApplicationConfigTemplate represents a template for the application config
	// for cluster creation.
	// It contains the seed configuration for the application, which is
	// applied during post clustering. This configuration is application specific.
	ApplicationConfigTemplate string `json:"application_config_template" yaml:"application_config_template"`

	// Variables is a map of variable names to variable definitions.
	Variables ClusterTemplateVariables `json:"variables" yaml:"variables"`
}

// ClusterTemplateVariables defines the variables, that can be used in a
// cluster config template.
type ClusterTemplateVariables map[string]ClusterTemplateVariable

// Value implements the sql driver.Valuer interface.
func (c ClusterTemplateVariables) Value() (driver.Value, error) {
	return json.Marshal(c)
}

// Scan implements the sql.Scanner interface.
func (c *ClusterTemplateVariables) Scan(value any) error {
	if value == nil {
		return fmt.Errorf("null is not a valid token seeds")
	}

	switch v := value.(type) {
	case string:
		if len(v) == 0 {
			*c = ClusterTemplateVariables{}
			return nil
		}

		return json.Unmarshal([]byte(v), c)
	case []byte:
		if len(v) == 0 {
			*c = ClusterTemplateVariables{}
			return nil
		}

		return json.Unmarshal(v, c)
	default:
		return fmt.Errorf("type %T is not supported for token seeds", value)
	}
}

// ClusterTemplateVariable defines the properties of a variable, that
// can be used in a cluster config template.
type ClusterTemplateVariable struct {
	// Description describes a cluster config template variable.
	// Example: Long name for the cluster.
	Description string `json:"description" yaml:"description"`

	// DefaultValue is the default value applied for a cluster config variable
	// if no value is provided for the variable.
	// Example: Incus cluster
	DefaultValue string `json:"default" yaml:"default"`
}

// ClusterTemplate defines a template, which can be used to form a cluster
// of servers running Hypervisor OS.
//
// swagger:model
type ClusterTemplate struct {
	ClusterTemplatePost `yaml:",inline"`

	// LastUpdated is the time, when this information has been updated for the last time in RFC3339 format.
	// Example: 2024-11-12T16:15:00Z
	LastUpdated time.Time `json:"last_updated" yaml:"last_updated"`
}
