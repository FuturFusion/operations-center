package response

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
)

// EtagHash hashes the provided data and returns the sha256.
func EtagHash(data any) (string, error) {
	etag := sha256.New()
	err := json.NewEncoder(etag).Encode(data)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", etag.Sum(nil)), nil
}
