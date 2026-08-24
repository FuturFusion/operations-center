package redfish

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strings"

	"github.com/stmcginnis/gofish/schemas"

	"github.com/FuturFusion/operations-center/internal/domain"
)

// maxInsertMediaAttempts bounds the number of times insertMedia retries the
// InsertMedia action after adding a parameter the BMC reported as missing. One
// attempt per parameter insertMedia knows how to supply, plus the initial one.
const maxInsertMediaAttempts = 3

// insertMedia attaches mediaURL to a virtual media slot.
//
// Three ways of doing so exist, in descending order of preference: the standard
// VirtualMedia.InsertMedia action, a vendor specific action for BMCs not
// offering the standard one, and modifying the resource directly for BMCs
// offering no action at all.
func insertMedia(virtualMedia virtualMediaSlot, mediaURL string) (*schemas.TaskMonitorInfo, error) {
	if virtualMedia.SupportsMediaInsert {
		return insertMediaAction(virtualMedia, mediaURL)
	}

	target := oemActionTarget(virtualMedia, "InsertVirtualMedia")
	if target != "" {
		// Vendor specific actions predate the properties the standard action
		// takes besides the image, so the image is all which is sent.
		return postVirtualMediaAction(virtualMedia, target, map[string]any{"Image": mediaURL})
	}

	return nil, patchVirtualMedia(virtualMedia, new(mediaURL))
}

// insertMediaAction attaches mediaURL using the standard
// VirtualMedia.InsertMedia action.
//
// The Redfish specification declares Image as the only required parameter of
// the action and has the service default Inserted and WriteProtected to true.
// Sending more might break interoperability.
//
// Start from the minimal request the specification asks for, add the parameters
// the BMC declares in the action info of InsertMedia, and add whatever it
// reports as missing after a rejected attempt.
func insertMediaAction(virtualMedia virtualMediaSlot, mediaURL string) (*schemas.TaskMonitorInfo, error) {
	params := &schemas.VirtualMediaInsertMediaParameters{
		Image: mediaURL,
	}

	// The action info is optional, BMCs not offering one simply leave the
	// request minimal.
	actionInfo, err := virtualMedia.InsertMediaActionInfo()
	if err == nil {
		err = applyInsertMediaActionInfo(params, actionInfo, mediaURL)
		if err != nil {
			return nil, err
		}
	}

	var taskMonitor *schemas.TaskMonitorInfo

	for range maxInsertMediaAttempts {
		taskMonitor, err = virtualMedia.InsertMedia(params)
		if err == nil {
			return taskMonitor, nil
		}

		if !addMissingInsertMediaParameter(params, mediaURL, err) {
			break
		}
	}

	return nil, redfishRequestError(err, virtualMedia.registry, http.MethodPost, standardActionTarget(virtualMedia, "VirtualMedia.InsertMedia"), params)
}

// applyInsertMediaActionInfo adds the parameters the BMC declares for
// InsertMedia to the request.
func applyInsertMediaActionInfo(params *schemas.VirtualMediaInsertMediaParameters, actionInfo *schemas.ActionInfo, mediaURL string) error {
	for _, parameter := range actionInfo.Parameters {
		switch parameter.Name {
		case "TransferProtocolType":
			transferProtocolType := transferProtocolTypeForURL(mediaURL)
			if transferProtocolType == "" {
				continue
			}

			if len(parameter.AllowableValues) > 0 && !slices.Contains(parameter.AllowableValues, string(transferProtocolType)) {
				return fmt.Errorf("BMC does not support transfer protocol %q for virtual media, supported protocols are: %s: %w", transferProtocolType, strings.Join(parameter.AllowableValues, ", "), domain.ErrOperationNotPermitted)
			}

			params.TransferProtocolType = new(transferProtocolType)

		case "TransferMethod":
			// Streaming leaves the image where it is, uploading would copy it
			// into the BMC. Only ask for streaming, if the BMC offers it.
			if len(parameter.AllowableValues) > 0 && !slices.Contains(parameter.AllowableValues, string(schemas.StreamTransferMethod)) {
				continue
			}

			params.TransferMethod = new(schemas.StreamTransferMethod)
		}
	}

	return nil
}

