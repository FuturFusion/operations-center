package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/klauspost/compress/gzhttp"
)

// compressionMinSize is the smallest response body in bytes that is compressed.
// Smaller bodies gain little and cost CPU.
const compressionMinSize = 1024

func compressionMiddleware(next http.Handler) (http.Handler, error) {
	wrapper, err := gzhttp.NewWrapper(
		gzhttp.MinSize(compressionMinSize),
		gzhttp.ContentTypes([]string{"application/json"}),
		gzhttp.EnableZstd(false),
	)
	if err != nil {
		return nil, fmt.Errorf("Failed to create compression middleware: %w", err)
	}

	// gzhttp discards errors from its final response flush and close.
	compressed := wrapper(next)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Pass IncusOS responses through as IncusOS sent them.
		if isProxyPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		compressed.ServeHTTP(w, r)
	}), nil
}

func isProxyPath(path string) bool {
	if strings.HasPrefix(path, "/os/") {
		return true
	}

	rest, ok := strings.CutPrefix(path, "/1.0/provisioning/servers/")
	if !ok {
		return false
	}

	_, subPath, ok := strings.Cut(rest, "/")
	if !ok {
		return false
	}

	return subPath == "os" || strings.HasPrefix(subPath, "os/")
}
