package provisioning

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/Masterminds/semver/v3"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/shared/api"
)

// BIOSProfileMatch selects the servers a BIOS profile applies to. An empty
// field matches any value, a match without any field set matches every server.
//
// The string fields are regular expressions, which are matched
// case-insensitively against the complete value reported by the BMC.
type BIOSProfileMatch struct {
	// Manufacturer is matched against BMCData.ServerManufacturer.
	Manufacturer string `json:"manufacturer" yaml:"manufacturer"`

	// Model is matched against BMCData.ServerModel.
	Model string `json:"model" yaml:"model"`

	// ProcessorManufacturer is matched against BMCData.ServerProcessorManufacturer.
	ProcessorManufacturer string `json:"processor_manufacturer" yaml:"processor_manufacturer"`

	// ProcessorArchitecture is matched against BMCData.ServerProcessorArchitecture.
	ProcessorArchitecture string `json:"processor_architecture" yaml:"processor_architecture"`

	// ProcessorInstructionSet is matched against BMCData.ServerProcessorInstructionSet.
	ProcessorInstructionSet string `json:"processor_instruction_set" yaml:"processor_instruction_set"`

	// CPUSockets is compared to BMCData.ServerCPUSockets. If unset, the number
	// of CPU sockets is not taken into account.
	CPUSockets *int `json:"cpu_sockets" yaml:"cpu_sockets"`

	// HasTPM is compared to BMCData.ServerHasTPM. If unset, the presence of a
	// trusted platform module is not taken into account.
	HasTPM *bool `json:"has_tpm" yaml:"has_tpm"`

	// BIOSVersion is a semver constraint for BMCData.ServerBIOSVersion.
	// See https://github.com/Masterminds/semver#basic-comparisons for syntax.
	BIOSVersion string `json:"bios_version" yaml:"bios_version"`
}

func (m BIOSProfileMatch) regexpFields(data api.BMCData) []struct {
	name    string
	pattern string
	value   string
} {
	return []struct {
		name    string
		pattern string
		value   string
	}{
		{name: "manufacturer", pattern: m.Manufacturer, value: data.ServerManufacturer},
		{name: "model", pattern: m.Model, value: data.ServerModel},
		{name: "processor_manufacturer", pattern: m.ProcessorManufacturer, value: data.ServerProcessorManufacturer},
		{name: "processor_architecture", pattern: m.ProcessorArchitecture, value: data.ServerProcessorArchitecture},
		{name: "processor_instruction_set", pattern: m.ProcessorInstructionSet, value: data.ServerProcessorInstructionSet},
	}
}

// compileMatchPattern compiles pattern into a regular expression, that matches
// the complete value case-insensitively.
func compileMatchPattern(name string, pattern string) (*regexp.Regexp, error) {
	expression, err := regexp.Compile(`(?i)\A(?:` + pattern + `)\z`)
	if err != nil {
		return nil, domain.NewValidationErrf("Invalid %s pattern %q: %v", name, pattern, err)
	}

	return expression, nil
}

func (m BIOSProfileMatch) Validate() error {
	for _, field := range m.regexpFields(api.BMCData{}) {
		if field.pattern == "" {
			continue
		}

		_, err := compileMatchPattern(field.name, field.pattern)
		if err != nil {
			return err
		}
	}

	if m.CPUSockets != nil && *m.CPUSockets < 1 {
		return domain.NewValidationErrf("Invalid cpu_sockets %d, at least 1 CPU socket is required", *m.CPUSockets)
	}

	if m.BIOSVersion != "" {
		_, err := semver.NewConstraint(m.BIOSVersion)
		if err != nil {
			return domain.NewValidationErrf("Invalid BIOS version constraint %q: %v", m.BIOSVersion, err)
		}
	}

	return nil
}

