package api

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Token defines a registration token for use during registration.
//
// swagger:model
type Token struct {
	TokenPut

	// UUID of the token, which serves as the the token.
	// Example: b32d0079-c48b-4957-b1cb-bef54125c861
	UUID uuid.UUID `json:"uuid" yaml:"uuid"`
}

// TokenPut defines the configurable properties of Token.
//
// swagger:model
type TokenPut struct {
	// Value indicating, how many times the token might be used for registration.
	// Example: 10
	UsesRemaining int `json:"uses_remaining" yaml:"uses_remaining"`

	// The time at which the token expires in RFC3339 format with seconds precision.
	// Example: "2025-02-04T07:25:47Z"
	ExpireAt time.Time `json:"expire_at" yaml:"expire_at"`

	// Description of this token.
	// Example: "Test Environment"
	Description string `json:"description" yaml:"description"`
}

type ImageType string

const (
	ImageTypeISO ImageType = "iso"
	ImageTypeRaw ImageType = "raw"
)

var imageTypes = map[ImageType]struct {
	fileExt        string
	updateFileType UpdateFileType
}{
	ImageTypeISO: {
		fileExt:        ".iso",
		updateFileType: UpdateFileTypeImageISO,
	},
	ImageTypeRaw: {
		fileExt:        ".raw",
		updateFileType: UpdateFileTypeImageRaw,
	},
}

func (i ImageType) String() string {
	return string(i)
}

// MarshalText implements the encoding.TextMarshaler interface.
func (i ImageType) MarshalText() ([]byte, error) {
	return []byte(i), nil
}

// UnmarshalText implements the encoding.TextUnmarshaler interface.
func (i *ImageType) UnmarshalText(text []byte) error {
	if len(text) == 0 {
		return fmt.Errorf("image type is empty")
	}

	_, ok := imageTypes[ImageType(text)]
	if !ok {
		return fmt.Errorf("%q is not a valid image type", string(text))
	}

	*i = ImageType(text)

	return nil
}

func (i ImageType) FileExt() string {
	return imageTypes[i].fileExt
}

func (i ImageType) UpdateFileType() UpdateFileType {
	return imageTypes[i].updateFileType
}

// TokenImagePost defines the configuration to generate a pre-seeded ISO or raw
// image for a given Token.
//
// Operations Center just passes through the provided configuration for
// application.yaml, install.yaml and network.yaml as is without any validation
// of the provided configuration besides of ensuring it to be valid yaml.
//
// swagger:model
type TokenImagePost struct {
	// Type contains the type of image to be generated.
	// Possible values for status are: iso, raw
	// Example: iso
	Type ImageType `json:"type" yaml:"type"`

	// Seeds represents the seed configuration for e.g. application.yaml,
	// install.yaml and network.yaml.
	Seeds TokenImagePostSeeds `json:"seeds" yaml:"seeds"`
}

type TokenImagePostSeeds struct {
	// Applications represents the applications configuration (application.yaml) to be included in the pre-seeded image.
	Applications map[string]any `json:"applications" yaml:"applications"`

	// Network represents the network configuration (network.yaml) to be included in the pre-seeded image.
	Network map[string]any `json:"network" yaml:"network"`

	// Install represents the install configuration (install.yaml) to be included in the pre-seeded image.
	Install map[string]any `json:"install" yaml:"install"`
}
