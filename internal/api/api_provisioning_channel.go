package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/FuturFusion/operations-center/internal/authz"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/transaction"
	"github.com/FuturFusion/operations-center/internal/util/response"
	"github.com/FuturFusion/operations-center/shared/api"
)

type channelsHandler struct {
	service provisioning.ChannelService
}

func registerChannelsHandler(router Router, authorizer *authz.Authorizer, service provisioning.ChannelService) {
	handler := &channelsHandler{
		service: service,
	}

	router.HandleFunc("GET /{$}", response.With(handler.channelsGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("POST /{$}", response.With(handler.channelsPost, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanCreate)))
	router.HandleFunc("GET /{name}", response.With(handler.channelGet, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanView)))
	router.HandleFunc("PUT /{name}", response.With(handler.channelPut, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanEdit)))
	router.HandleFunc("DELETE /{name}", response.With(handler.channelDelete, assertPermission(authorizer, authz.ObjectTypeServer, authz.EntitlementCanDelete)))
}

// swagger:operation GET /1.0/provisioning/channels channels channels_get
//
//	Get the channels
//
//	Returns a list of channels for updates (URLs).
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    description: API channels
//	    schema:
//	      type: object
//	      description: Sync response
//	      properties:
//	        type:
//	          type: string
//	          description: Response type
//	          example: sync
//	        status:
//	          type: string
//	          description: Status description
//	          example: Success
//	        status_code:
//	          type: integer
//	          description: Status code
//	          example: 200
//	        metadata:
//	          type: array
//	          description: List of channels
//	          items:
//	            type: string
//	          example: |-
//	            [
//	              "/1.0/provisioning/channels/one",
//	              "/1.0/provisioning/channels/two"
//	            ]
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"

// swagger:operation GET /1.0/provisioning/channels?recursion=1 channels channels_get_recursion
//
//	Get the channels
//
//	Returns a list of channels for updates (structs).
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    description: API channels
//	    schema:
//	      type: object
//	      description: Sync response
//	      properties:
//	        type:
//	          type: string
//	          description: Response type
//	          example: sync
//	        status:
//	          type: string
//	          description: Status description
//	          example: Success
//	        status_code:
//	          type: integer
//	          description: Status code
//	          example: 200
//	        metadata:
//	          type: array
//	          description: List of channels
//	          items:
//	            $ref: "#/definitions/Channel"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (u *channelsHandler) channelsGet(r *http.Request) response.Response {
	// Parse the recursion field.
	recursion, err := strconv.Atoi(r.FormValue("recursion"))
	if err != nil {
		recursion = 0
	}

	if recursion == 1 {
		channels, err := u.service.GetAll(r.Context())
		if err != nil {
			return response.SmartError(err)
		}

		result := make([]api.Channel, 0, len(channels))
		for _, channel := range channels {
			result = append(result, api.Channel{
				ChannelPost: api.ChannelPost{
					Name: channel.Name,
					ChannelPut: api.ChannelPut{
						Description: channel.Description,
					},
				},
				LastUpdated: channel.LastUpdated,
			})
		}

		return response.SyncResponse(true, result)
	}

	channelNames, err := u.service.GetAllNames(r.Context())
	if err != nil {
		return response.SmartError(err)
	}

	result := make([]string, 0, len(channelNames))
	for _, name := range channelNames {
		result = append(result, fmt.Sprintf("/%s/provisioning/channels/%s", api.APIVersion, name))
	}

	return response.SyncResponse(true, result)
}

// swagger:operation POST /1.0/provisioning/channels channels channels_post
//
//	Add an channel
//
//	Creates a new channel for updates.
//
//	---
//	consumes:
//	  - application/json
//	produces:
//	  - application/json
//	parameters:
//	  - in: body
//	    name: channel
//	    description: Channel configuration
//	    required: true
//	    schema:
//	      $ref: "#/definitions/ChannelPost"
//	responses:
//	  "200":
//	    $ref: "#/responses/EmptySyncResponse"
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (u *channelsHandler) channelsPost(r *http.Request) response.Response {
	var channel api.ChannelPost

	// Decode into the new channel.
	err := json.NewDecoder(r.Body).Decode(&channel)
	if err != nil {
		return response.BadRequest(err)
	}

	_, err = u.service.Create(r.Context(), provisioning.Channel{
		Name:        channel.Name,
		Description: channel.Description,
	})
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed creating channel: %w", err))
	}

	return response.SyncResponseLocation(true, nil, "/"+api.APIVersion+"/provisioning/channels/"+channel.Name)
}

