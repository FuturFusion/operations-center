package terraform

import "reflect"

func maxKeyLength(m any) int {
	v := reflect.ValueOf(m)

	if v.Kind() != reflect.Map {
		return 0
	}

	if v.Type().Key().Kind() != reflect.String {
		return 0
	}

	maxLen := 0
	for _, key := range v.MapKeys() {
		maxLen = max(maxLen, len(key.String()))
	}

	return maxLen
}
