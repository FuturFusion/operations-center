package api

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strconv"
)

// ConfigMap type is used to hold incus config. In contrast to plain
// map[string]string it provides unmarshal methods for JSON and YAML, which
// gracefully handle numbers and bools.
//
// swagger:model
// swagger:type object
//
// Example:
//
//	{"user.mykey": "foo"}
type ConfigMap map[string]string

// Backwards compatibility tests.
var (
	_ map[string]string = ConfigMap{}
	_ ConfigMap         = map[string]string{}
)

// UnmarshalJSON implements json.Unmarshaler interface.
func (m *ConfigMap) UnmarshalJSON(data []byte) error {
	var raw map[string]any

	err := json.Unmarshal(data, &raw)
	if err != nil {
		return fmt.Errorf("JSON data not valid for ConfigMap: %w", err)
	}

	return m.fromMapStringAny(raw)
}

// UnmarshalYAML implements yaml.Unmarshaler interface.
func (m *ConfigMap) UnmarshalYAML(unmarshal func(any) error) error {
	var raw map[string]any
	err := unmarshal(&raw)
	if err != nil {
		return fmt.Errorf("YAML data not valid for ConfigMap: %w", err)
	}

	return m.fromMapStringAny(raw)
}

func (m *ConfigMap) fromMapStringAny(raw map[string]any) error {
	if raw == nil {
		return nil
	}

	result := *m
	if result == nil {
		result = make(ConfigMap, len(raw))
	}

	for k, v := range raw {
		switch val := v.(type) {
		case string:
			result[k] = val

		case int:
			result[k] = strconv.FormatInt(int64(val), 10)

		case uint64:
			result[k] = strconv.FormatUint(val, 10)

		case float64:
			if val == float64(int64(val)) {
				result[k] = strconv.FormatInt(int64(val), 10)
			} else {
				result[k] = strconv.FormatFloat(val, 'g', -1, 64)
			}

		case bool:
			result[k] = strconv.FormatBool(val)

		case nil:
			result[k] = ""

		default:
			return fmt.Errorf("type %T is not supported in %T", v, m)
		}
	}

	*m = result

	return nil
}

// Value implements the sql driver.Valuer interface.
func (m ConfigMap) Value() (driver.Value, error) {
	return json.Marshal(m)
}

// Scan implements the sql.Scanner interface.
func (m *ConfigMap) Scan(value any) error {
	if value == nil {
		return fmt.Errorf("null is not a valid ConfigMap")
	}

	switch v := value.(type) {
	case string:
		if len(v) == 0 {
			*m = ConfigMap{}
			return nil
		}

		return json.Unmarshal([]byte(v), m)

	case []byte:
		if len(v) == 0 {
			*m = ConfigMap{}
			return nil
		}

		return json.Unmarshal(v, m)

	default:
		return fmt.Errorf("type %T is not supported for ConfigMap", value)
	}
}
