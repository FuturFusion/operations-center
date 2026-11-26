package api

import (
	"net/http"
	"strconv"
)

const (
	recursionNone    = 0
	recursionObjects = 1
	recursionFull    = 2
)

func recursionLevel(r *http.Request) int {
	recursion, err := strconv.Atoi(r.FormValue("recursion"))
	if err != nil || recursion < recursionNone {
		return recursionNone
	}

	return min(recursion, recursionFull)
}

func isRecursionRequest(r *http.Request) bool {
	return recursionLevel(r) != recursionNone
}
