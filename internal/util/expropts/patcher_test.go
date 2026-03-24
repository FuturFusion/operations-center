package expropts_test

import (
	"testing"

	"github.com/expr-lang/expr"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/expropts"
)

type SpecialFloat32 float32

type SpecialFloat64 float64

type SpecialInt int

type SpecialInt32 int32

type SpecialString string

type SpecialUint uint

type SpecialUint32 uint32

type Data struct {
	SpecialFloat32 SpecialFloat32
	Float32        float32

	SpecialFloat64 SpecialFloat64
	Float64        float64

	SpecialInt SpecialInt
	Int        int

	SpecialInt32 SpecialInt32
	Int32        int32

	SpecialDescription SpecialString
	Description        string

	SpecialUint SpecialUint
	Uint        uint

	SpecialUint32 SpecialUint32
	Uint32        uint32
}

func TestUnderlyingBaseTypePatcher(t *testing.T) {
	tests := []struct {
		name       string
		expression string

		assertCompileErr require.ErrorAssertionFunc
		assertRuntimeErr require.ErrorAssertionFunc
		assertEquality   require.BoolAssertionFunc
	}{
		// float32
		{
			name:       `float32 property == float32`,
			expression: `Float32 == 9.25`,

			assertCompileErr: require.NoError,
			assertRuntimeErr: require.NoError,
			assertEquality:   require.True,
		},
		{
			name:       `special float32 property == float32`,
			expression: `SpecialFloat32 == 9.25`,

			assertCompileErr: require.NoError,
			assertRuntimeErr: require.NoError,
			assertEquality:   require.True,
		},
		{
			name:       `float32 == special float32 property`,
			expression: `9.25 == SpecialFloat32`,

			assertCompileErr: require.NoError,
			assertRuntimeErr: require.NoError,
			assertEquality:   require.True,
		},
		{
			name:       `special float32 property == float32 property`,
			expression: `SpecialFloat32 == Float32`,

			assertCompileErr: require.NoError,
			assertRuntimeErr: require.NoError,
			assertEquality:   require.True,
		},
		{
			name:       `special float32 property == special float32 property`,
			expression: `SpecialFloat32 == SpecialFloat32`,

			assertCompileErr: require.NoError,
			assertRuntimeErr: require.NoError,
			assertEquality:   require.True,
		},

		// float64
		{
			name:       `float64 property == float64`,
			expression: `Float64 == 10.5`,

			assertCompileErr: require.NoError,
			assertRuntimeErr: require.NoError,
			assertEquality:   require.True,
		},
		{
			name:       `special float64 property == float64`,
			expression: `SpecialFloat64 == 10.5`,

			assertCompileErr: require.NoError,
			assertRuntimeErr: require.NoError,
			assertEquality:   require.True,
		},
		{
			name:       `float64 == special float64 property`,
			expression: `10.5 == SpecialFloat64`,

			assertCompileErr: require.NoError,
			assertRuntimeErr: require.NoError,
			assertEquality:   require.True,
		},
		{
			name:       `special float64 property == float64 property`,
			expression: `SpecialFloat64 == Float64`,

			assertCompileErr: require.NoError,
			assertRuntimeErr: require.NoError,
			assertEquality:   require.True,
		},
		{
			name:       `special float64 property == special float64 property`,
			expression: `SpecialFloat64 == SpecialFloat64`,

			assertCompileErr: require.NoError,
			assertRuntimeErr: require.NoError,
			assertEquality:   require.True,
		},

		// int
		{
			name:       `int property == int`,
			expression: `Int == 10`,

			assertCompileErr: require.NoError,
			assertRuntimeErr: require.NoError,
			assertEquality:   require.True,
		},
		{
			name:       `special int property == int`,
			expression: `SpecialInt == 10`,

			assertCompileErr: require.NoError,
			assertRuntimeErr: require.NoError,
			assertEquality:   require.True,
		},
		{
			name:       `int == special int property`,
			expression: `10 == SpecialInt`,

			assertCompileErr: require.NoError,
			assertRuntimeErr: require.NoError,
			assertEquality:   require.True,
		},
		{
			name:       `special int property == int property`,
			expression: `SpecialInt == Int`,

			assertCompileErr: require.NoError,
			assertRuntimeErr: require.NoError,
			assertEquality:   require.True,
		},
		{
			name:       `special int property == special int property`,
			expression: `SpecialInt == SpecialInt`,

			assertCompileErr: require.NoError,
			assertRuntimeErr: require.NoError,
			assertEquality:   require.True,
		},

		// int32
		{
			name:       `int32 property == int32`,
			expression: `Int32 == 32`,

			assertCompileErr: require.NoError,
			assertRuntimeErr: require.NoError,
			assertEquality:   require.True,
		},
		{
			name:       `special int32 property == int32`,
			expression: `SpecialInt32 == 32`,

			assertCompileErr: require.NoError,
			assertRuntimeErr: require.NoError,
			assertEquality:   require.True,
		},
		{
			name:       `int32 == special int32 property`,
			expression: `32 == SpecialInt32`,

			assertCompileErr: require.NoError,
			assertRuntimeErr: require.NoError,
			assertEquality:   require.True,
		},
		{
			name:       `special int32 property == int32 property`,
			expression: `SpecialInt32 == Int32`,

			assertCompileErr: require.NoError,
			assertRuntimeErr: require.NoError,
			assertEquality:   require.True,
		},
		{
			name:       `special int32 property == special int32 property`,
			expression: `SpecialInt32 == SpecialInt32`,

			assertCompileErr: require.NoError,
			assertRuntimeErr: require.NoError,
			assertEquality:   require.True,
		},

		// string
		{
			name:       `string property == string`,
			expression: `Description == "foo"`,

			assertCompileErr: require.NoError,
			assertRuntimeErr: require.NoError,
			assertEquality:   require.True,
		},
		{
			name:       `special string property == string`,
			expression: `SpecialDescription == "foo"`,

			assertCompileErr: require.NoError,
			assertRuntimeErr: require.NoError,
			assertEquality:   require.True,
		},
		{
			name:       `string == special string property`,
			expression: `"foo" == SpecialDescription`,

			assertCompileErr: require.NoError,
			assertRuntimeErr: require.NoError,
			assertEquality:   require.True,
		},
		{
			name:       `special string property == string property`,
			expression: `SpecialDescription == Description`,

			assertCompileErr: require.NoError,
			assertRuntimeErr: require.NoError,
			assertEquality:   require.True,
		},
		{
			name:       `special string property == special string property`,
			expression: `SpecialDescription == SpecialDescription`,

			assertCompileErr: require.NoError,
			assertRuntimeErr: require.NoError,
			assertEquality:   require.True,
		},
		{
			name:       `special string property == string with cast`,
			expression: `string(SpecialDescription) == "foo"`,

			assertCompileErr: require.NoError,
			assertRuntimeErr: require.NoError,
			assertEquality:   require.True,
		},
		{
			name:       `special string property == special string property with one side casted`,
			expression: `SpecialDescription == string(SpecialDescription)`,

			assertCompileErr: require.NoError,
			assertRuntimeErr: require.NoError,
			assertEquality:   require.True,
		},

		// uint
		{
			name:       `uint property == uint`,
			expression: `Uint == 100`,

			assertCompileErr: require.NoError,
			assertRuntimeErr: require.NoError,
			assertEquality:   require.True,
		},
		{
			name:       `special uint property == uint`,
			expression: `SpecialUint == 100`,

			assertCompileErr: require.NoError,
			assertRuntimeErr: require.NoError,
			assertEquality:   require.True,
		},
		{
			name:       `uint == special uint property`,
			expression: `100 == SpecialUint`,

			assertCompileErr: require.NoError,
			assertRuntimeErr: require.NoError,
			assertEquality:   require.True,
		},
		{
			name:       `special uint property == uint property`,
			expression: `SpecialUint == Uint`,

			assertCompileErr: require.NoError,
			assertRuntimeErr: require.NoError,
			assertEquality:   require.True,
		},
		{
			name:       `special uint property == special uint property`,
			expression: `SpecialUint == SpecialUint`,

			assertCompileErr: require.NoError,
			assertRuntimeErr: require.NoError,
			assertEquality:   require.True,
		},

		// uint32
		{
			name:       `uint32 property == uint32`,
			expression: `Uint32 == 320`,

			assertCompileErr: require.NoError,
			assertRuntimeErr: require.NoError,
			assertEquality:   require.True,
		},
		{
			name:       `special uint32 property == uint32`,
			expression: `SpecialUint32 == 320`,

			assertCompileErr: require.NoError,
			assertRuntimeErr: require.NoError,
			assertEquality:   require.True,
		},
		{
			name:       `uint32 == special uint32 property`,
			expression: `320 == SpecialUint32`,

			assertCompileErr: require.NoError,
			assertRuntimeErr: require.NoError,
			assertEquality:   require.True,
		},
		{
			name:       `special uint32 property == uint32 property`,
			expression: `SpecialUint32 == Uint32`,

			assertCompileErr: require.NoError,
			assertRuntimeErr: require.NoError,
			assertEquality:   require.True,
		},
		{
			name:       `special uint32 property == special uint32 property`,
			expression: `SpecialUint32 == SpecialUint32`,

			assertCompileErr: require.NoError,
			assertRuntimeErr: require.NoError,
			assertEquality:   require.True,
		},

		// Other binary operator
		{
			name:       `other binary operator`,
			expression: `"foo" startsWith "f"`,

			assertCompileErr: require.NoError,
			assertRuntimeErr: require.NoError,
			assertEquality:   require.True,
		},

		// Errors
		{
			name:       `not existing property`,
			expression: `SpecialDescription1 == ""`,

			assertCompileErr: require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			filterExpression, err := expr.Compile(
				tc.expression,
				expr.Env(Data{}),
				expr.AsBool(),
				expr.Patch(expropts.UnderlyingBaseTypePatcher{}),
				expr.Function("toFloat64", expropts.ToFloat64,
					new(func(any) float64)),
			)
			tc.assertCompileErr(t, err)

			if err != nil {
				return
			}

			result, err := expr.Run(filterExpression, Data{
				Float32:            9.25,
				SpecialFloat32:     9.25,
				Float64:            10.5,
				SpecialFloat64:     10.5,
				Int:                10,
				SpecialInt:         10,
				Int32:              32,
				SpecialInt32:       32,
				Description:        "foo",
				SpecialDescription: "foo",
				Uint:               100,
				SpecialUint:        100,
				Uint32:             320,
				SpecialUint32:      320,
			})
			tc.assertRuntimeErr(t, err)

			if err != nil {
				return
			}

			tc.assertEquality(t, result.(bool))
		})
	}
}
