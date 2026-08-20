package decodestrict_test

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/decodestrict"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestJSON(t *testing.T) {
	tests := []struct {
		name string
		body string

		assertErr          require.ErrorAssertionFunc
		wantErrContain     string
		wantInterfaceCount int
		wantNetworkVersion string
	}{
		{
			name: "success - flat network seed",
			body: `{
  "type": "iso",
  "architecture": "x86_64",
  "seeds": {
    "network": {
      "version": "1",
      "interfaces": [
        {
          "name": "enp5s0",
          "addresses": ["dhcp4", "slaac"]
        }
      ]
    }
  }
}`,

			assertErr:          require.NoError,
			wantInterfaceCount: 1,
			wantNetworkVersion: "1",
		},
		{
			name: "success - empty network seed",
			body: `{"type": "iso", "architecture": "x86_64", "seeds": {"network": {}}}`,

			assertErr: require.NoError,
		},
		{
			name: "error - incus seed is not accepted",
			body: `{
  "seeds": {
    "incus": {
      "version": "1"
    }
  }
}`,

			assertErr:      require.Error,
			wantErrContain: `unknown field "incus"`,
		},
		{
			name: "error - update seed is not accepted",
			body: `{
  "seeds": {
    "update": {
      "version": "1"
    }
  }
}`,

			assertErr:      require.Error,
			wantErrContain: `unknown field "update"`,
		},
		{
			name: "error - network seed nested in config block",
			body: `{
  "seeds": {
    "network": {
      "config": {
        "interfaces": [
          {
            "name": "enp5s0"
          }
        ]
      }
    }
  }
}`,

			assertErr:      require.Error,
			wantErrContain: `unknown field "config"`,
		},
		{
			name: "error - unknown top level field",
			body: `{"type": "iso", "unknown": "value"}`,

			assertErr:      require.Error,
			wantErrContain: `unknown field "unknown"`,
		},
		{
			name: "error - empty body",
			body: ``,

			assertErr:      require.Error,
			wantErrContain: io.EOF.Error(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var tokenImagePost api.TokenImagePost

			err := decodestrict.JSON(strings.NewReader(tc.body), &tokenImagePost)

			tc.assertErr(t, err)
			if tc.wantErrContain != "" {
				require.ErrorContains(t, err, tc.wantErrContain)
				return
			}

			require.Equal(t, tc.wantNetworkVersion, tokenImagePost.Seeds.Network.Version)
			require.Len(t, tokenImagePost.Seeds.Network.Interfaces, tc.wantInterfaceCount)
		})
	}
}

func TestYAML_tokenSeedConfigs(t *testing.T) {
	tests := []struct {
		name string
		body string

		assertErr          require.ErrorAssertionFunc
		wantErrContain     string
		wantInterfaceCount int
	}{
		{
			name: "success - seeds document as used by the CLI",
			body: `---
applications:
  version: "1"
  applications:
    - name: incus
network:
  version: "1"
  interfaces:
    - name: enp5s0
      hwaddr: enp5s0
      required_for_online: both
      addresses:
        - dhcp4
        - slaac
`,

			assertErr:          require.NoError,
			wantInterfaceCount: 1,
		},
		{
			name: "error - incus seed is not accepted",
			body: `---
incus:
  version: "1"
`,

			assertErr:      require.Error,
			wantErrContain: "field incus not found in type",
		},
		{
			name: "error - update seed is not accepted",
			body: `---
update:
  version: "1"
`,

			assertErr:      require.Error,
			wantErrContain: "field update not found in type",
		},
		{
			name: "success - empty document",
			body: ``,

			assertErr: require.NoError,
		},
		{
			name: "error - network seed nested in config block",
			body: `---
network:
  config:
    interfaces:
      - name: enp5s0
`,

			assertErr:      require.Error,
			wantErrContain: "field config not found in type",
		},
		{
			name: "error - unknown top level field",
			body: `---
unknown: value
`,

			assertErr:      require.Error,
			wantErrContain: "field unknown not found in type",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var seeds api.TokenSeedConfigs

			err := decodestrict.YAML([]byte(tc.body), &seeds)

			tc.assertErr(t, err)
			if tc.wantErrContain != "" {
				require.ErrorContains(t, err, tc.wantErrContain)
				return
			}

			require.Len(t, seeds.Network.Interfaces, tc.wantInterfaceCount)
		})
	}
}

func TestYAML_tokenSeedPut(t *testing.T) {
	tests := []struct {
		name string
		body string

		assertErr          require.ErrorAssertionFunc
		wantErrContain     string
		wantDescription    string
		wantInterfaceCount int
	}{
		{
			name: "success",
			body: `---
description: some description
public: false
seeds:
  network:
    version: "1"
    interfaces:
      - name: enp5s0
`,

			assertErr:          require.NoError,
			wantDescription:    "some description",
			wantInterfaceCount: 1,
		},
		{
			name: "error - seeds document provided instead of token seed put",
			body: `---
applications:
  version: "1"
  applications:
    - name: incus
network:
  version: "1"
`,

			assertErr:      require.Error,
			wantErrContain: "field applications not found in type",
		},
		{
			name: "error - incus seed is not accepted",
			body: `---
description: some description
public: false
seeds:
  incus:
    version: "1"
`,

			assertErr:      require.Error,
			wantErrContain: "field incus not found in type",
		},
		{
			name: "error - update seed is not accepted",
			body: `---
description: some description
public: false
seeds:
  update:
    version: "1"
`,

			assertErr:      require.Error,
			wantErrContain: "field update not found in type",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var tokenSeedPut api.TokenSeedPut

			err := decodestrict.YAML([]byte(tc.body), &tokenSeedPut)

			tc.assertErr(t, err)
			if tc.wantErrContain != "" {
				require.ErrorContains(t, err, tc.wantErrContain)
				return
			}

			require.Equal(t, tc.wantDescription, tokenSeedPut.Description)
			require.Len(t, tokenSeedPut.Seeds.Network.Interfaces, tc.wantInterfaceCount)
		})
	}
}
