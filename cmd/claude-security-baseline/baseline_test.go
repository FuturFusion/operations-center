package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/testing/errassert"
)

func TestParseBaseline(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data string

		assertErr require.ErrorAssertionFunc
		want      []entry
	}{
		{
			name: "valid",
			data: `
findings:
  - id: v3:aaa
    status: accepted
    reason: Known.
    file: a.go
    cwe: CWE-1
  - id: v3:bbb
    status: false-positive
    reason: Not reachable.
`,

			assertErr: require.NoError,
			want: []entry{
				{ID: "v3:aaa", Status: "accepted", Reason: "Known.", File: "a.go", CWE: "CWE-1"},
				{ID: "v3:bbb", Status: "false-positive", Reason: "Not reachable."},
			},
		},
		{
			name: "only comments",
			data: "# Nothing yet.\n",

			assertErr: require.NoError,
			want:      nil,
		},
		{
			name: "invalid YAML",
			data: "findings: [",

			assertErr: errassert.Contains("parse baseline"),
		},
		{
			name: "missing id",
			data: "findings:\n  - status: accepted\n    reason: x\n",

			assertErr: errassert.Contains("baseline entry 1: missing id"),
		},
		{
			name: "duplicate id",
			data: "findings:\n  - {id: a, status: accepted, reason: x}\n  - {id: a, status: accepted, reason: y}\n",

			assertErr: errassert.Contains("baseline entry 2: duplicate id a"),
		},
		{
			name: "invalid status",
			data: "findings:\n  - {id: a, status: fixed, reason: x}\n",

			assertErr: errassert.Contains("status must be one of accepted, false-positive"),
		},
		{
			name: "missing reason",
			data: "findings:\n  - {id: a, status: accepted, reason: \" \"}\n",

			assertErr: errassert.Contains("baseline entry 1: missing reason"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseBaseline([]byte(tc.data))

			tc.assertErr(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestAppendBaseline(t *testing.T) {
	t.Parallel()

	newEntry := entry{ID: "v3:new", Status: "accepted", Reason: "A reason that is longer than eighty characters stays on one line in the file.", Date: "2026-10-29"}

	tests := []struct {
		name string
		data string

		assertErr require.ErrorAssertionFunc
		want      string
	}{
		{
			name: "empty file",
			data: "",

			assertErr: require.NoError,
			want: `findings:
  - id: v3:new
    status: accepted
    reason: A reason that is longer than eighty characters stays on one line in the file.
    date: "2026-10-29"
`,
		},
		{
			name: "only comments",
			data: "# Header.\n",

			assertErr: require.NoError,
			want: `# Header.
findings:
  - id: v3:new
    status: accepted
    reason: A reason that is longer than eighty characters stays on one line in the file.
    date: "2026-10-29"
`,
		},
		{
			name: "only comments without trailing newline",
			data: "# Header.",

			assertErr: require.NoError,
			want: `# Header.
findings:
  - id: v3:new
    status: accepted
    reason: A reason that is longer than eighty characters stays on one line in the file.
    date: "2026-10-29"
`,
		},
		{
			name: "empty flow list",
			data: "# Header.\nfindings: []\n",

			assertErr: require.NoError,
			want: `# Header.
findings:
  - id: v3:new
    status: accepted
    reason: A reason that is longer than eighty characters stays on one line in the file.
    date: "2026-10-29"
`,
		},
		{
			name: "null list",
			data: "findings:\n",

			assertErr: require.NoError,
			want: `findings:
  - id: v3:new
    status: accepted
    reason: A reason that is longer than eighty characters stays on one line in the file.
    date: "2026-10-29"
`,
		},
		{
			name: "existing entries and comments",
			data: `# Header.
findings:
  # First entry.
  - id: v3:old
    status: false-positive
    reason: Old.
`,

			assertErr: require.NoError,
			want: `# Header.
findings:
  # First entry.
  - id: v3:old
    status: false-positive
    reason: Old.
  - id: v3:new
    status: accepted
    reason: A reason that is longer than eighty characters stays on one line in the file.
    date: "2026-10-29"
`,
		},
		{
			name: "invalid YAML",
			data: "findings: [",

			assertErr: errassert.Contains("parse baseline"),
		},
		{
			name: "top level is a list",
			data: "- a\n",

			assertErr: errassert.Contains("top level is not a mapping"),
		},
		{
			name: "findings is not a list",
			data: "findings: a\n",

			assertErr: errassert.Contains("findings is not a list"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := appendBaseline([]byte(tc.data), []entry{newEntry})

			tc.assertErr(t, err)
			require.Equal(t, tc.want, string(got))
		})
	}
}
