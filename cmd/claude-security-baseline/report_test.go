package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/testing/errassert"
)

func TestParseJSONL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data string

		assertErr require.ErrorAssertionFunc
		want      []finding
	}{
		{
			name: "valid without trailing newline",
			data: "{\"id\": \"F1\", \"claudeSecurityPluginFindingId\": \"v3:a\", \"severity\": \"LOW\"}\n\n{\"id\": \"F2\", \"claudeSecurityPluginFindingId\": \"v3:b\"}",

			assertErr: require.NoError,
			want: []finding{
				{Number: "F1", ID: "v3:a", Severity: "LOW", jsonLine: "{\"id\": \"F1\", \"claudeSecurityPluginFindingId\": \"v3:a\", \"severity\": \"LOW\"}\n"},
				{Number: "F2", ID: "v3:b", jsonLine: "{\"id\": \"F2\", \"claudeSecurityPluginFindingId\": \"v3:b\"}\n"},
			},
		},
		{
			name: "invalid JSON",
			data: "{",

			assertErr: errassert.Contains("JSONL line 1"),
		},
		{
			name: "missing finding ID",
			data: "{\"id\": \"F1\"}\n",

			assertErr: errassert.Contains("record without id or claudeSecurityPluginFindingId"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseJSONL([]byte(tc.data))

			tc.assertErr(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestSuppressSARIF(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data string

		assertErr require.ErrorAssertionFunc
		want      string
	}{
		{
			name: "other content passes through",
			data: `{"n": 1.50, "s": "<&>", "runs": [{"x": 1}, {"results": [{"properties": {"claudeSecurityPlugin": {"claudeSecurityPluginFindingId": "v3:a"}}}, {"y": 2}]}]}`,

			assertErr: require.NoError,
			want: `{
  "n": 1.50,
  "runs": [
    {
      "x": 1
    },
    {
      "results": [
        {
          "properties": {
            "claudeSecurityPlugin": {
              "claudeSecurityPluginFindingId": "v3:a"
            }
          },
          "suppressions": [
            {
              "kind": "external",
              "status": "accepted",
              "justification": "accepted: <Known & fine>"
            }
          ]
        },
        {
          "y": 2
        }
      ]
    }
  ],
  "s": "<&>"
}
`,
		},
		{
			name: "no runs",
			data: `{"version": "2.1.0"}`,

			assertErr: require.NoError,
			want: `{
  "version": "2.1.0"
}
`,
		},
		{
			name: "invalid JSON",
			data: "{",

			assertErr: errassert.Contains("parse SARIF"),
		},
		{
			name: "runs is not a list",
			data: `{"runs": 1}`,

			assertErr: errassert.Contains("parse SARIF runs"),
		},
		{
			name: "results is not a list",
			data: `{"runs": [{"results": 1}]}`,

			assertErr: errassert.Contains("parse SARIF results"),
		},
		{
			name: "properties is not an object",
			data: `{"runs": [{"results": [{"properties": 1}]}]}`,

			assertErr: errassert.Contains("parse SARIF result properties"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := suppressSARIF([]byte(tc.data), map[string]entry{"v3:a": {ID: "v3:a", Status: "accepted", Reason: "<Known & fine>"}})

			tc.assertErr(t, err)
			require.Equal(t, tc.want, string(got))
		})
	}
}
