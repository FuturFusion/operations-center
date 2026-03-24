package expropts

import "reflect"

func ToFloat64(params ...any) (any, error) {
	a := reflect.ValueOf(params[0])

	// Both convertible to the same base kind? Compare as base.
	if a.Type().ConvertibleTo(reflect.TypeFor[float64]()) {
		switch {
		case a.CanFloat():
			return float64(a.Float()), nil

		case a.CanInt():
			return float64(a.Int()), nil

		case a.CanUint():
			return float64(a.Uint()), nil
		}
	}

	return 0.0, nil
}
