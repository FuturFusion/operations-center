package authn

import (
	"errors"
	"fmt"
	"mime"
	"net/http"
)

// checkContentType rejects requests, which a browser can send cross-site
// without a CORS preflight. This is a second layer of CSRF protection.
// Browsers can send a POST without a body, so POST always needs a Content-Type.
// Other unsafe methods always cause a preflight, so they only need it with a
// body.
func checkContentType(r *http.Request) error {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return nil
	}

	// A browser can not connect to the unix socket.
	if r.RemoteAddr == "@" && r.TLS == nil {
		return nil
	}

	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		// A length of -1 means unknown, the request might have a body.
		if r.Method == http.MethodPost || r.ContentLength != 0 {
			return errors.New("Missing Content-Type")
		}

		return nil
	}

	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return fmt.Errorf("Invalid Content-Type %q: %w", contentType, err)
	}

	switch mediaType {
	case "text/plain", "application/x-www-form-urlencoded", "multipart/form-data":
		return fmt.Errorf("Unsupported Content-Type %q", contentType)
	}

	return nil
}
