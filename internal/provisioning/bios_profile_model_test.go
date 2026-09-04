package provisioning_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/util/testing/errassert"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestBIOSProfileMatch_Matches(t *testing.T) {
	tests := []struct {
		name  string
		match provisioning.BIOSProfileMatch
		data  api.BMCData

		assertErr   require.ErrorAssertionFunc
		wantMatches bool
	}{
		{
			name:  "empty match applies to any server",
			match: provisioning.BIOSProfileMatch{},
			data: api.BMCData{
				ServerManufacturer: "Supermicro",
				ServerModel:        "SYS-1029U",
			},

			assertErr:   require.NoError,
			wantMatches: true,
		},
		{
			name: "manufacturer - case insensitive match",
			match: provisioning.BIOSProfileMatch{
				Manufacturer: `dell inc\.`,
			},
			data: api.BMCData{
				ServerManufacturer: "Dell Inc.",
			},

			assertErr:   require.NoError,
			wantMatches: true,
		},
		{
			name: "manufacturer - the complete value has to match",
			match: provisioning.BIOSProfileMatch{
				Manufacturer: "Dell",
			},
			data: api.BMCData{
				ServerManufacturer: "Dell Inc.",
			},

			assertErr:   require.NoError,
			wantMatches: false,
		},
		{
			name: "manufacturer - alternation is anchored as a whole",
			match: provisioning.BIOSProfileMatch{
				Manufacturer: `Dell Inc\.|Dell Computer Corporation`,
			},
			data: api.BMCData{
				ServerManufacturer: "Dell Computer Corporation",
			},

			assertErr:   require.NoError,
			wantMatches: true,
		},
		{
			name: "model - regex match",
			match: provisioning.BIOSProfileMatch{
				Model: "PowerEdge R[0-9]{2}0",
			},
			data: api.BMCData{
				ServerModel: "PowerEdge R770",
			},

			assertErr:   require.NoError,
			wantMatches: true,
		},
		{
			name: "model - regex does not match",
			match: provisioning.BIOSProfileMatch{
				Model: "PowerEdge R[0-9]{2}0",
			},
			data: api.BMCData{
				ServerModel: "PowerEdge R6615",
			},

			assertErr:   require.NoError,
			wantMatches: false,
		},
		{
			name: "model - value is trimmed before it is matched",
			match: provisioning.BIOSProfileMatch{
				Model: "PowerEdge R770",
			},
			data: api.BMCData{
				ServerModel: "  PowerEdge R770 ",
			},

			assertErr:   require.NoError,
			wantMatches: true,
		},
		{
			name: "model - invalid regex pattern",
			match: provisioning.BIOSProfileMatch{
				Model: "PowerEdge [R70",
			},
			data: api.BMCData{
				ServerModel: "PowerEdge R770",
			},

			assertErr:   errassert.ValidationErrorContains(`Invalid model pattern "PowerEdge [R70"`),
			wantMatches: false,
		},
		{
			name: "processor manufacturer - matches",
			match: provisioning.BIOSProfileMatch{
				ProcessorManufacturer: "AMD",
			},
			data: api.BMCData{
				ServerProcessorManufacturer: "AMD",
			},

			assertErr:   require.NoError,
			wantMatches: true,
		},
		{
			name: "processor manufacturer - does not match",
			match: provisioning.BIOSProfileMatch{
				ProcessorManufacturer: "AMD",
			},
			data: api.BMCData{
				ServerProcessorManufacturer: "Intel",
			},

			assertErr:   require.NoError,
			wantMatches: false,
		},
		{
			name: "processor architecture and instruction set - match",
			match: provisioning.BIOSProfileMatch{
				ProcessorArchitecture:   "x86",
				ProcessorInstructionSet: "x86-64",
			},
			data: api.BMCData{
				ServerProcessorArchitecture:   "x86",
				ServerProcessorInstructionSet: "x86-64",
			},

			assertErr:   require.NoError,
			wantMatches: true,
		},
		{
			name: "processor instruction set - does not match",
			match: provisioning.BIOSProfileMatch{
				ProcessorArchitecture:   "x86",
				ProcessorInstructionSet: "x86-64",
			},
			data: api.BMCData{
				ServerProcessorArchitecture:   "x86",
				ServerProcessorInstructionSet: "x86",
			},

			assertErr:   require.NoError,
			wantMatches: false,
		},
		{
			name: "cpu sockets - matches",
			match: provisioning.BIOSProfileMatch{
				CPUSockets: new(2),
			},
			data: api.BMCData{
				ServerCPUSockets: 2,
			},

			assertErr:   require.NoError,
			wantMatches: true,
		},
		{
			name: "cpu sockets - does not match",
			match: provisioning.BIOSProfileMatch{
				CPUSockets: new(2),
			},
			data: api.BMCData{
				ServerCPUSockets: 1,
			},

			assertErr:   require.NoError,
			wantMatches: false,
		},
		{
			name: "cpu sockets - unknown to the BMC does not match",
			match: provisioning.BIOSProfileMatch{
				CPUSockets: new(2),
			},
			data: api.BMCData{},

			assertErr:   require.NoError,
			wantMatches: false,
		},
		{
			name: "has tpm - matches",
			match: provisioning.BIOSProfileMatch{
				HasTPM: new(true),
			},
			data: api.BMCData{
				ServerHasTPM: true,
			},

			assertErr:   require.NoError,
			wantMatches: true,
		},
		{
			name: "has tpm - false matches a server without TPM",
			match: provisioning.BIOSProfileMatch{
				HasTPM: new(false),
			},
			data: api.BMCData{
				ServerHasTPM: false,
			},

			assertErr:   require.NoError,
			wantMatches: true,
		},
		{
			name: "has tpm - does not match",
			match: provisioning.BIOSProfileMatch{
				HasTPM: new(true),
			},
			data: api.BMCData{
				ServerHasTPM: false,
			},

			assertErr:   require.NoError,
			wantMatches: false,
		},
		{
			name: "bios version - constraint satisfied",
			match: provisioning.BIOSProfileMatch{
				BIOSVersion: ">= 1.7.0",
			},
			data: api.BMCData{
				ServerBIOSVersion: "1.7.5",
			},

			assertErr:   require.NoError,
			wantMatches: true,
		},
		{
			name: "bios version - constraint not satisfied",
			match: provisioning.BIOSProfileMatch{
				BIOSVersion: ">= 1.7.0",
			},
			data: api.BMCData{
				ServerBIOSVersion: "1.6.9",
			},

			assertErr:   require.NoError,
			wantMatches: false,
		},
		{
			name: "bios version - version not interpretable",
			match: provisioning.BIOSProfileMatch{
				BIOSVersion: ">= 1.7.0",
			},
			data: api.BMCData{
				ServerBIOSVersion: "L2.05",
			},

			assertErr:   require.NoError,
			wantMatches: false,
		},
		{
			name: "bios version - invalid constraint",
			match: provisioning.BIOSProfileMatch{
				BIOSVersion: "not a constraint",
			},
			data: api.BMCData{
				ServerBIOSVersion: "1.7.5",
			},

			assertErr:   errassert.ValidationError,
			wantMatches: false,
		},
		{
			name: "all criteria combined",
			match: provisioning.BIOSProfileMatch{
				Manufacturer:            `Dell Inc\.`,
				Model:                   "PowerEdge R[0-9]{2}0",
				ProcessorManufacturer:   "AMD",
				ProcessorArchitecture:   "x86",
				ProcessorInstructionSet: "x86-64",
				CPUSockets:              new(2),
				HasTPM:                  new(true),
				BIOSVersion:             ">= 1.7.0",
			},
			data: api.BMCData{
				ServerManufacturer:            "Dell Inc.",
				ServerModel:                   "PowerEdge R760",
				ServerProcessorManufacturer:   "AMD",
				ServerProcessorArchitecture:   "x86",
				ServerProcessorInstructionSet: "x86-64",
				ServerCPUSockets:              2,
				ServerHasTPM:                  true,
				ServerBIOSVersion:             "2.0.1",
			},

			assertErr:   require.NoError,
			wantMatches: true,
		},
		{
			name: "all criteria combined - manufacturer differs",
			match: provisioning.BIOSProfileMatch{
				Manufacturer:            `Dell Inc\.`,
				Model:                   "PowerEdge R[0-9]{2}0",
				ProcessorManufacturer:   "AMD",
				ProcessorArchitecture:   "x86",
				ProcessorInstructionSet: "x86-64",
				CPUSockets:              new(2),
				HasTPM:                  new(true),
				BIOSVersion:             ">= 1.7.0",
			},
			data: api.BMCData{
				ServerManufacturer:            "Supermicro",
				ServerModel:                   "PowerEdge R760",
				ServerProcessorManufacturer:   "AMD",
				ServerProcessorArchitecture:   "x86",
				ServerProcessorInstructionSet: "x86-64",
				ServerCPUSockets:              2,
				ServerHasTPM:                  true,
				ServerBIOSVersion:             "2.0.1",
			},

			assertErr:   require.NoError,
			wantMatches: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			matches, err := tc.match.Matches(tc.data)

			tc.assertErr(t, err)
			require.Equal(t, tc.wantMatches, matches)
		})
	}
}

