package api

import (
	"maps"
)

// Names of the UEFI secure boot key databases.
const (
	SecureBootDatabaseKEK = "KEK"
	SecureBootDatabaseDB  = "db"
	SecureBootDatabaseDBX = "dbx"
)

// BIOSSecureBootDatabase holds the secure boot certificates and signatures of a
// single secure boot database, that are allowed to stay during the
// initialization of a server.
//
// swagger:model
type BIOSSecureBootDatabase struct {
	// Certificates is keyed by the SHA256 fingerprint of the certificate in
	// lower case hex notation. A value of true keeps the certificate, a value of
	// false removes it.
	Certificates map[string]bool `json:"certificates,omitempty" yaml:"certificates,omitempty"`

	// Signatures is keyed by the signature value. A value of true keeps
	// the signature, a value of false removes it.
	Signatures map[string]bool `json:"signatures,omitempty" yaml:"signatures,omitempty"`
}

func (d BIOSSecureBootDatabase) Clone() BIOSSecureBootDatabase {
	return BIOSSecureBootDatabase{
		Certificates: maps.Clone(d.Certificates),
		Signatures:   maps.Clone(d.Signatures),
	}
}

// BIOSSecureBoot holds the secure boot configuration per secure boot database.
//
// swagger:model
type BIOSSecureBoot struct {
	// DB holds the signature database.
	DB BIOSSecureBootDatabase `json:"db" yaml:"db"`

	// DBX holds the forbidden signature database.
	DBX BIOSSecureBootDatabase `json:"dbx" yaml:"dbx"`

	// KEK holds the key exchange key database.
	KEK BIOSSecureBootDatabase `json:"kek" yaml:"kek"`
}

func (s BIOSSecureBoot) Clone() BIOSSecureBoot {
	return BIOSSecureBoot{
		DB:  s.DB.Clone(),
		DBX: s.DBX.Clone(),
		KEK: s.KEK.Clone(),
	}
}

// BIOSProfileResolution is the outcome of the resolution of the BIOS profiles
// for a server. It holds the BIOS attributes and the secure boot configuration
// accumulated from all the BIOS profiles matching the server, which are applied
// to the server before IncusOS is installed on it.
//
// swagger:model
type BIOSProfileResolution struct {
	// Profiles holds the names of the BIOS profiles, that contributed to the
	// resolution, in the order they have been applied.
	// Example: ["dell-poweredge", "dell-poweredge-r7x0"]
	Profiles []string `json:"profiles" yaml:"profiles"`

	// Attributes holds the BIOS attribute names and values to apply to the
	// server via BMC, e.g. {"SecureBoot": "Enabled", "TpmSecurity": "On"}.
	Attributes map[string]any `json:"attributes" yaml:"attributes"`

	// DeferredAttributes holds the BIOS attribute names and values, that are
	// applied to the server in a second pass, once the attributes above are in
	// effect, e.g. {"Tpm2Algorithm": "SHA256"}.
	DeferredAttributes map[string]any `json:"deferred_attributes" yaml:"deferred_attributes"`

	// SecureBoot holds the secure boot certificates and signatures, that are
	// allowed to stay during the initialization of the server.
	SecureBoot BIOSSecureBoot `json:"secure_boot" yaml:"secure_boot"`

	// Deployment holds the timings of the automated deployment, that deviate
	// from the defaults.
	Deployment ServerDeploymentSettings `json:"deployment,omitzero" yaml:"deployment,omitempty"`
}

