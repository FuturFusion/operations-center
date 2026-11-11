package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/testing/errassert"
)

func TestMarkdownReport(t *testing.T) {
	t.Parallel()

	twoFindings := []finding{
		{Number: "F1", Title: "One", Severity: "HIGH"},
		{Number: "F2", Title: "Two", Severity: "LOW"},
	}

	tests := []struct {
		name     string
		markdown string
		findings []finding

		assertSplitErr require.ErrorAssertionFunc
		assertCheckErr require.ErrorAssertionFunc
		wantOrder      []string
		wantWarnings   []string
	}{
		{
			name:     "valid, findings section at the end",
			markdown: "# R\n\n## Findings\n\n### F1 — One (HIGH, confidence high)\n\n```\n### F9 — Code (LOW, confidence low)\n```\n\n### F2 – Two (LOW, confidence low)\n",
			findings: twoFindings,

			assertSplitErr: require.NoError,
			assertCheckErr: require.NoError,
			wantOrder:      []string{"F1", "F2"},
		},
		{
			name:     "no findings section and no findings",
			markdown: "# R\n\nNo findings.\n",
			findings: nil,

			assertSplitErr: require.NoError,
			assertCheckErr: require.NoError,
		},
		{
			name:     "empty findings section",
			markdown: "# R\n\n## Findings\n\nNone.\n\n## What was verified\n",
			findings: nil,

			assertSplitErr: require.NoError,
			assertCheckErr: require.NoError,
		},
		{
			name:     "title differs",
			markdown: "## Findings\n### F1 - Other (HIGH, confidence high)\n### F2 - Two (LOW, confidence low)\n",
			findings: twoFindings,

			assertSplitErr: require.NoError,
			assertCheckErr: require.NoError,
			wantOrder:      []string{"F1", "F2"},
			wantWarnings:   []string{"F1: the markdown title differs from the JSONL title"},
		},
		{
			name:     "unexpected heading",
			markdown: "## Findings\n### Something else\n",

			assertSplitErr: errassert.Contains(`markdown line 2: unexpected heading "### Something else"`),
			assertCheckErr: require.NoError,
		},
		{
			name:     "second section for one finding",
			markdown: "## Findings\n### F1 - One (HIGH, confidence high)\n### F1 - One (HIGH, confidence high)\n",

			assertSplitErr: errassert.Contains("markdown line 3: second section for F1"),
			assertCheckErr: require.NoError,
		},
		{
			name:     "missing and unknown sections",
			markdown: "## Findings\n### F1 - One (HIGH, confidence high)\n### F3 - Three (LOW, confidence low)\n",
			findings: twoFindings,

			assertSplitErr: require.NoError,
			assertCheckErr: errassert.Contains("missing sections: F2, unknown sections: F3"),
			wantOrder:      []string{"F1", "F3"},
		},
		{
			name:     "severity differs",
			markdown: "## Findings\n### F1 - One (LOW, confidence high)\n### F2 - Two (LOW, confidence low)\n",
			findings: twoFindings,

			assertSplitErr: require.NoError,
			assertCheckErr: errassert.Contains("F1: severity LOW in markdown, HIGH in JSONL"),
			wantOrder:      []string{"F1", "F2"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			md, splitErr := splitMarkdown(tc.markdown)
			warnings, checkErr := md.check(tc.findings)

			tc.assertSplitErr(t, splitErr)
			tc.assertCheckErr(t, checkErr)
			require.Equal(t, tc.wantOrder, md.order, "order of the finding sections")
			require.Equal(t, tc.wantWarnings, warnings)
		})
	}
}