// addMissingInsertMediaParameter adds a parameter the BMC reported as missing
// to the request and reports whether the request is worth retrying.
func addMissingInsertMediaParameter(params *schemas.VirtualMediaInsertMediaParameters, mediaURL string, err error) bool {
	if params.TransferProtocolType == nil && isParameterMissing(err, "TransferProtocolType") {
		transferProtocolType := transferProtocolTypeForURL(mediaURL)
		if transferProtocolType != "" {
			params.TransferProtocolType = new(transferProtocolType)

			return true
		}
	}

	if params.TransferMethod == nil && isParameterMissing(err, "TransferMethod") {
		params.TransferMethod = new(schemas.StreamTransferMethod)

		return true
	}

	return false
}

// ejectMedia detaches the image from a virtual media slot, using the same three
// ways of doing so as insertMedia.
func ejectMedia(virtualMedia virtualMediaSlot) (*schemas.TaskMonitorInfo, error) {
	if virtualMedia.SupportsMediaEject {
		return virtualMedia.EjectMedia()
	}

	target := oemActionTarget(virtualMedia, "EjectVirtualMedia")
	if target != "" {
		return postVirtualMediaAction(virtualMedia, target, map[string]any{})
	}

	return nil, patchVirtualMedia(virtualMedia, nil)
}

// standardActionTarget returns the target URI of the standard action with the
// given name, e.g. "VirtualMedia.InsertMedia".
func standardActionTarget(virtualMedia virtualMediaSlot, action string) string {
	var resource struct {
		Actions map[string]struct {
			Target string `json:"target"`
		} `json:"Actions"`
	}

	err := json.Unmarshal(virtualMedia.RawData, &resource)
	if err != nil {
		return ""
	}

	return resource.Actions["#"+action].Target
}

// maxOEMActionDepth bounds how deeply oemActionTarget descends into the "Oem"
// object of a resource.
const maxOEMActionDepth = 3

// oemActionTarget returns the target URI of the vendor specific action with the
// given name offered by the virtual media resource, or an empty string if the
// BMC offers none.
func oemActionTarget(virtualMedia virtualMediaSlot, action string) string {
	var resource struct {
		Actions struct {
			OEM map[string]any `json:"Oem"`
		} `json:"Actions"`
	}

	err := json.Unmarshal(virtualMedia.RawData, &resource)
	if err != nil {
		return ""
	}

	return findOEMActionTarget(resource.Actions.OEM, action, maxOEMActionDepth)
}

// findOEMActionTarget searches an "Oem" object for the action with the given
// name, descending into the vendor groupings the action may be nested in.
func findOEMActionTarget(oem map[string]any, action string, depth int) string {
	if depth <= 0 {
		return ""
	}

	for _, name := range slices.Sorted(maps.Keys(oem)) {
		member, ok := oem[name].(map[string]any)
		if !ok {
			continue
		}

		// Actions are named "#<Type>.<Action>", anything else is a grouping.
		if !strings.HasPrefix(name, "#") {
			target := findOEMActionTarget(member, action, depth-1)
			if target != "" {
				return target
			}

			continue
		}

		_, actionName, found := lastCut(name, ".")
		if !found || actionName != action {
			continue
		}

		target, ok := member["target"].(string)
		if ok && target != "" {
			return target
		}
	}

	return ""
}

// postVirtualMediaAction invokes an action of the virtual media resource.
func postVirtualMediaAction(virtualMedia virtualMediaSlot, target string, payload any) (*schemas.TaskMonitorInfo, error) {
	resp, taskMonitor, err := schemas.PostWithTask(virtualMedia.GetClient(), target, payload, virtualMedia.Headers(), false) //nolint:bodyclose // Closed by DeferredCleanupHTTPResponse.
	defer schemas.DeferredCleanupHTTPResponse(resp)

	if err != nil {
		return nil, redfishRequestError(err, virtualMedia.registry, http.MethodPost, target, payload)
	}

	return taskMonitor, nil
}

// maxPatchVirtualMediaAttempts bounds the number of times patchVirtualMedia
// repeats the request after dropping a property the BMC rejected. One attempt
// per property it is allowed to drop, plus the initial one.
const maxPatchVirtualMediaAttempts = 3

// patchVirtualMedia modifies the virtual media resource directly, for BMCs
// exposing neither a standard nor a vendor specific action. Passing a nil image
// ejects.
func patchVirtualMedia(virtualMedia virtualMediaSlot, image *string) error {
	payload := map[string]any{
		"Image":    image,
		"Inserted": image != nil,
	}

	// Only meaningful while media is attached.
	if image != nil {
		payload["WriteProtected"] = true
	}

	var err error

	for range maxPatchVirtualMediaAttempts {
		err = patchVirtualMediaResource(virtualMedia, payload)
		if err == nil {
			return nil
		}

		if !dropRejectedProperty(payload, err) {
			break
		}
	}

	return err
}