func TestBIOSProfile_Matches(t *testing.T) {
	tests := []struct {
		name    string
		matches []provisioning.BIOSProfileMatch
		data    api.BMCData

		assertErr   require.ErrorAssertionFunc
		wantMatches bool
	}{
		{
			name:    "no match definition never applies",
			matches: nil,
			data: api.BMCData{
				ServerManufacturer: "Dell Inc.",
			},

			assertErr:   require.NoError,
			wantMatches: false,
		},
		{
			name: "the first of multiple match definitions applies",
			matches: []provisioning.BIOSProfileMatch{
				{Model: "PowerEdge R7[0-9]0"},
				{Model: "PowerEdge R6[0-9]0"},
			},
			data: api.BMCData{
				ServerModel: "PowerEdge R770",
			},

			assertErr:   require.NoError,
			wantMatches: true,
		},
		{
			name: "the last of multiple match definitions applies",
			matches: []provisioning.BIOSProfileMatch{
				{Model: "PowerEdge R7[0-9]0"},
				{Model: "PowerEdge R6[0-9]0"},
			},
			data: api.BMCData{
				ServerModel: "PowerEdge R660",
			},

			assertErr:   require.NoError,
			wantMatches: true,
		},
		{
			name: "none of the match definitions applies",
			matches: []provisioning.BIOSProfileMatch{
				{Model: "PowerEdge R7[0-9]0"},
				{Model: "PowerEdge R6[0-9]0"},
			},
			data: api.BMCData{
				ServerModel: "PowerEdge R6615",
			},

			assertErr:   require.NoError,
			wantMatches: false,
		},
		{
			name: "error - invalid match definition",
			matches: []provisioning.BIOSProfileMatch{
				{Model: "PowerEdge R7[0-9]0"},
				{Model: "PowerEdge [R6"}, // invalid
			},
			data: api.BMCData{
				ServerModel: "PowerEdge R6615",
			},

			assertErr:   errassert.ValidationErrorContains(`Invalid BIOS profile "dell-poweredge"`),
			wantMatches: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			profile := provisioning.BIOSProfile{
				Name:  "dell-poweredge",
				Match: tc.matches,
			}

			matches, err := profile.Matches(tc.data)

			tc.assertErr(t, err)
			require.Equal(t, tc.wantMatches, matches)
		})
	}
}