// Matches reports, whether the BMC data collected for a server satisfies the match.
func (m BIOSProfileMatch) Matches(data api.BMCData) (bool, error) {
	for _, field := range m.regexpFields(data) {
		if field.pattern == "" {
			continue
		}

		expression, err := compileMatchPattern(field.name, field.pattern)
		if err != nil {
			return false, err
		}

		if !expression.MatchString(strings.TrimSpace(field.value)) {
			return false, nil
		}
	}

	if m.CPUSockets != nil && *m.CPUSockets != data.ServerCPUSockets {
		return false, nil
	}

	if m.HasTPM != nil && *m.HasTPM != data.ServerHasTPM {
		return false, nil
	}

	if m.BIOSVersion != "" {
		constraint, err := semver.NewConstraint(m.BIOSVersion)
		if err != nil {
			return false, domain.NewValidationErrf("Invalid BIOS version constraint %q: %v", m.BIOSVersion, err)
		}

		version, err := semver.NewVersion(strings.TrimSpace(data.ServerBIOSVersion))
		if err != nil {
			// A BIOS version, that can not be interpreted, never satisfies a constraint.
			return false, nil
		}

		if !constraint.Check(version) {
			return false, nil
		}
	}

	return true, nil
}

// BIOSSecureBootDatabase holds the secure boot certificates and signatures of a
// single secure boot database, that are allowed to stay during the
// initialization of a server. A value of true keeps the entry, a value of false
// removes it. A null value drops the entry from the set accumulated during the
// resolution of the BIOS profiles.
type BIOSSecureBootDatabase struct {
	// Certificates is keyed by the SHA256 fingerprint of the certificate in
	// lower case hex notation.
	Certificates map[string]*bool `json:"certificates" yaml:"certificates"`

	// Signatures is keyed by the signature value.
	Signatures map[string]*bool `json:"signatures" yaml:"signatures"`
}

func (d BIOSSecureBootDatabase) Clone() BIOSSecureBootDatabase {
	return BIOSSecureBootDatabase{
		Certificates: maps.Clone(d.Certificates),
		Signatures:   maps.Clone(d.Signatures),
	}
}

type BIOSSecureBoot struct {
	DB  BIOSSecureBootDatabase `json:"db" yaml:"db"`
	DBX BIOSSecureBootDatabase `json:"dbx" yaml:"dbx"`
	KEK BIOSSecureBootDatabase `json:"kek" yaml:"kek"`
}

func (s BIOSSecureBoot) Clone() BIOSSecureBoot {
	return BIOSSecureBoot{
		DB:  s.DB.Clone(),
		DBX: s.DBX.Clone(),
		KEK: s.KEK.Clone(),
	}
}

func (s BIOSSecureBoot) IsEmpty() bool {
	for _, database := range []BIOSSecureBootDatabase{s.DB, s.DBX, s.KEK} {
		if len(database.Certificates) > 0 || len(database.Signatures) > 0 {
			return false
		}
	}

	return true
}

// BIOSProfile is a set of BIOS attributes, that is applied to the servers
// selected by any of its matches.
type BIOSProfile struct {
	Name        string             `json:"name"        yaml:"name"`
	Description string             `json:"description" yaml:"description"`
	Match       []BIOSProfileMatch `json:"match"       yaml:"match"`
	Priority    int                `json:"priority"    yaml:"priority"`

	// Attributes holds the BIOS attribute names and values to apply. A null
	// value drops the attribute from the set accumulated during the resolution
	// of the BIOS profiles, so the attribute is left untouched.
	Attributes map[string]any `json:"attributes" yaml:"attributes"`

	// SecureBoot holds the secure boot certificates and signatures, that are
	// allowed to stay during the initialization of the server.
	SecureBoot BIOSSecureBoot `json:"secure_boot" yaml:"secure_boot"`
}