// dropRejectedProperty removes the property the BMC reported as unknown or not
// writable from the payload and reports whether the request is worth repeating.
//
// Image is never dropped, sending it is the whole point of the request.
func dropRejectedProperty(payload map[string]any, err error) bool {
	for _, property := range []string{"Inserted", "WriteProtected"} {
		_, present := payload[property]
		if present && isPropertyRejected(err, property) {
			delete(payload, property)

			return true
		}
	}

	return false
}

// patchVirtualMediaResource sends the payload to the virtual media resource.
func patchVirtualMediaResource(virtualMedia virtualMediaSlot, payload any) error {
	headers := virtualMedia.Headers()

	_, conditional := headers["If-Match"]

	err := patchWithHeaders(virtualMedia, payload, headers)
	if err == nil || !conditional || !isPreconditionRejected(err) {
		return err
	}

	delete(headers, "If-Match")

	return patchWithHeaders(virtualMedia, payload, headers)
}

func patchWithHeaders(virtualMedia virtualMediaSlot, payload any, headers map[string]string) error {
	// gofish consumes and closes the response of a request it reports an error
	// for, so there is only something left to clean up on success.
	resp, err := virtualMedia.GetClient().PatchWithHeaders(virtualMedia.ODataID, payload, headers) //nolint:bodyclose // Closed by CleanupHTTPResponse.
	if err == nil {
		err = schemas.CleanupHTTPResponse(resp)
	}

	return redfishRequestError(err, virtualMedia.registry, http.MethodPatch, virtualMedia.ODataID, payload)
}

// virtualMediaHasMedia reports whether the virtual media slot currently holds
// an image.
func virtualMediaHasMedia(vm virtualMediaSlot) bool {
	hasImage := vm.Image != "" || vm.ImageName != ""

	if vm.Inserted == nil {
		return hasImage
	}

	return *vm.Inserted && hasImage
}

// checkMediaTypeSupported verifies the virtual media slot accepts the kind of
// image about to be attached.
func checkMediaTypeSupported(virtualMedia virtualMediaSlot, virtualMediaID string, mediaURL string) error {
	mediaTypes := mediaTypesForURL(mediaURL)
	if len(mediaTypes) == 0 || len(virtualMedia.MediaTypes) == 0 {
		return nil
	}

	for _, supported := range virtualMedia.MediaTypes {
		if slices.Contains(mediaTypes, supported) {
			return nil
		}
	}

	supported := make([]string, 0, len(virtualMedia.MediaTypes))
	for _, mediaType := range virtualMedia.MediaTypes {
		supported = append(supported, string(mediaType))
	}

	return fmt.Errorf("Virtual media %q does not accept the media, it supports %s: %w", virtualMediaID, strings.Join(supported, ", "), domain.ErrOperationNotPermitted)
}

// mediaTypesForURL returns the virtual media types able to hold the image the
// URL points at, derived from its file extension. An unknown extension yields
// no types, leaving the decision to the BMC.
func mediaTypesForURL(mediaURL string) []schemas.VirtualMediaType {
	parsedURL, err := url.Parse(mediaURL)
	if err != nil {
		return nil
	}

	switch strings.ToLower(path.Ext(parsedURL.Path)) {
	case ".iso":
		return []schemas.VirtualMediaType{schemas.CDVirtualMediaType, schemas.DVDVirtualMediaType}

	case ".img", ".raw", ".bin":
		return []schemas.VirtualMediaType{schemas.USBStickVirtualMediaType, schemas.FloppyVirtualMediaType}
	}

	return nil
}

// transferProtocolTypeForURL returns the Redfish transfer protocol matching the
// scheme of the image URL, or an empty string for a scheme without one.
func transferProtocolTypeForURL(mediaURL string) schemas.TransferProtocolType {
	parsedURL, err := url.Parse(mediaURL)
	if err != nil {
		return ""
	}

	switch strings.ToLower(parsedURL.Scheme) {
	case "https":
		return schemas.HTTPSTransferProtocolType

	case "http":
		return schemas.HTTPTransferProtocolType
	}

	return ""
}