func TestBIOSProfile_Validate(t *testing.T) {
	tests := []struct {
		name    string
		profile provisioning.BIOSProfile

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "valid",
			profile: provisioning.BIOSProfile{
				Name: "dell-poweredge-r7x0",
				Match: []provisioning.BIOSProfileMatch{
					{
						Manufacturer: `Dell Inc\.`,
						Model:        "PowerEdge R[0-9]{2}0",
						BIOSVersion:  ">= 1.7.0",
					},
				},
				Attributes: map[string]any{
					"SecureBoot":       "Enabled",
					"SecureBootPolicy": "Custom",
				},
			},

			assertErr: require.NoError,
		},
		{
			name: "valid - deferred attributes only",
			profile: provisioning.BIOSProfile{
				Name:  "dell-poweredge-tpm-algorithm",
				Match: []provisioning.BIOSProfileMatch{{Manufacturer: `Dell Inc\.`}},
				DeferredAttributes: map[string]any{
					"Tpm2Algorithm": "SHA256",
				},
			},

			assertErr: require.NoError,
		},
		{
			name: "valid - secure boot only",
			profile: provisioning.BIOSProfile{
				Name:  "dell-poweredge-secure-boot",
				Match: []provisioning.BIOSProfileMatch{{Manufacturer: `Dell Inc\.`}},
				SecureBoot: provisioning.BIOSSecureBoot{
					DB: provisioning.BIOSSecureBootDatabase{
						Certificates: map[string]*bool{"abcd": new(true)},
					},
				},
			},

			assertErr: require.NoError,
		},
		{
			name: "error - name empty",
			profile: provisioning.BIOSProfile{
				Name:  "", // invalid
				Match: []provisioning.BIOSProfileMatch{{}},
				Attributes: map[string]any{
					"SecureBoot": "Enabled",
				},
			},

			assertErr: errassert.ValidationError,
		},
		{
			name: "error - name contains prohibited characters",
			profile: provisioning.BIOSProfile{
				Name:  "dell/poweredge", // invalid
				Match: []provisioning.BIOSProfileMatch{{}},
				Attributes: map[string]any{
					"SecureBoot": "Enabled",
				},
			},

			assertErr: errassert.ValidationError,
		},
		{
			name: "error - match empty",
			profile: provisioning.BIOSProfile{
				Name:  "dell-poweredge",
				Match: nil, // invalid
				Attributes: map[string]any{
					"SecureBoot": "Enabled",
				},
			},

			assertErr: errassert.ValidationErrorContains("at least one match is required"),
		},
		{
			name: "error - attributes, deferred attributes and secure boot empty",
			profile: provisioning.BIOSProfile{
				Name:               "dell-poweredge",
				Match:              []provisioning.BIOSProfileMatch{{}},
				Attributes:         nil, // invalid
				DeferredAttributes: nil, // invalid
			},

			assertErr: errassert.ValidationError,
		},
		{
			name: "error - invalid model pattern",
			profile: provisioning.BIOSProfile{
				Name: "dell-poweredge",
				Match: []provisioning.BIOSProfileMatch{
					{
						Model: "PowerEdge [R70", // invalid
					},
				},
				Attributes: map[string]any{
					"SecureBoot": "Enabled",
				},
			},

			assertErr: errassert.ValidationError,
		},
		{
			name: "error - invalid cpu sockets",
			profile: provisioning.BIOSProfile{
				Name: "dell-poweredge",
				Match: []provisioning.BIOSProfileMatch{
					{
						CPUSockets: new(0), // invalid
					},
				},
				Attributes: map[string]any{
					"SecureBoot": "Enabled",
				},
			},

			assertErr: errassert.ValidationError,
		},
		{
			name: "error - invalid bios version constraint",
			profile: provisioning.BIOSProfile{
				Name: "dell-poweredge",
				Match: []provisioning.BIOSProfileMatch{
					{
						BIOSVersion: "not a constraint", // invalid
					},
				},
				Attributes: map[string]any{
					"SecureBoot": "Enabled",
				},
			},

			assertErr: errassert.ValidationError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.profile.Validate()

			tc.assertErr(t, err)
		})
	}
}

