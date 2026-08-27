//nolint:unused
package api

import (
	"github.com/FuturFusion/operations-center/shared/api"
	apisystem "github.com/FuturFusion/operations-center/shared/api/system"
)

// The types in this file exist only so that `swagger generate spec` can derive
// the responses of the API from real Go types.

// swaggerSyncResponseBody holds the envelope every sync response shares.
type swaggerSyncResponseBody struct {
	// Example: sync
	Type string `json:"type"`

	// Example: Success
	Status string `json:"status"`

	// Example: 200
	StatusCode int `json:"status_code"`
}

// A list of relative URLs, one per entry.
//
// swagger:response URLsResponse
type swaggerURLsResponse struct {
	// in: body
	Body struct {
		swaggerSyncResponseBody
		// Example: ["/1.0/provisioning/servers/server1", "/1.0/provisioning/servers/server2"]
		Metadata []string `json:"metadata"`
	}
}

// List of API endpoints
//
// swagger:response APIEndpointsResponse
type swaggerAPIEndpointsResponse struct {
	// in: body
	Body struct {
		swaggerSyncResponseBody
		Metadata []string `json:"metadata"`
	}
}

// Server environment and configuration
//
// swagger:response ServerUntrustedResponse
type swaggerServerUntrustedResponse struct {
	// in: body
	Body struct {
		swaggerSyncResponseBody
		Metadata api.ServerUntrusted `json:"metadata"`
	}
}

// The channel
//
// swagger:response ChannelResponse
type swaggerChannelResponse struct {
	// in: body
	Body struct {
		swaggerSyncResponseBody
		Metadata api.Channel `json:"metadata"`
	}
}

// The channels
//
// swagger:response ChannelsResponse
type swaggerChannelsResponse struct {
	// in: body
	Body struct {
		swaggerSyncResponseBody
		Metadata []api.Channel `json:"metadata"`
	}
}

// The cluster
//
// swagger:response ClusterResponse
type swaggerClusterResponse struct {
	// in: body
	Body struct {
		swaggerSyncResponseBody
		Metadata api.Cluster `json:"metadata"`
	}
}

// The clusters
//
// swagger:response ClustersResponse
type swaggerClustersResponse struct {
	// in: body
	Body struct {
		swaggerSyncResponseBody
		Metadata []api.Cluster `json:"metadata"`
	}
}

// The cluster artifact
//
// swagger:response ClusterArtifactResponse
type swaggerClusterArtifactResponse struct {
	// in: body
	Body struct {
		swaggerSyncResponseBody
		Metadata api.ClusterArtifact `json:"metadata"`
	}
}

// The cluster artifacts
//
// swagger:response ClusterArtifactsResponse
type swaggerClusterArtifactsResponse struct {
	// in: body
	Body struct {
		swaggerSyncResponseBody
		Metadata []api.ClusterArtifact `json:"metadata"`
	}
}

// The cluster template
//
// swagger:response ClusterTemplateResponse
type swaggerClusterTemplateResponse struct {
	// in: body
	Body struct {
		swaggerSyncResponseBody
		Metadata api.ClusterTemplate `json:"metadata"`
	}
}

// The cluster templates
//
// swagger:response ClusterTemplatesResponse
type swaggerClusterTemplatesResponse struct {
	// in: body
	Body struct {
		swaggerSyncResponseBody
		Metadata []api.ClusterTemplate `json:"metadata"`
	}
}

// The image source
//
// swagger:response ImageSourceResponse
type swaggerImageSourceResponse struct {
	// in: body
	Body struct {
		swaggerSyncResponseBody
		Metadata api.ImageSource `json:"metadata"`
	}
}

// The image sources
//
// swagger:response ImageSourcesResponse
type swaggerImageSourcesResponse struct {
	// in: body
	Body struct {
		swaggerSyncResponseBody
		Metadata []api.ImageSource `json:"metadata"`
	}
}

// The Incus image
//
// swagger:response IncusImageResponse
type swaggerIncusImageResponse struct {
	// in: body
	Body struct {
		swaggerSyncResponseBody
		Metadata api.IncusImage `json:"metadata"`
	}
}

// The Incus images
//
// swagger:response IncusImagesResponse
type swaggerIncusImagesResponse struct {
	// in: body
	Body struct {
		swaggerSyncResponseBody
		Metadata []api.IncusImage `json:"metadata"`
	}
}

// The aggregated inventory resources
//
// swagger:response InventoryAggregatesResponse
type swaggerInventoryAggregatesResponse struct {
	// in: body
	Body struct {
		swaggerSyncResponseBody
		Metadata []api.InventoryAggregate `json:"metadata"`
	}
}

