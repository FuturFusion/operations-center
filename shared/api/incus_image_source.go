package api

import "time"

// IncusImageSourceLocal is the name of the image source of manually uploaded
// images. It always exists and can not be changed.
const IncusImageSourceLocal = "local"

// ImageSourcePut represents the fields available for update for an image source.
//
// swagger:model
type ImageSourcePut struct {
	// URL of the image source.
	// Example: https://images.linuxcontainers.org
	URL string `json:"url" yaml:"url"`

	// FilterExpression applied to each file when fetching images from the image
	// source. Only files matching the filter expression are fetched.
	// Example: architecture == "amd64"
	FilterExpression string `json:"filter_expression" yaml:"filter_expression"`
}

type ImageSourcePost struct {
	ImageSourcePut `yaml:",inline"`

	// Name of the image source. It can only contain lower case letters, digits
	// and underscores. The names "local" and "create" are reserved.
	// Example: linuxcontainers
	Name string `json:"name" yaml:"name"`
}

// ImageSource defines an image source like images.linuxcontainers.org.
//
// The model name is set explicitly, because the type name collides with
// ImageSource from github.com/lxc/incus/v7/shared/api. Without it, go-swagger
// derives both names from the package paths, which makes them change whenever
// an unrelated colliding type is added.
//
// swagger:model IncusImageSource
type ImageSource struct {
	ImageSourcePost `yaml:",inline"`

	// LastUpdated is the time, when this information has been updated for the last time in RFC3339 format.
	// Example: 2024-11-12T16:15:00Z
	LastUpdated time.Time `json:"last_updated" yaml:"last_updated"`
}