func TestBIOSProfiles_Resolve(t *testing.T) {
	profiles := provisioning.BIOSProfiles{
		{
			Name:     "vendor",
			Priority: 100,
			Match:    []provisioning.BIOSProfileMatch{{Manufacturer: `Dell Inc\.`}},
			Attributes: map[string]any{
				"SecureBoot":         "Enabled",
				"SecureBootMode":     "UserMode",
				"NumaNodesPerSocket": "1",
			},
			DeferredAttributes: map[string]any{
				"Tpm2Algorithm": "SHA1",
				"Tpm2Hierarchy": "Enabled",
			},
			SecureBoot: provisioning.BIOSSecureBoot{
				DB: provisioning.BIOSSecureBootDatabase{
					Certificates: map[string]*bool{"aaaa": new(true), "bbbb": new(true)},
					Signatures:   map[string]*bool{"1111": new(true)},
				},
				KEK: provisioning.BIOSSecureBootDatabase{
					Certificates: map[string]*bool{"cccc": new(true)},
				},
			},
		},
		{
			// Same priority as "model", but sorts before it by name.
			Name:     "aaa-same-priority",
			Priority: 200,
			Match:    []provisioning.BIOSProfileMatch{{Manufacturer: `Dell Inc\.`}},
			Attributes: map[string]any{
				"SecureBootPolicy": "Standard",
			},
		},
		{
			Name:     "model",
			Priority: 200,
			Match:    []provisioning.BIOSProfileMatch{{Model: "PowerEdge R[0-9]{2}0"}},
			Attributes: map[string]any{
				// Overwrites the value of the vendor profile.
				"SecureBootPolicy": "Custom",
				// Removes the attribute contributed by the vendor profile.
				"NumaNodesPerSocket": nil,
				// Extends the set of attributes.
				"TpmSecurity": "On",
			},
			DeferredAttributes: map[string]any{
				// Overwrites and removes, just like the attributes above.
				"Tpm2Algorithm": "SHA256",
				"Tpm2Hierarchy": nil,
			},
			SecureBoot: provisioning.BIOSSecureBoot{
				DB: provisioning.BIOSSecureBootDatabase{
					// Overwrites, removes and extends.
					Certificates: map[string]*bool{"aaaa": new(false), "bbbb": nil, "dddd": new(true)},
					Signatures:   map[string]*bool{"2222": new(true)},
				},
			},
		},
		{
			Name:     "other-vendor",
			Priority: 300,
			Match:    []provisioning.BIOSProfileMatch{{Manufacturer: "Supermicro"}},
			Attributes: map[string]any{
				"SecureBoot": "Disabled",
			},
		},
		{
			Name:     "invalid",
			Priority: 400,
			Match:    []provisioning.BIOSProfileMatch{{Model: "PowerEdge [R7"}},
			Attributes: map[string]any{
				"SecureBoot": "Disabled",
			},
		},
	}

	tests := []struct {
		name     string
		profiles provisioning.BIOSProfiles
		data     api.BMCData

		assertErr require.ErrorAssertionFunc
		want      *provisioning.BIOSProfileResolution
	}{
		{
			name:     "no profile matches",
			profiles: profiles[:len(profiles)-1],
			data: api.BMCData{
				ServerManufacturer: "HPE",
				ServerModel:        "ProLiant DL380",
			},

			assertErr: require.NoError,
			want:      nil,
		},
		{
			name:     "a single profile matches",
			profiles: profiles[:len(profiles)-1],
			data: api.BMCData{
				ServerManufacturer: "Supermicro",
				ServerModel:        "SYS-1029U",
			},

			assertErr: require.NoError,
			want: &provisioning.BIOSProfileResolution{
				Profiles: []string{"other-vendor"},
				Attributes: map[string]any{
					"SecureBoot": "Disabled",
				},
				DeferredAttributes: map[string]any{},
			},
		},
		{
			name:     "the matching profiles are merged by priority ascending",
			profiles: profiles[:len(profiles)-1],
			data: api.BMCData{
				ServerManufacturer: "Dell Inc.",
				ServerModel:        "PowerEdge R770",
			},

			assertErr: require.NoError,
			want: &provisioning.BIOSProfileResolution{
				Profiles: []string{"vendor", "aaa-same-priority", "model"},
				Attributes: map[string]any{
					"SecureBoot":       "Enabled",
					"SecureBootMode":   "UserMode",
					"SecureBootPolicy": "Custom",
					"TpmSecurity":      "On",
				},
				DeferredAttributes: map[string]any{
					"Tpm2Algorithm": "SHA256",
				},
				SecureBoot: api.BIOSSecureBoot{
					DB: api.BIOSSecureBootDatabase{
						Certificates: map[string]bool{"aaaa": false, "dddd": true},
						Signatures:   map[string]bool{"1111": true, "2222": true},
					},
					KEK: api.BIOSSecureBootDatabase{
						Certificates: map[string]bool{"cccc": true},
					},
				},
			},
		},
		{
			name:     "error - invalid profile",
			profiles: profiles,
			data: api.BMCData{
				ServerManufacturer: "Dell Inc.",
				ServerModel:        "PowerEdge R770",
			},

			assertErr: errassert.ValidationError,
			want:      nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resolution, err := tc.profiles.Resolve(tc.data)

			tc.assertErr(t, err)
			require.Equal(t, tc.want, resolution)
		})
	}
}