// The server
//
// swagger:response ServerResponse
type swaggerServerResponse struct {
	// in: body
	Body struct {
		swaggerSyncResponseBody
		Metadata api.Server `json:"metadata"`
	}
}

// The servers
//
// swagger:response ServersResponse
type swaggerServersResponse struct {
	// in: body
	Body struct {
		swaggerSyncResponseBody
		Metadata []api.Server `json:"metadata"`
	}
}

// The result of a server registration
//
// swagger:response ServerRegistrationResultResponse
type swaggerServerRegistrationResultResponse struct {
	// in: body
	Body struct {
		swaggerSyncResponseBody
		Metadata api.ServerRegistrationResponse `json:"metadata"`
	}
}

// List of BMC log sources
//
// swagger:response ServerBMCLogSourcesResponse
type swaggerServerBMCLogSourcesResponse struct {
	// in: body
	Body struct {
		swaggerSyncResponseBody
		Metadata []string `json:"metadata"`
	}
}

// The BMC log events
//
// swagger:response ServerBMCLogEventsResponse
type swaggerServerBMCLogEventsResponse struct {
	// in: body
	Body struct {
		swaggerSyncResponseBody
		Metadata []api.BMCLogEvent `json:"metadata"`
	}
}

// The BIOS attributes known to the BMC
//
// swagger:response ServerBMCBIOSAttributesResponse
type swaggerServerBMCBIOSAttributesResponse struct {
	// in: body
	Body struct {
		swaggerSyncResponseBody
		Metadata []api.BIOSAttribute `json:"metadata"`
	}
}

// The acceptable values of a BIOS attribute
//
// swagger:response ServerBMCBIOSAttributeResponse
type swaggerServerBMCBIOSAttributeValuesResponse struct {
	// in: body
	Body struct {
		swaggerSyncResponseBody
		Metadata api.BIOSAttribute `json:"metadata"`
	}
}

// The resolved BIOS profiles
//
// swagger:response BIOSProfileResolutionResponse
type swaggerBIOSProfileResolutionResponse struct {
	// in: body
	Body struct {
		swaggerSyncResponseBody
		Metadata api.BIOSProfileResolution `json:"metadata"`
	}
}

// The BMC dump
//
// swagger:response ServerBMCDumpResponse
type swaggerServerBMCDumpResponse struct {
	// in: body
	Body struct {
		swaggerSyncResponseBody
		Metadata api.BMCDump `json:"metadata"`
	}
}

// The network configuration of the server
//
// swagger:response ServerSystemNetworkResponse
type swaggerServerSystemNetworkResponse struct {
	// in: body
	Body struct {
		swaggerSyncResponseBody
		Metadata api.ServerSystemNetwork `json:"metadata"`
	}
}

// The storage configuration of the server
//
// swagger:response ServerSystemStorageResponse
type swaggerServerSystemStorageResponse struct {
	// in: body
	Body struct {
		swaggerSyncResponseBody
		Metadata api.ServerSystemStorage `json:"metadata"`
	}
}

// The update configuration of the server
//
// swagger:response ServerSystemUpdateResponse
type swaggerServerSystemUpdateResponse struct {
	// in: body
	Body struct {
		swaggerSyncResponseBody
		Metadata api.ServerSystemUpdate `json:"metadata"`
	}
}

// The token
//
// swagger:response TokenResponse
type swaggerTokenResponse struct {
	// in: body
	Body struct {
		swaggerSyncResponseBody
		Metadata api.Token `json:"metadata"`
	}
}

// The tokens
//
// swagger:response TokensResponse
type swaggerTokensResponse struct {
	// in: body
	Body struct {
		swaggerSyncResponseBody
		Metadata []api.Token `json:"metadata"`
	}
}

// The location of the generated image
//
// swagger:response TokenImageResponse
type swaggerTokenImageResponse struct {
	// in: body
	Body struct {
		swaggerSyncResponseBody
		Metadata struct {
			// Example: /1.0/provisioning/tokens/b32d0079-c48b-4957-b1cb-bef54125c861/image/9d73586d-2937-4e3a-8ed0-be999abe6387
			Image string `json:"image"`
		} `json:"metadata"`
	}
}

// The provider configuration of the token
//
// swagger:response TokenProviderConfigResponse
type swaggerTokenProviderConfigResponse struct {
	// in: body
	Body struct {
		swaggerSyncResponseBody
		Metadata api.TokenProviderConfig `json:"metadata"`
	}
}

// The token seed configurations
//
// swagger:response TokenSeedsResponse
type swaggerTokenSeedsResponse struct {
	// in: body
	Body struct {
		swaggerSyncResponseBody
		Metadata []api.TokenSeed `json:"metadata"`
	}
}

