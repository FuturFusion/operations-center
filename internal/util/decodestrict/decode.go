// Package decodestrict provides strict decoding helpers for JSON and YAML
// documents. In contrast to the plain encoding/json and yaml decoders, fields
// present in the document but unknown to the target type are reported as an
// error instead of being silently ignored.
package decodestrict

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"go.yaml.in/yaml/v4"
)

// JSON decodes the JSON document read from r into target. Fields present in the
// document, which are not known to target, cause an error.
func JSON(r io.Reader, target any) error {
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()

	err := decoder.Decode(target)
	if err != nil {
		return fmt.Errorf("Failed to decode JSON: %w", err)
	}

	return nil
}

// YAML decodes the YAML document in data into target. Fields present in the
// document, which are not known to target, cause an error. An empty document
// leaves target untouched and does not cause an error.
func YAML(data []byte, target any) error {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)

	err := decoder.Decode(target)
	if err != nil {
		// An empty document is reported as io.EOF by the decoder, while
		// yaml.Unmarshal accepts it. Keep the more forgiving behavior.
		if errors.Is(err, io.EOF) {
			return nil
		}

		return fmt.Errorf("Failed to decode YAML: %w", err)
	}

	return nil
}
