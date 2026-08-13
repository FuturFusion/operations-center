package redfish

import (
	"fmt"
	"net/url"
	"path"
	"slices"
	"strings"

	"github.com/stmcginnis/gofish/schemas"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/util/ptr"
)

// maxInsertMediaAttempts bounds the number of times insertMedia retries the
// InsertMedia action after adding a parameter the BMC reported as missing. One
// attempt per parameter insertMedia knows how to supply, plus the initial one.
const maxInsertMediaAttempts = 3

// insertMedia attaches mediaURL to a virtual media slot.
//
// The Redfish specification declares Image as the only required parameter of
// VirtualMedia.InsertMedia and has the service default Inserted and
// WriteProtected to true. Sending more might break interoperability.
//
// Start from the minimal request the specification asks for, add the parameters
// the BMC declares in the action info of InsertMedia, and add whatever it
// reports as missing after a rejected attempt.
func insertMedia(virtualMedia *schemas.VirtualMedia, mediaURL string) (*schemas.TaskMonitorInfo, error) {
	if !virtualMedia.SupportsMediaInsert {
		return nil, patchVirtualMedia(virtualMedia, ptr.To(mediaURL))
	}

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

	return nil, err
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

			params.TransferProtocolType = ptr.To(transferProtocolType)

		case "TransferMethod":
			// Streaming leaves the image where it is, uploading would copy it
			// into the BMC. Only ask for streaming, if the BMC offers it.
			if len(parameter.AllowableValues) > 0 && !slices.Contains(parameter.AllowableValues, string(schemas.StreamTransferMethod)) {
				continue
			}

			params.TransferMethod = ptr.To(schemas.StreamTransferMethod)
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
			params.TransferProtocolType = ptr.To(transferProtocolType)

			return true
		}
	}

	if params.TransferMethod == nil && isParameterMissing(err, "TransferMethod") {
		params.TransferMethod = ptr.To(schemas.StreamTransferMethod)

		return true
	}

	return false
}

// ejectMedia detaches the image from a virtual media slot.
func ejectMedia(virtualMedia *schemas.VirtualMedia) (*schemas.TaskMonitorInfo, error) {
	if !virtualMedia.SupportsMediaEject {
		return nil, patchVirtualMedia(virtualMedia, nil)
	}

	return virtualMedia.EjectMedia()
}

// patchVirtualMedia modifies the virtual media resource directly, for BMCs not
// exposing the InsertMedia and EjectMedia actions. Passing a nil image ejects.
func patchVirtualMedia(virtualMedia *schemas.VirtualMedia, image *string) error {
	payload := map[string]any{
		"Image":    image,
		"Inserted": image != nil,
	}

	err := virtualMedia.Patch(virtualMedia.ODataID, payload)
	if err == nil {
		return nil
	}

	if !isPropertyRejected(err, "Inserted") {
		return err
	}

	delete(payload, "Inserted")

	return virtualMedia.Patch(virtualMedia.ODataID, payload)
}

// virtualMediaHasMedia reports whether the virtual media slot currently holds
// an image.
func virtualMediaHasMedia(vm *schemas.VirtualMedia) bool {
	hasImage := vm.Image != "" || vm.ImageName != ""

	if vm.Inserted == nil {
		return hasImage
	}

	return *vm.Inserted && hasImage
}

// checkMediaTypeSupported verifies the virtual media slot accepts the kind of
// image about to be attached.
func checkMediaTypeSupported(virtualMedia *schemas.VirtualMedia, virtualMediaID string, mediaURL string) error {
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