// The token seed configuration
//
// swagger:response TokenSeedResponse
type swaggerTokenSeedResponse struct {
	// in: body
	Body struct {
		swaggerSyncResponseBody
		Metadata api.TokenSeed `json:"metadata"`
	}
}

// The update
//
// swagger:response UpdateResponse
type swaggerUpdateResponse struct {
	// in: body
	Body struct {
		swaggerSyncResponseBody
		Metadata api.Update `json:"metadata"`
	}
}

// The updates
//
// swagger:response UpdatesResponse
type swaggerUpdatesResponse struct {
	// in: body
	Body struct {
		swaggerSyncResponseBody
		Metadata []api.Update `json:"metadata"`
	}
}

// The files of the update
//
// swagger:response UpdateFilesResponse
type swaggerUpdateFilesResponse struct {
	// in: body
	Body struct {
		swaggerSyncResponseBody
		Metadata []api.UpdateFile `json:"metadata"`
	}
}

// The changelog
//
// swagger:response UpdateChangelogResponse
type swaggerUpdateChangelogResponse struct {
	// in: body
	Body struct {
		swaggerSyncResponseBody
		Metadata api.UpdateChangelog `json:"metadata"`
	}
}

// The changelogs
//
// swagger:response UpdateChangelogsResponse
type swaggerUpdateChangelogsResponse struct {
	// in: body
	Body struct {
		swaggerSyncResponseBody
		Metadata api.UpdateChangelogs `json:"metadata"`
	}
}

// The certificate of Operations Center
//
// swagger:response SystemCertificateResponse
type swaggerSystemCertificateResponse struct {
	// in: body
	Body struct {
		swaggerSyncResponseBody
		Metadata apisystem.Certificate `json:"metadata"`
	}
}

// The network configuration of Operations Center
//
// swagger:response SystemNetworkResponse
type swaggerSystemNetworkResponse struct {
	// in: body
	Body struct {
		swaggerSyncResponseBody
		Metadata apisystem.Network `json:"metadata"`
	}
}

// The security configuration of Operations Center
//
// swagger:response SystemSecurityResponse
type swaggerSystemSecurityResponse struct {
	// in: body
	Body struct {
		swaggerSyncResponseBody
		Metadata apisystem.Security `json:"metadata"`
	}
}

// The settings of Operations Center
//
// swagger:response SystemSettingsResponse
type swaggerSystemSettingsResponse struct {
	// in: body
	Body struct {
		swaggerSyncResponseBody
		Metadata apisystem.Settings `json:"metadata"`
	}
}

// The updates configuration of Operations Center
//
// swagger:response SystemUpdatesResponse
type swaggerSystemUpdatesResponse struct {
	// in: body
	Body struct {
		swaggerSyncResponseBody
		Metadata apisystem.Updates `json:"metadata"`
	}
}

// The warning
//
// swagger:response WarningResponse
type swaggerWarningResponse struct {
	// in: body
	Body struct {
		swaggerSyncResponseBody
		Metadata api.Warning `json:"metadata"`
	}
}

// The warnings
//
// swagger:response WarningsResponse
type swaggerWarningsResponse struct {
	// in: body
	Body struct {
		swaggerSyncResponseBody
		Metadata []api.Warning `json:"metadata"`
	}
}

// Empty sync response
//
// swagger:response EmptySyncResponse
type swaggerEmptySyncResponse struct {
	// in: body
	Body struct {
		swaggerSyncResponseBody
	}
}

// Error responses

// swaggerErrorResponseBody holds the envelope every error response shares.
type swaggerErrorResponseBody struct {
	// Example: error
	Type string `json:"type"`

	// Example: bad request
	Status string `json:"status"`

	// Example: 400
	StatusCode int `json:"status_code"`
}

// Bad Request
//
// swagger:response BadRequest
type swaggerBadRequest struct {
	// in: body
	Body struct {
		swaggerErrorResponseBody
	}
}

// Forbidden
//
// swagger:response Forbidden
type swaggerForbidden struct {
	// in: body
	Body struct {
		swaggerErrorResponseBody
	}
}

// Precondition Failed
//
// swagger:response PreconditionFailed
type swaggerPreconditionFailed struct {
	// in: body
	Body struct {
		swaggerErrorResponseBody
	}
}

// Internal Server Error
//
// swagger:response InternalServerError
type swaggerInternalServerError struct {
	// in: body
	Body struct {
		swaggerErrorResponseBody
	}
}

// Not found
//
// swagger:response NotFound
type swaggerNotFound struct {
	// in: body
	Body struct {
		swaggerErrorResponseBody
	}
}
