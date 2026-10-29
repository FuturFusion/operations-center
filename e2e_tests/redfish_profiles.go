package e2e

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/FuturFusion/operations-center/shared/api"
)

// redfishProfile describes a profile of the Redfish proxy and what the
// deployment of a server behind a proxy with this profile is expected to do.
type redfishProfile struct {
	name string

	// biosAttributes are added to the attributes of the BIOS profile handed to
	// the deployment.
	biosAttributes map[string]string

	// biosDeferredAttributes are the deferred attributes of the BIOS profile
	// handed to the deployment.
	biosDeferredAttributes map[string]string

	// deployFlags are additional flags for "server deploy".
	deployFlags []string

	// requiredStates must appear in the deployment history.
	requiredStates []api.ServerDeploymentState

	// forbiddenStates must not appear in the deployment history.
	forbiddenStates []api.ServerDeploymentState

	// expectSecureBoot is the expected value of security.secureboot on the instance.
	expectSecureBoot string

	// expectPartialMediaRead requires 0 < media_bytes_read < image size, so the
	// server has read the streamed media on demand. Unset requires the whole
	// image, since the proxy downloads it before the server boots.
	expectPartialMediaRead bool
}

// redfishProfileDefinitions holds the profiles of the Redfish proxy known to
// the end 2 end tests. The names are the names of the embedded profiles of the
// proxy.
var redfishProfileDefinitions = map[string]redfishProfile{
	"generic": {
		name: "generic",
		requiredStates: []api.ServerDeploymentState{
			api.ServerDeploymentStateApplyBIOS,
			api.ServerDeploymentStateSecureBoot,
			api.ServerDeploymentStateAttachMedia,
		},
		forbiddenStates: []api.ServerDeploymentState{
			api.ServerDeploymentStateApplyBIOSDeferred,
		},
		expectSecureBoot: "true",
	},
	"deferred-bios": {
		name: "deferred-bios",
		// ModeOption is only accepted, once Mode is Performance.
		biosAttributes: map[string]string{
			"Mode": "Performance",
		},
		biosDeferredAttributes: map[string]string{
			"ModeOption": "Enabled",
		},
		requiredStates: []api.ServerDeploymentState{
			api.ServerDeploymentStateApplyBIOS,
			api.ServerDeploymentStateApplyBIOSDeferred,
			api.ServerDeploymentStateVerifyBIOSDeferred,
		},
		expectSecureBoot: "true",
	},
	"no-secure-boot-write": {
		name:        "no-secure-boot-write",
		deployFlags: []string{"--secure-boot-enrollment-media"},
		requiredStates: []api.ServerDeploymentState{
			api.ServerDeploymentStateResetSecureBootKeys,
			api.ServerDeploymentStateAttachSecureBootMedia,
			api.ServerDeploymentStateWaitSecureBootEnrolled,
		},
		forbiddenStates: []api.ServerDeploymentState{
			api.ServerDeploymentStateSecureBoot,
		},
		expectSecureBoot: "true",
	},
	// The OEM insert shows in the BMC recording, not in the states.
	"oem-media": {
		name: "oem-media",
		requiredStates: []api.ServerDeploymentState{
			api.ServerDeploymentStateAttachMedia,
		},
		expectSecureBoot: "true",
	},
	// The proxy attaches the media as a FUSE file and fails the insert, if it
	// can not stream the media. The e2e host needs FUSE for this profile.
	"stream-media": {
		name: "stream-media",
		requiredStates: []api.ServerDeploymentState{
			api.ServerDeploymentStateAttachMedia,
			api.ServerDeploymentStateWaitInstall,
		},
		expectSecureBoot:       "true",
		expectPartialMediaRead: true,
	},
}

// redfishProfiles returns the profiles named in
// OPERATIONS_CENTER_E2E_TEST_REDFISH_PROFILES, in the given order.
func redfishProfiles(names string) ([]redfishProfile, error) {
	profiles := []redfishProfile{}
	unknown := []string{}

	for name := range strings.SplitSeq(names, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}

		profile, ok := redfishProfileDefinitions[name]
		if !ok {
			unknown = append(unknown, name)
			continue
		}

		profiles = append(profiles, profile)
	}

	if len(unknown) > 0 {
		return nil, fmt.Errorf("Unknown Redfish proxy profiles %q, known profiles are %q", unknown, knownRedfishProfileNames())
	}

	if len(profiles) == 0 {
		return nil, fmt.Errorf("No Redfish proxy profile given, known profiles are %q", knownRedfishProfileNames())
	}

	return profiles, nil
}

func knownRedfishProfileNames() []string {
	return slices.Sorted(maps.Keys(redfishProfileDefinitions))
}

// redfishProxyModel returns the model the Redfish proxy reports for a profile.
func redfishProxyModel(profileName string) string {
	return "Incus (" + profileName + ")"
}