func (p BIOSProfile) Validate() error {
	if p.Name == "" {
		return domain.NewValidationErrf("Invalid BIOS profile, name can not be empty")
	}

	if strings.ContainsAny(p.Name, nameProhibitedCharacters) {
		return domain.NewValidationErrf("Invalid BIOS profile %q, name can not contain any of %q", p.Name, nameProhibitedCharacters)
	}

	if len(p.Match) == 0 {
		return domain.NewValidationErrf("Invalid BIOS profile %q, at least one match is required", p.Name)
	}

	if len(p.Attributes) == 0 && p.SecureBoot.IsEmpty() {
		return domain.NewValidationErrf("Invalid BIOS profile %q, attributes and secure boot can not both be empty", p.Name)
	}

	for _, match := range p.Match {
		err := match.Validate()
		if err != nil {
			return domain.NewValidationErrf("Invalid BIOS profile %q: %v", p.Name, err)
		}
	}

	return nil
}

// Matches reports, whether any of the matches of the profile is satisfied by
// the BMC data collected for a server.
func (p BIOSProfile) Matches(data api.BMCData) (bool, error) {
	for _, match := range p.Match {
		matches, err := match.Matches(data)
		if err != nil {
			return false, domain.NewValidationErrf("Invalid BIOS profile %q: %v", p.Name, err)
		}

		if matches {
			return true, nil
		}
	}

	return false, nil
}

func (p BIOSProfile) Clone() BIOSProfile {
	clone := p
	clone.Match = slices.Clone(p.Match)
	clone.Attributes = maps.Clone(p.Attributes)
	clone.SecureBoot = p.SecureBoot.Clone()

	return clone
}

// BIOSProfileResolution is the outcome of the resolution of the BIOS profiles
// for a server. It holds the attributes and the secure boot configuration
// accumulated from all the BIOS profiles matching the server.
type BIOSProfileResolution struct {
	// Profiles holds the names of the BIOS profiles, that contributed to the
	// resolution, in the order they have been applied.
	Profiles []string `json:"profiles" yaml:"profiles"`

	// Attributes holds the BIOS attribute names and values to apply.
	Attributes map[string]any `json:"attributes" yaml:"attributes"`

	// SecureBoot holds the secure boot certificates and signatures, that are
	// allowed to stay during the initialization of the server.
	SecureBoot api.BIOSSecureBoot `json:"secure_boot" yaml:"secure_boot"`
}

// ValidateAgainstBIOSAttributes checks the attributes of the resolution against
// the BIOS attributes reported by the BMC of a server and reports all
// mismatches found as a single domain.ErrValidation.
func (r BIOSProfileResolution) ValidateAgainstBIOSAttributes(biosAttributes []api.BIOSAttribute) error {
	knownAttributes := make(map[string]api.BIOSAttribute, len(biosAttributes))
	for _, biosAttribute := range biosAttributes {
		knownAttributes[biosAttribute.Name] = biosAttribute
	}

	problems := []string{}

	for _, name := range slices.Sorted(maps.Keys(r.Attributes)) {
		biosAttribute, ok := knownAttributes[name]
		if !ok {
			problems = append(problems, fmt.Sprintf("%q is not known to the BMC", name))
			continue
		}

		err := validateBIOSAttributeValue(biosAttribute, r.Attributes[name])
		if err != nil {
			problems = append(problems, fmt.Sprintf("%q: %v", name, err))
		}
	}

	if len(problems) == 0 {
		return nil
	}

	return domain.NewValidationErrf("BIOS profiles [%s] are not applicable: %s", strings.Join(r.Profiles, ", "), strings.Join(problems, ", "))
}

