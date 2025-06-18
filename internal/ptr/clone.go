package ptr

import "encoding/json"

// Clone creates a copy of the given argument by using json.Marshal and
// json.Unmarshal. Therefore, this only works for arguments, that can be
// json marshalled. Als private attributes are not handled as a limitation
// of this.
func Clone[T any](v *T) (*T, error) {
	var clone T

	body, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(body, &clone)
	if err != nil {
		return nil, err
	}

	return &clone, nil
}
