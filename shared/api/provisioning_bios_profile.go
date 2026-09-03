package api

import (
	"maps"
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
}
