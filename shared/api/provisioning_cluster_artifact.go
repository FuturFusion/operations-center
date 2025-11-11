package api

import (
	"time"
)

// ClusterArtifact defines an artifact, which belongs to a given cluster.
//
// swagger:model
type ClusterArtifact struct {
	// Cluster the artifact belongs to.
	// Example: cluster01
	Cluster string `json:"cluster" yaml:"cluster"`

	// Name of the artifact
	// Example: terraform-configuration
	Name string `json:"name" yaml:"name"`

	// Description of the cluster artifact.
	// Example: Terraform configuration.
	Description string `json:"description" yaml:"description"`

	// Properties contains properties of the artifact as key/value pairs.
	// Example (in YAML notation for readability):
	//   properties:
	//     arch: x86_64
	//     os: linux
	Properties ConfigMap `json:"properties" yaml:"properties"`

	// Files contains the list of files, that form the given artifact.
	Files []ClusterArtifactFile `json:"files" yaml:"files"`

	// LastUpdated is the time, when this information has been updated for the
	// last time in RFC3339 format.
	// Example: 2024-11-12T16:15:00Z
	LastUpdated time.Time `json:"last_updated" yaml:"last_updated"`
}

// ClusterArtifactFile defines a single file of a cluster artifact.
//
// swagger:model
type ClusterArtifactFile struct {
	// Name of the artifact file.
	// Example: somefile.txt
	Name string `json:"name" yaml:"name"`

	// MimeType of the artifact file, used when the file content is returned
	// to set the correct Content-Type header.
	// Example: text/plain
	MimeType string `json:"mime_type" yaml:"mime_type"`

	// Size of the File in bytes.
	// Example: 54300000
	Size int64 `json:"size" yaml:"size"`
}
