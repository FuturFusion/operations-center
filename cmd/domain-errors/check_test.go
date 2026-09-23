package main

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/cmd/domain-errors/scan"
)

func TestCheck(t *testing.T) {
	tests := []struct {
		name    string
		pattern string

		wantRules []string
	}{
		{
			name:    "violations - one finding per rule",
			pattern: "./testdata/violations",

			wantRules: []string{
				"bare-kind-escapes",
				"cause-reclassifies-error",
				"detail-key-not-snake-case",
				"hint-client-specific",
				"hint-no-full-sentence",
				"hint-not-capitalized",
				"kind-without-domain-error",
				"kind-without-domain-error",
				"message-empty",
				"message-not-capitalized",
				"message-trailing-period",
				"message-wraps-error",
				"reason-not-declared",
				"unclassified-boundary-error",
			},
		},
		{
			name:    "clean - no findings",
			pattern: "./testdata/clean",

			wantRules: nil,
		},
		{
			name:    "service - an error without a kind, the classified ones are silent",
			pattern: "./testdata/service",

			wantRules: []string{"error-without-kind"},
		},
		{
			// A bare kind is the sentinel, which the service above translates,
			// so it is only reported outside this layer.
			name:    "repo - a bare kind is allowed",
			pattern: "./testdata/repo/sqlite",

			wantRules: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pkgs, err := scan.Load(tc.pattern)
			require.NoError(t, err)

			result, err := scan.Scan(pkgs)
			require.NoError(t, err)

			// The reasons are only declared in shared/api, which is not part of
			// the inspected package, so every one of them would be reported as
			// unused.
			result.DeclaredReasons = nil

			var gotRules []string
			for _, f := range check(result) {
				gotRules = append(gotRules, f.Rule)
			}

			sort.Strings(gotRules)

			require.Equal(t, tc.wantRules, gotRules)
		})
	}
}

func TestCheckUnknownConstructs(t *testing.T) {
	result := &scan.Result{
		UnknownConstructs: []scan.UnknownConstruct{
			{Name: "NewSomethingErr", What: "constructor"},
			{Name: "WithSuggestion", What: "builder method of domain.Error"},
		},
	}

	findings := check(result)

	require.Len(t, findings, 2)

	for _, f := range findings {
		require.Equal(t, "unknown-domain-construct", f.Rule)
	}

	require.Contains(t, findings[0].Msg+findings[1].Msg, "NewSomethingErr")
	require.Contains(t, findings[0].Msg+findings[1].Msg, "WithSuggestion")
}

func TestScanKnowsEveryDomainConstruct(t *testing.T) {
	pkgs, err := scan.Load(scan.DomainPkg)
	require.NoError(t, err)

	result, err := scan.Scan(pkgs)
	require.NoError(t, err)

	require.Empty(t, result.UnknownConstructs,
		"the domain package gained a way of building an error, which cmd/domain-errors/scan does not read")
}

func TestCheckIgnoreDirective(t *testing.T) {
	pkgs, err := scan.Load("./testdata/violations")
	require.NoError(t, err)

	result, err := scan.Scan(pkgs)
	require.NoError(t, err)

	var ignored, reported int

	for _, wrap := range result.KindWraps {
		if wrap.Ignored {
			ignored++

			continue
		}

		reported++
	}

	require.Equal(t, 1, ignored, "exactly one kind wrap carries an ignore directive")
	require.Equal(t, 2, reported, "the other kind wraps are reported")
}
