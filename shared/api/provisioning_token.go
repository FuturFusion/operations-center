package api

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lxc/incus-os/incus-osd/api"
	"github.com/lxc/incus-os/incus-osd/api/images"
)

// Token defines a registration token for use during registration.
//
// swagger:model
type Token struct {
	TokenPut `yaml:",inline"`

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
	updateFileType images.UpdateFileType
}{
	ImageTypeISO: {
		fileExt:        ".iso",
		updateFileType: images.UpdateFileTypeImageISO,
	},
	ImageTypeRaw: {
		fileExt:        ".raw",
		updateFileType: images.UpdateFileTypeImageRaw,
	},
}

func (i ImageType) String() string {
	return string(i)
}

func (i ImageType) IsValid() bool {
	_, ok := imageTypes[i]
	return ok
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

func (i ImageType) UpdateFileType() images.UpdateFileType {
	return imageTypes[i].updateFileType
}

// TokenImagePost defines the configuration to generate a pre-seeded ISO or raw
// image for a given Token.
//
// Operations Center just passes through the provided configuration for
// applications.yaml, install.yaml and network.yaml as is without any validation
// of the provided configuration besides of ensuring it to be valid yaml.
//
// swagger:model
type TokenImagePost struct {
	// Type contains the type of image to be generated.
	// Possible values for status are: iso, raw
	// Example: iso
	Type ImageType `json:"type" yaml:"type"`

	// Architecture contains the CPU architecture the image should be generated
	// for.
	Architecture images.UpdateFileArchitecture `json:"architecture" yaml:"architecture"`

	// Seeds represents the seed configuration for e.g. applications.yaml,
	// install.yaml and network.yaml.
	Seeds TokenSeedConfigs `json:"seeds" yaml:"seeds"`
}

// TokenProviderConfig defines the provider configuration for a given token.
//
// swagger:model
type TokenProviderConfig struct {
	api.SystemProviderConfig `yaml:",inline"`

	Version string `json:"version" yaml:"version"`
}

type TokenSeedConfigs struct {
	// Applications represents the applications configuration (applications.yaml) to be included in the pre-seeded image.
	Applications map[string]any `json:"applications" yaml:"applications"`

	// Incus represents the incus preseed configuration (incus.yaml) fo be included in the pre-seeded image.
	Incus map[string]any `json:"incus" yaml:"incus"`

	// Install represents the install configuration (install.yaml) to be included in the pre-seeded image.
	Install map[string]any `json:"install" yaml:"install"`

	// MigrationManager represents the seed configuration for migration manager (migration-manager.yaml) to be included in the pre-seeded image.
	MigrationManager map[string]any `json:"migration_manager" yaml:"migration_manager"`

	// Network represents the network configuration (network.yaml) to be included in the pre-seeded image.
	Network map[string]any `json:"network" yaml:"network"`

	// OperationsCenter represents the seed configuration for operations center (operations-center.yaml) to be included in the pre-seeded image.
	OperationsCenter map[string]any `json:"operations_center" yaml:"operations_center"`

	// Update represents the seed configuration for updates (update.yaml) to be included in the pre-seeded image.
	Update map[string]any `json:"update" yaml:"update"`
}

// TokenSeedPost defines a named token seed configuration, for which a
// pre-seeded ISO or raw image can be fetched later.
//
// Operations Center just passes through the provided configuration for
// application.yaml, install.yaml and network.yaml as is without any validation
// of the provided configuration besides of ensuring it to be valid yaml.
//
// swagger:model
type TokenSeedPost struct {
	// Name contains the name of the token seed configuration.
	// Example: MyConfig
	Name string `json:"name" yaml:"name"`

	TokenSeedPut `yaml:",inline"`
}

// TokenSeedPut defines the updateable part of a named token seed
// configuration, for which a pre-seeded ISO or raw image can be fetched later.
//
// Operations Center just passes through the provided configuration for
// application.yaml, install.yaml and network.yaml as is without any validation
// of the provided configuration besides of ensuring it to be valid yaml.
//
// swagger:model
type TokenSeedPut struct {
	// Description contains the description of the token seed configuration.
	// Example: Configuration for lab servers.
	Description string `json:"description" yaml:"description"`

	// Public defines, if images generated based on the given token seed
	// configuration can be retrieved without authentication. If public is set to
	// `true`, no authentication is necessary, otherwise authentication is
	// required.
	// Example: true
	Public bool `json:"public" yaml:"public"`

	// Seeds represents the seed configuration for e.g. application.yaml,
	// install.yaml and network.yaml.
	Seeds TokenSeedConfigs `json:"seeds" yaml:"seeds"`
}

// TokenSeed defines a named token seed configuration, for which a
// pre-seeded ISO or raw image can be fetched later.
//
// Operations Center just passes through the provided configuration for
// application.yaml, install.yaml and network.yaml as is without any validation
// of the provided configuration besides of ensuring it to be valid yaml.
//
// swagger:model
type TokenSeed struct {
	TokenSeedPost `yaml:",inline"`

	// UUID of the token.
	// Example: b32d0079-c48b-4957-b1cb-bef54125c861
	Token uuid.UUID `json:"token_uuid" yaml:"token_uuid"`

	// LastUpdated is the time, when this information has been updated for the last time in RFC3339 format.
	// Example: 2024-11-12T16:15:00Z
	LastUpdated time.Time `json:"last_updated" yaml:"last_updated"`
}
