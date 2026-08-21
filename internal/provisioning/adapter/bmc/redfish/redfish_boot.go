package redfish

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/stmcginnis/gofish/schemas"

	"github.com/FuturFusion/operations-center/internal/domain"
)

// bootSourceForVirtualMediaType maps the type of media a virtual media slot
// holds to the boot source the server boots that media from.
var bootSourceForVirtualMediaType = map[schemas.VirtualMediaType]schemas.BootSource{
	schemas.CDVirtualMediaType:       schemas.CdBootSource,
	schemas.DVDVirtualMediaType:      schemas.CdBootSource,
	schemas.USBStickVirtualMediaType: schemas.UsbBootSource,
	schemas.FloppyVirtualMediaType:   schemas.FloppyBootSource,
}

// bootSourcesForMediaTypes returns the boot sources able to boot the media types
// a virtual media slot reports.
func bootSourcesForMediaTypes(mediaTypes []schemas.VirtualMediaType) []schemas.BootSource {
	bootSources := make([]schemas.BootSource, 0, len(mediaTypes))

	for _, mediaType := range mediaTypes {
		bootSource, ok := bootSourceForVirtualMediaType[mediaType]
		if !ok || slices.Contains(bootSources, bootSource) {
			continue
		}

		bootSources = append(bootSources, bootSource)
	}

	return bootSources
}

// bootMediaTypes returns the media types to derive the boot source from: those
// of the image about to be attached, narrowed down to what the slot supports.
// Either side being unknown leaves the other one to decide on its own.
func bootMediaTypes(virtualMedia virtualMediaSlot, mediaURL string) []schemas.VirtualMediaType {
	imageMediaTypes := mediaTypesForURL(mediaURL)

	if len(imageMediaTypes) == 0 {
		return virtualMedia.MediaTypes
	}

	if len(virtualMedia.MediaTypes) == 0 {
		return imageMediaTypes
	}

	mediaTypes := make([]schemas.VirtualMediaType, 0, len(imageMediaTypes))

	for _, mediaType := range imageMediaTypes {
		if slices.Contains(virtualMedia.MediaTypes, mediaType) {
			mediaTypes = append(mediaTypes, mediaType)
		}
	}

	return mediaTypes
}

// bootSourceForVirtualMedia returns the boot source to point the server at in
// order to boot the media attached to the given virtual media slot.
func bootSourceForVirtualMedia(system *schemas.ComputerSystem, virtualMedia virtualMediaSlot, virtualMediaID string, mediaURL string) (schemas.BootSource, error) {
	bootSources := bootSourcesForMediaTypes(bootMediaTypes(virtualMedia, mediaURL))
	if len(bootSources) == 0 {
		return "", fmt.Errorf("Virtual media %q does not report a media type the server knows how to boot from: %w", virtualMediaID, domain.ErrOperationNotPermitted)
	}

	allowedTargets := system.Boot.AllowableBootSourceOverrideTargetValues
	if len(allowedTargets) == 0 {
		return bootSources[0], nil
	}

	for _, bootSource := range bootSources {
		if slices.Contains(allowedTargets, bootSource) {
			return bootSource, nil
		}
	}

	targets := make([]string, 0, len(allowedTargets))
	for _, target := range allowedTargets {
		targets = append(targets, string(target))
	}

	return "", fmt.Errorf("Server cannot be set to boot from virtual media %q, it boots from: %s: %w", virtualMediaID, strings.Join(targets, ", "), domain.ErrOperationNotPermitted)
}

// preferredBootSourceOverrideEnabled lists the ways of overriding the boot
// device, in descending order of preference.
var preferredBootSourceOverrideEnabled = []schemas.BootSourceOverrideEnabled{
	schemas.OnceBootSourceOverrideEnabled,
	schemas.ContinuousBootSourceOverrideEnabled,
}

