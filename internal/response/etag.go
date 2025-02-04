package response

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/FuturFusion/operations-center/shared/api"
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

// EtagCheck validates the hash of the current state with the hash
// provided by the client.
func EtagCheck(r *http.Request, data any) error {
	match := r.Header.Get("If-Match")
	if match == "" {
		return nil
	}

	match = strings.Trim(match, "\"")

	hash, err := EtagHash(data)
	if err != nil {
		return err
	}

	if hash != match {
		return api.StatusErrorf(http.StatusPreconditionFailed, "ETag doesn't match: %s vs %s", hash, match)
	}

	return nil
}