// validateBIOSAttributeValue checks a single value against the type, the
// acceptable values and the boundaries the BMC declares for the attribute.
func validateBIOSAttributeValue(biosAttribute api.BIOSAttribute, value any) error {
	stringValue := fmt.Sprint(value)

	if len(biosAttribute.AcceptableValues) > 0 && !slices.Contains(biosAttribute.AcceptableValues, stringValue) {
		return fmt.Errorf("value %q is not one of the acceptable values [%s]", stringValue, strings.Join(biosAttribute.AcceptableValues, ", "))
	}

	if biosAttribute.LowerBound != nil || biosAttribute.UpperBound != nil {
		intValue, err := strconv.ParseInt(stringValue, 10, 64)
		if err != nil {
			return fmt.Errorf("value %q is not an integer", stringValue)
		}

		if biosAttribute.LowerBound != nil && intValue < *biosAttribute.LowerBound {
			return fmt.Errorf("value %d is below the lower bound %d", intValue, *biosAttribute.LowerBound)
		}

		if biosAttribute.UpperBound != nil && intValue > *biosAttribute.UpperBound {
			return fmt.Errorf("value %d is above the upper bound %d", intValue, *biosAttribute.UpperBound)
		}
	}

	if biosAttribute.MinLength != nil && int64(len(stringValue)) < *biosAttribute.MinLength {
		return fmt.Errorf("value %q is shorter than the minimum length %d", stringValue, *biosAttribute.MinLength)
	}

	if biosAttribute.MaxLength != nil && int64(len(stringValue)) > *biosAttribute.MaxLength {
		return fmt.Errorf("value %q is longer than the maximum length %d", stringValue, *biosAttribute.MaxLength)
	}

	return nil
}

type BIOSProfiles []BIOSProfile

// Sort orders the profiles in the order they are applied, by priority ascending
// and finally by name.
func (p BIOSProfiles) Sort() {
	slices.SortStableFunc(p, func(a BIOSProfile, b BIOSProfile) int {
		if a.Priority != b.Priority {
			return a.Priority - b.Priority
		}

		return strings.Compare(a.Name, b.Name)
	})
}

// Resolve accumulates the attributes and the secure boot configuration of all
// the profiles matching the BMC data of a server. The profiles are processed by
// priority ascending, so a profile with a higher priority extends or overwrites
// what the profiles with a lower priority have contributed. A null value drops
// the respective entry from the accumulated set.
//
// Nil is returned, if no profile matches.
func (p BIOSProfiles) Resolve(data api.BMCData) (*BIOSProfileResolution, error) {
	profiles := slices.Clone(p)
	profiles.Sort()

	resolution := BIOSProfileResolution{
		Profiles:   []string{},
		Attributes: map[string]any{},
	}

	for _, profile := range profiles {
		matches, err := profile.Matches(data)
		if err != nil {
			return nil, err
		}

		if !matches {
			continue
		}

		resolution.Profiles = append(resolution.Profiles, profile.Name)

		mergeAttributes(resolution.Attributes, profile.Attributes)

		resolution.SecureBoot.DB = mergeSecureBootDatabase(resolution.SecureBoot.DB, profile.SecureBoot.DB)
		resolution.SecureBoot.DBX = mergeSecureBootDatabase(resolution.SecureBoot.DBX, profile.SecureBoot.DBX)
		resolution.SecureBoot.KEK = mergeSecureBootDatabase(resolution.SecureBoot.KEK, profile.SecureBoot.KEK)
	}

	if len(resolution.Profiles) == 0 {
		return nil, nil
	}

	return &resolution, nil
}

// mergeAttributes folds overlay into base. An attribute with a null value
// removes the respective key from base instead of overwriting it, so the
// attribute is left untouched on the server.
func mergeAttributes(base map[string]any, overlay map[string]any) {
	for key, value := range overlay {
		if value == nil {
			delete(base, key)
			continue
		}

		base[key] = value
	}
}

// mergeFlags folds overlay into base. An entry with a null value removes the
// respective key from base instead of overwriting it.
func mergeFlags(base map[string]bool, overlay map[string]*bool) map[string]bool {
	if len(overlay) == 0 {
		return base
	}

	if base == nil {
		base = map[string]bool{}
	}

	for key, value := range overlay {
		if value == nil {
			delete(base, key)
			continue
		}

		base[key] = *value
	}

	return base
}

func mergeSecureBootDatabase(base api.BIOSSecureBootDatabase, overlay BIOSSecureBootDatabase) api.BIOSSecureBootDatabase {
	base.Certificates = mergeFlags(base.Certificates, overlay.Certificates)
	base.Signatures = mergeFlags(base.Signatures, overlay.Signatures)

	return base
}
