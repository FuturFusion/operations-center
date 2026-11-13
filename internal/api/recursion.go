package api

import (
	"net/http"
	"strconv"
)

// isRecursionRequest follows Incus: any non-zero integer means recursive, an invalid value means not recursive.
func isRecursionRequest(r *http.Request) bool {
	recursion, err := strconv.Atoi(r.FormValue("recursion"))
	if err != nil {
		return false
	}

	return recursion != 0
}