// overrideBootDevice points the next boot of the server at bootSource.
func overrideBootDevice(system *schemas.ComputerSystem, registry *messageRegistry, bootSource schemas.BootSource) error {
	candidates, err := bootSourceOverrideEnabledCandidates(system)
	if err != nil {
		return err
	}

	for _, overrideEnabled := range candidates {
		err = setBoot(system, registry, &schemas.Boot{
			BootSourceOverrideTarget:  bootSource,
			BootSourceOverrideEnabled: overrideEnabled,
		})
		if err == nil {
			return nil
		}

		// Anything but the BMC turning the override mode down is final.
		if !isValueRejected(err, "BootSourceOverrideEnabled", string(overrideEnabled)) {
			break
		}
	}

	return err
}

// bootSourceOverrideEnabledCandidates returns the ways of overriding the boot
// device worth attempting, most preferred first.
func bootSourceOverrideEnabledCandidates(system *schemas.ComputerSystem) ([]schemas.BootSourceOverrideEnabled, error) {
	allowed := allowedBootSourceOverrideEnabled(system)
	if len(allowed) == 0 {
		return preferredBootSourceOverrideEnabled, nil
	}

	candidates := make([]schemas.BootSourceOverrideEnabled, 0, len(preferredBootSourceOverrideEnabled))

	for _, candidate := range preferredBootSourceOverrideEnabled {
		if slices.Contains(allowed, candidate) {
			candidates = append(candidates, candidate)
		}
	}

	if len(candidates) == 0 {
		values := make([]string, 0, len(allowed))
		for _, value := range allowed {
			values = append(values, string(value))
		}

		return nil, fmt.Errorf("BMC offers neither a one-time nor a continuous boot device override, it offers: %s: %w", strings.Join(values, ", "), domain.ErrOperationNotPermitted)
	}

	return candidates, nil
}

// allowedBootSourceOverrideEnabled returns the ways of overriding the boot
// device the BMC declares as allowable, or nil if it declares none.
func allowedBootSourceOverrideEnabled(system *schemas.ComputerSystem) []schemas.BootSourceOverrideEnabled {
	var resource struct {
		Boot struct {
			AllowableValues []schemas.BootSourceOverrideEnabled `json:"BootSourceOverrideEnabled@Redfish.AllowableValues"`
		} `json:"Boot"`
	}

	err := json.Unmarshal(system.RawData, &resource)
	if err != nil {
		return nil
	}

	return resource.Boot.AllowableValues
}

// restoreDefaultBootDevice puts the boot configuration of the server back to its
// default and reports whether it changed anything.
//
// Only an override pointing at one of the given boot sources is cleared,
// anything else was put in place for another purpose and is left alone.
func restoreDefaultBootDevice(system *schemas.ComputerSystem, registry *messageRegistry, bootSources []schemas.BootSource) (bool, error) {
	boot := system.Boot

	if boot.BootSourceOverrideEnabled == "" || boot.BootSourceOverrideEnabled == schemas.DisabledBootSourceOverrideEnabled {
		return false, nil
	}

	if !slices.Contains(bootSources, boot.BootSourceOverrideTarget) {
		return false, nil
	}

	err := setBoot(system, registry, &schemas.Boot{
		BootSourceOverrideTarget:  schemas.NoneBootSource,
		BootSourceOverrideEnabled: schemas.DisabledBootSourceOverrideEnabled,
	})
	if err == nil {
		return true, nil
	}

	if !isValueRejected(err, "BootSourceOverrideTarget", string(schemas.NoneBootSource)) {
		return false, err
	}

	err = setBoot(system, registry, &schemas.Boot{
		BootSourceOverrideEnabled: schemas.DisabledBootSourceOverrideEnabled,
	})
	if err != nil {
		return false, err
	}

	return true, nil
}

// setBoot applies a boot configuration to the system resource.
func setBoot(system *schemas.ComputerSystem, registry *messageRegistry, boot *schemas.Boot) error {
	err := system.SetBoot(boot)

	return redfishRequestError(err, registry, http.MethodPatch, system.ODataID, map[string]any{"Boot": boot})
}