// BIOSProfileMatch selects the servers a BIOS profile applies to. An empty
// field matches any value, a match without any field set matches every server.
//
// The string fields are regular expressions, which are matched
// case-insensitively against the complete value reported by the BMC.
//
// swagger:model
type BIOSProfileMatch struct {
	// Manufacturer is matched against the manufacturer of the server.
	// Example: Lenovo
	Manufacturer string `json:"manufacturer,omitempty" yaml:"manufacturer,omitempty"`

	// Model is matched against the model of the server.
	// Example: ThinkSystem SR6.*
	Model string `json:"model,omitempty" yaml:"model,omitempty"`

	// ProcessorManufacturer is matched against the manufacturer of the processor.
	// Example: AMD
	ProcessorManufacturer string `json:"processor_manufacturer,omitempty" yaml:"processor_manufacturer,omitempty"`

	// ProcessorArchitecture is matched against the architecture of the processor.
	// Example: x86
	ProcessorArchitecture string `json:"processor_architecture,omitempty" yaml:"processor_architecture,omitempty"`

	// ProcessorInstructionSet is matched against the instruction set of the processor.
	// Example: x86-64
	ProcessorInstructionSet string `json:"processor_instruction_set,omitempty" yaml:"processor_instruction_set,omitempty"`

	// CPUSockets is compared to the number of CPU sockets of the server.
	// Example: 2
	CPUSockets *int `json:"cpu_sockets,omitempty" yaml:"cpu_sockets,omitempty"`

	// HasTPM is compared to the presence of a trusted platform module.
	// Example: true
	HasTPM *bool `json:"has_tpm,omitempty" yaml:"has_tpm,omitempty"`

	// BIOSVersion is a semver constraint for the BIOS version of the server.
	// Example: >= 2.1.0
	BIOSVersion string `json:"bios_version,omitempty" yaml:"bios_version,omitempty"`
}

// BIOSProfileSecureBootDatabase holds the secure boot certificates and
// signatures of a single secure boot database as a BIOS profile defines them. A
// value of true keeps the entry, a value of false removes it and a null value
// drops what the BIOS profiles with a lower priority have set for the entry.
//
// swagger:model
type BIOSProfileSecureBootDatabase struct {
	// Certificates is keyed by the SHA256 fingerprint of the certificate in hex
	// notation.
	Certificates map[string]*bool `json:"certificates,omitempty" yaml:"certificates,omitempty"`

	// Signatures is keyed by the signature value.
	Signatures map[string]*bool `json:"signatures,omitempty" yaml:"signatures,omitempty"`
}

// BIOSProfileSecureBoot holds the secure boot configuration of a BIOS profile
// per secure boot database.
//
// swagger:model
type BIOSProfileSecureBoot struct {
	// DB holds the signature database.
	DB BIOSProfileSecureBootDatabase `json:"db" yaml:"db"`

	// DBX holds the forbidden signature database.
	DBX BIOSProfileSecureBootDatabase `json:"dbx" yaml:"dbx"`

	// KEK holds the key exchange key database.
	KEK BIOSProfileSecureBootDatabase `json:"kek" yaml:"kek"`
}

// BIOSProfile is a set of BIOS attributes and secure boot allow lists in the
// format of the BIOS profiles shipped with Operations Center.
//
// swagger:model
type BIOSProfile struct {
	// Name of the BIOS profile.
	// Example: lenovo-thinksystem
	Name string `json:"name" yaml:"name"`

	// Description of the BIOS profile.
	// Example: Lenovo ThinkSystem
	Description string `json:"description" yaml:"description"`

	// Match selects the servers the BIOS profile applies to.
	Match []BIOSProfileMatch `json:"match" yaml:"match"`

	// Priority orders the BIOS profiles matching a server, a higher priority
	// overwrites what a lower priority has contributed.
	// Example: 2000
	Priority int `json:"priority" yaml:"priority"`

	// Attributes holds the BIOS attribute names and values to apply to the
	// server via BMC, e.g. {"SecureBoot": "Enabled", "TpmSecurity": "On"}.
	Attributes map[string]any `json:"attributes,omitempty" yaml:"attributes,omitempty"`

	// DeferredAttributes holds the BIOS attribute names and values, that are
	// applied to the server in a second pass, once the attributes above are in
	// effect, e.g. {"Tpm2Algorithm": "SHA256"}.
	DeferredAttributes map[string]any `json:"deferred_attributes,omitempty" yaml:"deferred_attributes,omitempty"`

	// SecureBoot holds the secure boot certificates and signatures, that are
	// allowed to stay during the initialization of the server.
	SecureBoot BIOSProfileSecureBoot `json:"secure_boot" yaml:"secure_boot"`

	// Deployment holds the timings of the automated deployment, that deviate
	// from the defaults for the servers the BIOS profile applies to.
	Deployment ServerDeploymentSettings `json:"deployment,omitzero" yaml:"deployment,omitempty"`
}
