package api

import (
	"net/http"

	"github.com/FuturFusion/operations-center/internal/authz"
	"github.com/FuturFusion/operations-center/internal/response"
	"github.com/FuturFusion/operations-center/shared/api"
)

func assertPermission(authorizer *authz.Authorizer, _ authz.ObjectType, entitlement authz.Entitlement) func(next response.HandlerFunc) response.HandlerFunc {
	return func(next response.HandlerFunc) response.HandlerFunc {
		return func(r *http.Request) response.Response {
			details, err := authz.ExtractRequestDetails(r)
			if err != nil {
				return response.SmartError(api.StatusErrorf(http.StatusForbidden, "Failed to extract request details: %v", err))
			}

			err = (*authorizer).CheckPermission(r.Context(), details, authz.ObjectServer(), entitlement)
			if err != nil {
				return response.SmartError(err)
			}

			return next(r)
		}
	}
}
