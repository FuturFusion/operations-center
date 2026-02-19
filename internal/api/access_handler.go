package api

import (
	"net/http"

	"github.com/FuturFusion/operations-center/internal/security/authz"
	"github.com/FuturFusion/operations-center/internal/util/response"
	"github.com/FuturFusion/operations-center/shared/api"
)

func assertPermission(authorizer *authz.Authorizer, objectType authz.ObjectType, entitlement authz.Entitlement) func(next response.HandlerFunc) response.HandlerFunc { //nolint:unparam
	return func(next response.HandlerFunc) response.HandlerFunc {
		return func(r *http.Request) response.Response {
			resp := checkPermission(authorizer, r, objectType, entitlement)
			if resp != nil {
				return resp
			}

			return next(r)
		}
	}
}

func checkPermission(authorizer *authz.Authorizer, r *http.Request, objectType authz.ObjectType, entitlement authz.Entitlement) response.Response {
	obj, err := authz.ObjectFromRequest(r, objectType)
	if err != nil {
		return response.SmartError(err)
	}

	details, err := authz.ExtractRequestDetails(r)
	if err != nil {
		return response.SmartError(api.StatusErrorf(http.StatusForbidden, "Failed to extract request details: %v", err))
	}

	err = (*authorizer).CheckPermission(r.Context(), details, obj, entitlement)
	if err != nil {
		return response.SmartError(err)
	}

	return nil
}
