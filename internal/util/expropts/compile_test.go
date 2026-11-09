package expropts_test

import (
	"strings"
	"testing"

	"github.com/expr-lang/expr"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/expropts"
	"github.com/FuturFusion/operations-center/internal/util/testing/errassert"
)

func TestCompile(t *testing.T) {
	t.Parallel()

	type Env struct {
		Name string `expr:"name"`
	}

	nested := func(open string, closing string, depth int) string {
		return strings.Repeat(open, depth) + "true" + strings.Repeat(closing, depth)
	}

	// Pad a string literal, so the expression is exactly length bytes long.
	ofLength := func(length int) string {
		prefix := `name == "`
		return prefix + strings.Repeat("a", length-len(prefix)-1) + `"`
	}

	tests := []struct {
		name  string
		input string

		assertErr     require.ErrorAssertionFunc
		assertProgram require.ValueAssertionFunc
	}{
		{
			name:  "success - simple expression",
			input: `name == "one"`,

			assertErr:     require.NoError,
			assertProgram: require.NotNil,
		},
		{
			name:  "success - maximum length",
			input: ofLength(expropts.MaxExpressionLength),

			assertErr:     require.NoError,
			assertProgram: require.NotNil,
		},
		{
			name:  "success - maximum nesting depth",
			input: nested("(", ")", expropts.MaxExpressionNestingDepth),

			assertErr:     require.NoError,
			assertProgram: require.NotNil,
		},
		{
			name:  "success - sequential brackets are not nested",
			input: strings.Repeat("(true) && ", expropts.MaxExpressionNestingDepth+1) + "true",

			assertErr:     require.NoError,
			assertProgram: require.NotNil,
		},
		{
			name:  "success - brackets in string literal are not counted",
			input: `name == "` + strings.Repeat("([{", expropts.MaxExpressionNestingDepth) + `"`,

			assertErr:     require.NoError,
			assertProgram: require.NotNil,
		},
		{
			name:  "error - too long",
			input: ofLength(expropts.MaxExpressionLength + 1),

			assertErr:     errassert.ValidationErrorContains("Expression is too long"),
			assertProgram: require.Nil,
		},
		{
			name:  "error - too long with many opening parentheses",
			input: strings.Repeat("(", 500_000),

			assertErr:     errassert.ValidationErrorContains("Expression is too long"),
			assertProgram: require.Nil,
		},
		{
			name:  "error - nested too deeply with parentheses",
			input: nested("(", ")", expropts.MaxExpressionNestingDepth+1),

			assertErr:     errassert.ValidationErrorContains("Expression is nested too deeply"),
			assertProgram: require.Nil,
		},
		{
			name:  "error - nested too deeply with square brackets",
			input: nested("[", "]", expropts.MaxExpressionNestingDepth+1),

			assertErr:     errassert.ValidationErrorContains("Expression is nested too deeply"),
			assertProgram: require.Nil,
		},
		{
			name:  "error - nested too deeply with mixed brackets",
			input: strings.Repeat("([{", 11) + "true",

			assertErr:     errassert.ValidationErrorContains("Expression is nested too deeply"),
			assertProgram: require.Nil,
		},
		{
			name:  "error - lexer error is returned by expr.Compile",
			input: `name == "unterminated`,

			assertErr:     errassert.Contains("literal not terminated"),
			assertProgram: require.Nil,
		},
		{
			name:  "error - compile error is returned by expr.Compile",
			input: `"string"`,

			assertErr:     errassert.Contains("expected bool"),
			assertProgram: require.Nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			program, err := expropts.Compile(tc.input, expr.Env(Env{}), expr.AsBool())

			tc.assertErr(t, err)
			tc.assertProgram(t, program)
		})
	}
}
