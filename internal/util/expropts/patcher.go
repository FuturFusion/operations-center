package expropts

import (
	"github.com/expr-lang/expr/ast"
)

type UnderlyingBaseTypePatcher struct{}

func (UnderlyingBaseTypePatcher) Visit(node *ast.Node) {
	bin, ok := (*node).(*ast.BinaryNode)
	if !ok {
		return
	}

	switch bin.Operator {
	case "==", "!=", "<", ">", "<=", ">=":
	default:
		return
	}

	if bin.Left.Type() == bin.Right.Type() {
		return
	}

	if !bin.Left.Type().ConvertibleTo(bin.Right.Type()) {
		return
	}

	base := bin.Left
	target := &bin.Right

	switch base.Type().String() {
	case "float32", "float64", "int", "int8", "int16", "int32", "int64", "string", "uint", "uint8", "uint16", "uint32", "uint64":
	default:
		base = bin.Right
		target = &bin.Left
	}

	switch base.Type().String() {
	case "float32", "float64":
		*target = &ast.CallNode{
			Callee: &ast.IdentifierNode{
				Value: "toFloat64",
			},
			Arguments: []ast.Node{*target},
		}

	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64":
		*target = &ast.BuiltinNode{
			Name:      "int",
			Arguments: []ast.Node{*target},
		}

	case "string":
		*target = &ast.BuiltinNode{
			Name:      "string",
			Arguments: []ast.Node{*target},
		}
	}
}
