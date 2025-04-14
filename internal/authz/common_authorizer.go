package authz

import (
	"fmt"
	"net/http"

	"github.com/FuturFusion/operations-center/internal/authn"
)

// RequestDetails is a type representing an authorization request.
type RequestDetails struct {
	Username string
	Protocol string
}

type CommonAuthorizer struct{}

func (c *CommonAuthorizer) RequestDetails(r *http.Request) (*RequestDetails, error) {
	if r == nil {
		return nil, fmt.Errorf("Cannot inspect nil request")
	}

	if r.URL == nil {
		return nil, fmt.Errorf("Request URL is not set")
	}

	val := r.Context().Value(authn.CtxUsername)
	if val == nil {
		return nil, fmt.Errorf("Username not present in request context")
	}

	username, ok := val.(string)
	if !ok {
		return nil, fmt.Errorf("Request context username has incorrect type")
	}

	val = r.Context().Value(authn.CtxProtocol)
	if val == nil {
		return nil, fmt.Errorf("Protocol not present in request context")
	}

	protocol, ok := val.(string)
	if !ok {
		return nil, fmt.Errorf("Request context protocol has incorrect type")
	}

	return &RequestDetails{
		Username: username,
		Protocol: protocol,
	}, nil
}