// swagger:operation GET /1.0/provisioning/channels/{name} channels channel_get
//
//	Get the channel
//
//	Gets a specific channel for updates.
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    description: Channel
//	    schema:
//	      type: object
//	      description: Sync response
//	      properties:
//	        type:
//	          type: string
//	          description: Response type
//	          example: sync
//	        status:
//	          type: string
//	          description: Status description
//	          example: Success
//	        status_code:
//	          type: integer
//	          description: Status code
//	          example: 200
//	        metadata:
//	          $ref: "#/definitions/Channel"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (u *channelsHandler) channelGet(r *http.Request) response.Response {
	name := r.PathValue("name")

	channel, err := u.service.GetByName(r.Context(), name)
	if err != nil {
		return response.SmartError(err)
	}

	return response.SyncResponseETag(
		true,
		api.Channel{
			ChannelPost: api.ChannelPost{
				Name: channel.Name,
				ChannelPut: api.ChannelPut{
					Description: channel.Description,
				},
			},
			LastUpdated: channel.LastUpdated,
		},
		channel,
	)
}

// swagger:operation PUT /1.0/provisioning/channels/{name} channels channel_put
//
//	Update the channel
//
//	Updates the channel definition.
//
//	---
//	consumes:
//	  - application/json
//	produces:
//	  - application/json
//	parameters:
//	  - in: body
//	    name: channel
//	    description: Channel definition
//	    required: true
//	    schema:
//	      $ref: "#/definitions/ChannelPut"
//	responses:
//	  "200":
//	    $ref: "#/responses/EmptySyncResponse"
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "412":
//	    $ref: "#/responses/PreconditionFailed"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (u *channelsHandler) channelPut(r *http.Request) response.Response {
	name := r.PathValue("name")

	var channel api.ChannelPut

	err := json.NewDecoder(r.Body).Decode(&channel)
	if err != nil {
		return response.BadRequest(err)
	}

	ctx, trans := transaction.Begin(r.Context())
	defer func() {
		rollbackErr := trans.Rollback()
		if rollbackErr != nil {
			response.SmartError(fmt.Errorf("Transaction rollback failed: %v, reason: %w", rollbackErr, err))
		}
	}()

	currentChannel, err := u.service.GetByName(ctx, name)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed to get channel %q: %w", name, err))
	}

	// Validate ETag
	err = response.EtagCheck(r, currentChannel)
	if err != nil {
		return response.PreconditionFailed(err)
	}

	currentChannel.Description = channel.Description

	err = u.service.Update(ctx, *currentChannel)
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed updating channel %q: %w", name, err))
	}

	err = trans.Commit()
	if err != nil {
		return response.SmartError(fmt.Errorf("Failed commit transaction: %w", err))
	}

	return response.SyncResponseLocation(true, nil, "/"+api.APIVersion+"/provisioning/channels/"+name)
}

// swagger:operation DELETE /1.0/provisioning/channels/{name} channels channel_delete
//
//	Delete the channel
//
//	Removes the channel.
//
//	---
//	produces:
//	  - application/json
//	responses:
//	  "200":
//	    $ref: "#/responses/EmptySyncResponse"
//	  "400":
//	    $ref: "#/responses/BadRequest"
//	  "403":
//	    $ref: "#/responses/Forbidden"
//	  "500":
//	    $ref: "#/responses/InternalServerError"
func (u *channelsHandler) channelDelete(r *http.Request) response.Response {
	name := r.PathValue("name")

	err := u.service.DeleteByName(r.Context(), name)
	if err != nil {
		return response.SmartError(err)
	}

	return response.EmptySyncResponse
}
