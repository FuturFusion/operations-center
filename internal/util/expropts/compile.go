package expropts

import (
	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/file"
	"github.com/expr-lang/expr/parser/lexer"
	"github.com/expr-lang/expr/vm"

	"github.com/FuturFusion/operations-center/internal/domain"
)

const (
	// MaxExpressionLength is the maximum length of an expression in bytes.
	MaxExpressionLength = 4096

	// MaxExpressionNestingDepth is the maximum nesting depth of brackets in an expression.
	MaxExpressionNestingDepth = 32
)

// Compile checks the length and the nesting depth of input and then calls
// expr.Compile. The expr parser has no recursion limit. Deep nesting
// overflows the stack, which ends the process.
func Compile(input string, opts ...expr.Option) (*vm.Program, error) {
	if len(input) > MaxExpressionLength {
		return nil, domain.NewValidationErrf("Expression is too long: %d bytes, maximum is %d bytes", len(input), MaxExpressionLength)
	}

	// On a lexer error, expr.Compile returns the same error with a better message.
	tokens, err := lexer.Lex(file.NewSource(input))
	if err == nil {
		depth := 0
		for _, token := range tokens {
			switch {
			case token.Is(lexer.Bracket, "(", "[", "{"):
				depth++
				if depth > MaxExpressionNestingDepth {
					return nil, domain.NewValidationErrf("Expression is nested too deeply: maximum nesting depth is %d", MaxExpressionNestingDepth)
				}

			case token.Is(lexer.Bracket, ")", "]", "}"):
				depth--
			}
		}
	}

	return expr.Compile(input, opts...)
}