func TestBIOSProfiles_ResolveDoesNotMutateTheProfiles(t *testing.T) {
	profiles := provisioning.BIOSProfiles{
		{
			Name:     "high",
			Priority: 200,
			Match:    []provisioning.BIOSProfileMatch{{}},
			Attributes: map[string]any{
				"SecureBoot": "Enabled",
			},
		},
		{
			Name:     "low",
			Priority: 100,
			Match:    []provisioning.BIOSProfileMatch{{}},
			Attributes: map[string]any{
				"SecureBoot": "Disabled",
			},
		},
	}

	resolution, err := profiles.Resolve(api.BMCData{})
	require.NoError(t, err)
	require.Equal(t, []string{"low", "high"}, resolution.Profiles)

	// The order of the profiles given to Resolve is left untouched, only the
	// resolution is ordered by priority.
	require.Equal(t, "high", profiles[0].Name)
	require.Equal(t, "low", profiles[1].Name)

	// The attributes of the profiles are not affected by the merge.
	require.Equal(t, map[string]any{"SecureBoot": "Enabled"}, profiles[0].Attributes)
	require.Equal(t, map[string]any{"SecureBoot": "Disabled"}, profiles[1].Attributes)
}

func TestBIOSProfileResolution_ValidateAgainstBIOSAttributes(t *testing.T) {
	tests := []struct {
		name               string
		attributes         map[string]any
		deferredAttributes map[string]any
		biosAttributes     []api.BIOSAttribute

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "valid",
			attributes: map[string]any{
				"SecureBoot":         "Enabled",
				"NumaNodesPerSocket": "4",
				"SystemName":         "abcd",
			},
			biosAttributes: []api.BIOSAttribute{
				{
					Name:             "SecureBoot",
					Type:             "Enumeration",
					AcceptableValues: []string{"Enabled", "Disabled"},
				},
				{
					Name:       "NumaNodesPerSocket",
					Type:       "Integer",
					LowerBound: new(int64(0)),
					UpperBound: new(int64(8)),
				},
				{
					Name:      "SystemName",
					Type:      "String",
					MinLength: new(int64(1)),
					MaxLength: new(int64(8)),
				},
			},

			assertErr: require.NoError,
		},
		{
			// The attribute registry only publishes a deferred attribute once the
			// attributes, it depends on, are in effect, which is the very reason it
			// is applied in a second pass.
			name: "valid - deferred attribute unknown to the BMC",
			deferredAttributes: map[string]any{
				"Tpm2Algorithm": "SHA256",
			},
			biosAttributes: []api.BIOSAttribute{
				{
					Name: "TpmSecurity",
				},
			},

			assertErr: require.NoError,
		},
		{
			name: "error - deferred attribute value not acceptable",
			deferredAttributes: map[string]any{
				"Tpm2Algorithm": "SHA42",
			},
			biosAttributes: []api.BIOSAttribute{
				{
					Name:             "Tpm2Algorithm",
					Type:             "Enumeration",
					AcceptableValues: []string{"SHA1", "SHA256"},
				},
			},

			assertErr: errassert.ValidationErrorContains("SHA1, SHA256"),
		},
		{
			name: "error - attribute unknown to the BMC",
			attributes: map[string]any{
				"SecureBootPolicy": "Custom",
			},
			biosAttributes: []api.BIOSAttribute{
				{
					Name: "SecureBoot",
				},
			},

			assertErr: errassert.ValidationErrorContains(`"SecureBootPolicy" is not known to the BMC`),
		},
		{
			name: "error - value not acceptable",
			attributes: map[string]any{
				"SecureBoot": "Yes",
			},
			biosAttributes: []api.BIOSAttribute{
				{
					Name:             "SecureBoot",
					Type:             "Enumeration",
					AcceptableValues: []string{"Enabled", "Disabled"},
				},
			},

			assertErr: errassert.ValidationErrorContains("Enabled, Disabled"),
		},
		{
			name: "error - value out of bounds",
			attributes: map[string]any{
				"NumaNodesPerSocket": 16,
			},
			biosAttributes: []api.BIOSAttribute{
				{
					Name:       "NumaNodesPerSocket",
					Type:       "Integer",
					LowerBound: new(int64(0)),
					UpperBound: new(int64(8)),
				},
			},

			assertErr: errassert.ValidationErrorContains("above the upper bound 8"),
		},
		{
			name: "error - value too long",
			attributes: map[string]any{
				"SystemName": "way too long to be accepted",
			},
			biosAttributes: []api.BIOSAttribute{
				{
					Name:      "SystemName",
					Type:      "String",
					MinLength: new(int64(1)),
					MaxLength: new(int64(8)),
				},
			},

			assertErr: errassert.ValidationErrorContains("longer than the maximum length 8"),
		},
		{
			name: "error - all mismatches are reported",
			attributes: map[string]any{
				"SecureBoot":  "Yes",
				"TpmSecurity": "On",
			},
			biosAttributes: []api.BIOSAttribute{
				{
					Name:             "SecureBoot",
					Type:             "Enumeration",
					AcceptableValues: []string{"Enabled", "Disabled"},
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				errassert.ValidationError(tt, err, a...)
				require.ErrorContains(tt, err, `BIOS profiles [dell-poweredge]`)
				require.ErrorContains(tt, err, `"SecureBoot"`)
				require.ErrorContains(tt, err, `"TpmSecurity" is not known to the BMC`)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resolution := provisioning.BIOSProfileResolution{
				Profiles:           []string{"dell-poweredge"},
				Attributes:         tc.attributes,
				DeferredAttributes: tc.deferredAttributes,
			}

			err := resolution.ValidateAgainstBIOSAttributes(tc.biosAttributes)

			tc.assertErr(t, err)
		})
	}
}

func TestBIOSProfiles_Sort(t *testing.T) {
	profiles := provisioning.BIOSProfiles{
		{Name: "high-b", Priority: 100},
		{Name: "low", Priority: 10},
		{Name: "high-a", Priority: 100},
	}

	profiles.Sort()

	names := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		names = append(names, profile.Name)
	}

	require.Equal(t, []string{"low", "high-a", "high-b"}, names)
}
