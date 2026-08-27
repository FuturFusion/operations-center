package bios_test

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/adapter/bios"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestNewFromFS(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string

		assertErr    require.ErrorAssertionFunc
		wantProfiles []string
	}{
		{
			name: "profiles from multiple files, sorted by priority ascending",
			files: map[string]string{
				"profiles/a.yaml": `
- name: low
  match:
    - manufacturer: ACME
  priority: 1
  attributes:
    SecureBoot: Enabled
`,
				"profiles/b.yaml": `
- name: high
  match:
    - manufacturer: ACME
  priority: 100
  attributes:
    SecureBoot: Enabled
`,
			},

			assertErr:    require.NoError,
			wantProfiles: []string{"low", "high"},
		},
		{
			name:  "no profiles at all",
			files: map[string]string{},

			assertErr:    require.NoError,
			wantProfiles: []string{},
		},
		{
			name: "error - duplicate name",
			files: map[string]string{
				"profiles/a.yaml": `
- name: duplicate
  match:
    - manufacturer: ACME
  attributes:
    SecureBoot: Enabled
`,
				"profiles/b.yaml": `
- name: duplicate
  match:
    - manufacturer: ACME
  attributes:
    SecureBoot: Enabled
`,
			},

			assertErr: require.Error,
		},
		{
			name: "error - profile without attributes",
			files: map[string]string{
				"profiles/a.yaml": `
- name: without-attributes
  match:
    - manufacturer: ACME
`,
			},

			assertErr: require.Error,
		},
		{
			name: "error - profile without match",
			files: map[string]string{
				"profiles/a.yaml": `
- name: without-match
  attributes:
    SecureBoot: Enabled
`,
			},

			assertErr: require.Error,
		},
		{
			name: "error - invalid model pattern",
			files: map[string]string{
				"profiles/a.yaml": `
- name: invalid-model
  match:
    - model: "Model [9"
  attributes:
    SecureBoot: Enabled
`,
			},

			assertErr: require.Error,
		},
		{
			name: "error - malformed YAML",
			files: map[string]string{
				"profiles/a.yaml": `not: a: list`,
			},

			assertErr: require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fsys := fstest.MapFS{}
			for name, content := range tc.files {
				fsys[name] = &fstest.MapFile{Data: []byte(content)}
			}

			catalogue, err := bios.NewFromFS(fsys, "profiles")

			tc.assertErr(t, err)
			if err != nil {
				return
			}

			profiles, err := catalogue.GetAll(context.Background())
			require.NoError(t, err)

			names := make([]string, 0, len(profiles))
			for _, profile := range profiles {
				names = append(names, profile.Name)
			}

			require.Equal(t, tc.wantProfiles, names)
		})
	}
}

func TestCatalogue_Resolve(t *testing.T) {
	const profilesYAML = `
- name: any-acme
  match:
    - manufacturer: ACME
  priority: 1
  attributes:
    SecureBoot: Enabled
    NumaNodesPerSocket: "1"
  secure_boot:
    db:
      certificates:
        aaaa: true
        bbbb: true

- name: acme-9000
  match:
    - manufacturer: ACME
      model: "Model 9[0-9]00"
    # Also covers the models, that got renamed with the second generation.
    - manufacturer: ACME
      model: "Model 9[0-9]00 G2"
  priority: 100
  attributes:
    SecureBootPolicy: Custom
    # Left at whatever the server has configured.
    NumaNodesPerSocket: null
  secure_boot:
    db:
      certificates:
        bbbb: null
        cccc: false

- name: acme-9000-new-bios
  match:
    - manufacturer: ACME
      model: "Model 9[0-9]00"
      bios_version: ">= 2.0.0"
  priority: 200
  attributes:
    SecureBootPolicy: Standard
    TpmSecurity: "On"
`

	tests := []struct {
		name string
		data api.BMCData

		want *provisioning.BIOSProfileResolution
	}{
		{
			name: "all the matching profiles are merged",
			data: api.BMCData{
				ServerManufacturer: "ACME",
				ServerModel:        "Model 9100",
				ServerBIOSVersion:  "2.1.0",
			},

			want: &provisioning.BIOSProfileResolution{
				Profiles: []string{"any-acme", "acme-9000", "acme-9000-new-bios"},
				Attributes: map[string]any{
					"SecureBoot":       "Enabled",
					"SecureBootPolicy": "Standard",
					"TpmSecurity":      "On",
				},
				SecureBoot: api.BIOSSecureBoot{
					DB: api.BIOSSecureBootDatabase{
						Certificates: map[string]bool{"aaaa": true, "cccc": false},
					},
				},
			},
		},
		{
			name: "version constraint excludes the highest priority profile",
			data: api.BMCData{
				ServerManufacturer: "ACME",
				ServerModel:        "Model 9100",
				ServerBIOSVersion:  "1.9.0",
			},

			want: &provisioning.BIOSProfileResolution{
				Profiles: []string{"any-acme", "acme-9000"},
				Attributes: map[string]any{
					"SecureBoot":       "Enabled",
					"SecureBootPolicy": "Custom",
				},
				SecureBoot: api.BIOSSecureBoot{
					DB: api.BIOSSecureBootDatabase{
						Certificates: map[string]bool{"aaaa": true, "cccc": false},
					},
				},
			},
		},
		{
			name: "the second match definition of a profile applies as well",
			data: api.BMCData{
				ServerManufacturer: "ACME",
				ServerModel:        "Model 9100 G2",
				ServerBIOSVersion:  "1.9.0",
			},

			want: &provisioning.BIOSProfileResolution{
				Profiles: []string{"any-acme", "acme-9000"},
				Attributes: map[string]any{
					"SecureBoot":       "Enabled",
					"SecureBootPolicy": "Custom",
				},
				SecureBoot: api.BIOSSecureBoot{
					DB: api.BIOSSecureBootDatabase{
						Certificates: map[string]bool{"aaaa": true, "cccc": false},
					},
				},
			},
		},
		{
			name: "falls back to the catch all profile",
			data: api.BMCData{
				ServerManufacturer: "ACME",
				ServerModel:        "Model 1000",
			},

			want: &provisioning.BIOSProfileResolution{
				Profiles: []string{"any-acme"},
				Attributes: map[string]any{
					"SecureBoot":         "Enabled",
					"NumaNodesPerSocket": "1",
				},
				SecureBoot: api.BIOSSecureBoot{
					DB: api.BIOSSecureBootDatabase{
						Certificates: map[string]bool{"aaaa": true, "bbbb": true},
					},
				},
			},
		},
		{
			name: "no match",
			data: api.BMCData{
				ServerManufacturer: "Other",
				ServerModel:        "Model 9100",
			},

			want: nil,
		},
	}

	catalogue, err := bios.NewFromFS(fstest.MapFS{
		"profiles/test.yaml": &fstest.MapFile{Data: []byte(profilesYAML)},
	}, "profiles")
	require.NoError(t, err)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resolution, err := catalogue.Resolve(context.Background(), provisioning.Server{
				Name:    "one",
				BMCData: tc.data,
			})
			require.NoError(t, err)
			require.Equal(t, tc.want, resolution)
		})
	}
}
