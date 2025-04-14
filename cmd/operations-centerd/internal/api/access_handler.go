package api

import (
	"net/http"

	"github.com/FuturFusion/operations-center/internal/authz"
	"github.com/FuturFusion/operations-center/internal/response"
)

func assertPermission(authorizer authz.Authorizer, _ authz.ObjectType, entitlement authz.Entitlement) func(next response.HandlerFunc) response.HandlerFunc {
	return func(next response.HandlerFunc) response.HandlerFunc {
		return func(r *http.Request) response.Response {
			err := authorizer.CheckPermission(r.Context(), r, authz.ObjectServer(), entitlement)
			if err != nil {
				return response.SmartError(err)
			}

			return next(r)
		}
	}
}
