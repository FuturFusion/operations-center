package api

import (
	"fmt"
	"time"
)

// ServerDeploymentPost defines the request to deploy IncusOS on a server using
// the automated deployment control loop.
//
// swagger:model
type ServerDeploymentPost struct {
	// TokenUUID holds the UUID of the provisioning token that owns the seed the
	// installation media is generated from.
	// Example: 8f6c3d1a-2b4e-4c9a-9f7d-1a2b3c4d5e6f
	TokenUUID string `json:"token_uuid" yaml:"token_uuid"`

	// Seed holds the name of the token seed used to generate the installation
	// media. The referenced token seed must be public, since the BMC fetches
	// the image unauthenticated.
	// Example: some-seed-name
	Seed string `json:"seed" yaml:"seed"`

	// Type holds the type of image to generate. Possible values: iso, raw.
	// Optional, defaults to iso.
	// Example: iso
	Type string `json:"type" yaml:"type"`

	// Architecture holds the CPU architecture of the image to generate. Possible
	// values: x86_64, aarch64. Optional, defaults to x86_64.
	// Example: x86_64
	Architecture string `json:"architecture" yaml:"architecture"`

	// Channel holds the channel the most recent update should be taken from to
	// generate the image. Optional, defaults to the configured default channel.
	// Example: stable
	Channel string `json:"channel" yaml:"channel"`

	// VirtualMediaID identifies the virtual media device the installation media
	// is attached to, using the "<service>:<bmc-id>" notation (e.g. "system:1").
	// Optional, the first virtual media device advertising CD or DVD support is
	// picked automatically, if it is left empty, the ones offered by the system
	// taking precedence over the ones offered by the manager.
	// Example: system:1
	VirtualMediaID string `json:"virtual_media_id" yaml:"virtual_media_id"`

	// Force requests, that a token seed, which does not reboot the server upon
	// completion of the installation ("force_reboot"), is accepted. The
	// deployment then relies on the read progress of the installation media
	// alone to tell, when the first stage of the installation is done.
	// Example: false
	Force bool `json:"force" yaml:"force"`

	// SkipSecureBootCertificates requests, that the enrollment of the secure
	// boot certificates of IncusOS is skipped, which is required for a BMC,
	// whose Redfish API does not support the modification of the UEFI key
	// databases. The certificates are then expected to have been enrolled by an
	// operator before the deployment is triggered.
	// Example: false
	SkipSecureBootCertificates bool `json:"skip_secure_boot_certificates" yaml:"skip_secure_boot_certificates"`
}

// ServerDeploymentState is the state, the automated deployment of a server is in.
type ServerDeploymentState string

const (
	// ServerDeploymentStateRefreshBMCData collects the BMC data of the server,
	// so the deployment operates on an up to date view of the hardware.
	ServerDeploymentStateRefreshBMCData ServerDeploymentState = "refresh-bmc-data"

	// ServerDeploymentStateCheckBIOS reads the BIOS attributes of the server back
	// and records for both BIOS passes, whether they still have anything to
	// apply, so a server, that is configured correctly already, is not power
	// cycled for nothing.
	ServerDeploymentStateCheckBIOS ServerDeploymentState = "check-bios"

	// ServerDeploymentStatePowerOffBIOS powers the server off before the BIOS
	// attributes are applied.
	ServerDeploymentStatePowerOffBIOS ServerDeploymentState = "power-off-bios"

	// ServerDeploymentStateWaitPowerOffBIOS waits for the server to be powered off.
	ServerDeploymentStateWaitPowerOffBIOS ServerDeploymentState = "wait-power-off-bios"

	// ServerDeploymentStateApplyBIOS applies the resolved BIOS attributes.
	ServerDeploymentStateApplyBIOS ServerDeploymentState = "apply-bios"

	// ServerDeploymentStatePowerOnBIOS powers the server on, so the staged BIOS
	// attributes are picked up by the firmware.
	ServerDeploymentStatePowerOnBIOS ServerDeploymentState = "power-on-bios"

	// ServerDeploymentStateWaitBIOSApplied waits for the BIOS attributes to be applied.
	ServerDeploymentStateWaitBIOSApplied ServerDeploymentState = "wait-bios-applied"

	// ServerDeploymentStateVerifyBIOS reads the BIOS attributes back and
	// compares them to the resolved BIOS profile.
	ServerDeploymentStateVerifyBIOS ServerDeploymentState = "verify-bios"

	// ServerDeploymentStatePowerOffBIOSDeferred powers the server off before the
	// deferred BIOS attributes are applied.
	ServerDeploymentStatePowerOffBIOSDeferred ServerDeploymentState = "power-off-bios-deferred"

	// ServerDeploymentStateWaitPowerOffBIOSDeferred waits for the server to be powered off.
	ServerDeploymentStateWaitPowerOffBIOSDeferred ServerDeploymentState = "wait-power-off-bios-deferred"

	// ServerDeploymentStateApplyBIOSDeferred applies the resolved deferred BIOS
	// attributes, which only the firmware of a server, that has picked the
	// attributes applied before up, accepts.
	ServerDeploymentStateApplyBIOSDeferred ServerDeploymentState = "apply-bios-deferred"

	// ServerDeploymentStatePowerOnBIOSDeferred powers the server on, so the staged
	// deferred BIOS attributes are picked up by the firmware.
	ServerDeploymentStatePowerOnBIOSDeferred ServerDeploymentState = "power-on-bios-deferred"

	// ServerDeploymentStateWaitBIOSAppliedDeferred waits for the deferred BIOS
	// attributes to be applied.
	ServerDeploymentStateWaitBIOSAppliedDeferred ServerDeploymentState = "wait-bios-applied-deferred"

	// ServerDeploymentStateVerifyBIOSDeferred reads the deferred BIOS attributes
	// back and compares them to the resolved BIOS profile.
	ServerDeploymentStateVerifyBIOSDeferred ServerDeploymentState = "verify-bios-deferred"

	// ServerDeploymentStatePowerOffSecureBoot powers the server off, so the
	// secure boot databases can be reinitialized and the installation media can
	// be attached. The server stays off until it is booted from that media.
	ServerDeploymentStatePowerOffSecureBoot ServerDeploymentState = "power-off-secure-boot"

	// ServerDeploymentStateWaitPowerOffSecureBoot waits for the server to be powered off.
	ServerDeploymentStateWaitPowerOffSecureBoot ServerDeploymentState = "wait-power-off-secure-boot"

	// ServerDeploymentStateSecureBoot initializes the secure boot databases of
	// the server with the certificates of IncusOS.
	ServerDeploymentStateSecureBoot ServerDeploymentState = "secure-boot-certificates"

	// ServerDeploymentStateClearMedia ejects the media left in the virtual media
	// devices of the server.
	ServerDeploymentStateClearMedia ServerDeploymentState = "clear-media"

	// ServerDeploymentStateWaitMediaCleared waits for all the virtual media
	// devices to report no media inserted anymore.
	ServerDeploymentStateWaitMediaCleared ServerDeploymentState = "wait-media-cleared"

	// ServerDeploymentStatePowerOnSecureBoot powers the server on, so the
	// firmware picks the enrolled secure boot certificates up.
	ServerDeploymentStatePowerOnSecureBoot ServerDeploymentState = "power-on-secure-boot"

	// ServerDeploymentStateWaitSecureBootSettled waits for the firmware to have
	// picked the enrolled secure boot certificates up, which it signals by
	// rebooting the server on its own.
	ServerDeploymentStateWaitSecureBootSettled ServerDeploymentState = "wait-secure-boot-settled"

	// ServerDeploymentStatePowerOffSecureBootSettled powers the server off again,
	// so the installation media can be attached and booted.
	ServerDeploymentStatePowerOffSecureBootSettled ServerDeploymentState = "power-off-secure-boot-settled"

	// ServerDeploymentStateWaitPowerOffSecureBootSettled waits for the server to be powered off.
	ServerDeploymentStateWaitPowerOffSecureBootSettled ServerDeploymentState = "wait-power-off-secure-boot-settled"

	// ServerDeploymentStateAttachMedia attaches the installation media and
	// registers it as the boot device for the next boot.
	ServerDeploymentStateAttachMedia ServerDeploymentState = "attach-media"

	// ServerDeploymentStateWaitMediaAttached waits for the installation media to
	// be reported as inserted.
	ServerDeploymentStateWaitMediaAttached ServerDeploymentState = "wait-media-attached"

	// ServerDeploymentStatePowerOnInstall powers the server on, so it boots the
	// installation media.
	ServerDeploymentStatePowerOnInstall ServerDeploymentState = "power-on-install"

	// ServerDeploymentStateWaitInstall waits for the first stage of the IncusOS
	// installation to complete.
	ServerDeploymentStateWaitInstall ServerDeploymentState = "wait-install"

	// ServerDeploymentStateDetachMedia ejects the installation media, which also
	// restores the default boot device of the server.
	ServerDeploymentStateDetachMedia ServerDeploymentState = "detach-media"

	// ServerDeploymentStateWaitMediaDetached waits for the installation media to
	// be reported as ejected.
	ServerDeploymentStateWaitMediaDetached ServerDeploymentState = "wait-media-detached"

	// ServerDeploymentStateWaitReboot waits for the server to come back up after
	// the first stage of the installation.
	ServerDeploymentStateWaitReboot ServerDeploymentState = "wait-reboot"

	// ServerDeploymentStateWaitRegistration waits for the server to register
	// itself with Operations Center.
	ServerDeploymentStateWaitRegistration ServerDeploymentState = "wait-registration"

	// ServerDeploymentStateCleanup ejects any media still attached to the server.
	ServerDeploymentStateCleanup ServerDeploymentState = "cleanup"

	// ServerDeploymentStateCancel ejects the installation media and powers the
	// server off after the deployment has been cancelled.
	ServerDeploymentStateCancel ServerDeploymentState = "cancel"

	// ServerDeploymentStateWaitCancel waits for the server to be powered off.
	ServerDeploymentStateWaitCancel ServerDeploymentState = "wait-cancel"

	// ServerDeploymentStateCompleted is the terminal state of a successful deployment.
	ServerDeploymentStateCompleted ServerDeploymentState = "completed"

	// ServerDeploymentStateFailed is the terminal state of a failed deployment.
	// Nothing is cleaned up, so an operator can inspect the server through the BMC.
	ServerDeploymentStateFailed ServerDeploymentState = "failed"

	// ServerDeploymentStateCancelled is the terminal state of a cancelled deployment.
	ServerDeploymentStateCancelled ServerDeploymentState = "cancelled"
)

var serverDeploymentStates = map[ServerDeploymentState]struct{}{
	ServerDeploymentStateRefreshBMCData:                {},
	ServerDeploymentStateCheckBIOS:                     {},
	ServerDeploymentStatePowerOffBIOS:                  {},
	ServerDeploymentStateWaitPowerOffBIOS:              {},
	ServerDeploymentStateApplyBIOS:                     {},
	ServerDeploymentStatePowerOnBIOS:                   {},
	ServerDeploymentStateWaitBIOSApplied:               {},
	ServerDeploymentStateVerifyBIOS:                    {},
	ServerDeploymentStatePowerOffBIOSDeferred:          {},
	ServerDeploymentStateWaitPowerOffBIOSDeferred:      {},
	ServerDeploymentStateApplyBIOSDeferred:             {},
	ServerDeploymentStatePowerOnBIOSDeferred:           {},
	ServerDeploymentStateWaitBIOSAppliedDeferred:       {},
	ServerDeploymentStateVerifyBIOSDeferred:            {},
	ServerDeploymentStatePowerOffSecureBoot:            {},
	ServerDeploymentStateWaitPowerOffSecureBoot:        {},
	ServerDeploymentStateSecureBoot:                    {},
	ServerDeploymentStateClearMedia:                    {},
	ServerDeploymentStateWaitMediaCleared:              {},
	ServerDeploymentStatePowerOnSecureBoot:             {},
	ServerDeploymentStateWaitSecureBootSettled:         {},
	ServerDeploymentStatePowerOffSecureBootSettled:     {},
	ServerDeploymentStateWaitPowerOffSecureBootSettled: {},
	ServerDeploymentStateAttachMedia:                   {},
	ServerDeploymentStateWaitMediaAttached:             {},
	ServerDeploymentStatePowerOnInstall:                {},
	ServerDeploymentStateWaitInstall:                   {},
	ServerDeploymentStateDetachMedia:                   {},
	ServerDeploymentStateWaitMediaDetached:             {},
	ServerDeploymentStateWaitReboot:                    {},
	ServerDeploymentStateWaitRegistration:              {},
	ServerDeploymentStateCleanup:                       {},
	ServerDeploymentStateCancel:                        {},
	ServerDeploymentStateWaitCancel:                    {},
	ServerDeploymentStateCompleted:                     {},
	ServerDeploymentStateFailed:                        {},
	ServerDeploymentStateCancelled:                     {},
}

func (s ServerDeploymentState) String() string {
	return string(s)
}

// IsTerminal reports, if no further step is performed for a deployment in this state.
func (s ServerDeploymentState) IsTerminal() bool {
	switch s {
	case ServerDeploymentStateCompleted, ServerDeploymentStateFailed, ServerDeploymentStateCancelled:
		return true
	}

	return false
}

// MarshalText implements the encoding.TextMarshaler interface.
func (s ServerDeploymentState) MarshalText() ([]byte, error) {
	return []byte(s), nil
}

// UnmarshalText implements the encoding.TextUnmarshaler interface.
func (s *ServerDeploymentState) UnmarshalText(text []byte) error {
	if len(text) == 0 {
		*s = ""

		return nil
	}

	_, ok := serverDeploymentStates[ServerDeploymentState(text)]
	if !ok {
		return fmt.Errorf("%q is not a valid server deployment state", string(text))
	}

	*s = ServerDeploymentState(text)

	return nil
}

// ServerDeploymentStatus reports the progress of the automated deployment of a
// server. It is read only, deployments are triggered through the deploy and
// cancel-deploy endpoints.
//
// swagger:model
type ServerDeploymentStatus struct {
	// State holds the state of the deployment.
	// Example: wait-install
	State ServerDeploymentState `json:"state" yaml:"state"`

	// Request holds the deployment request, as it has been accepted.
	Request ServerDeploymentPost `json:"request" yaml:"request"`

	// ForceReboot reports, if the token seed used for the deployment reboots the
	// server on its own upon completion of the first stage of the installation.
	// Example: true
	ForceReboot bool `json:"force_reboot" yaml:"force_reboot"`

	// BIOSProfiles holds the names of the BIOS profiles, that have been resolved
	// for the server when the deployment was requested.
	// Example: ["dell-poweredge"]
	BIOSProfiles []string `json:"bios_profiles" yaml:"bios_profiles"`

	// BIOSAttributes holds the BIOS attributes, that have been resolved for the
	// server when the deployment was requested.
	BIOSAttributes map[string]any `json:"bios_attributes" yaml:"bios_attributes"`

	// BIOSDeferredAttributes holds the BIOS attributes, that have been resolved
	// for the server when the deployment was requested and that are applied in a
	// second pass, once the attributes above are in effect.
	BIOSDeferredAttributes map[string]any `json:"bios_deferred_attributes" yaml:"bios_deferred_attributes"`

	// SecureBoot holds the secure boot allow lists, that have been resolved for
	// the server when the deployment was requested. They name the entries of the
	// UEFI key databases, that are kept while the databases are reinitialized.
	SecureBoot BIOSSecureBoot `json:"secure_boot" yaml:"secure_boot"`

	// MediaURL holds the URL of the installation media attached to the server.
	MediaURL string `json:"media_url" yaml:"media_url"`

	// MediaBytesRead holds how much of the installation media the BMC has read so
	// far. It counts every byte of the image once, no matter how often the BMC
	// requested it, so a BMC re-requesting ranges it has fetched before does not
	// inflate it. It is -1, if no read progress is available.
	// Example: 966754304
	MediaBytesRead int64 `json:"media_bytes_read" yaml:"media_bytes_read"`

	// MediaSize holds the size of the installation media the bytes above are
	// read from. It is 0, if no read progress is available.
	// Example: 3433074688
	MediaSize int64 `json:"media_size" yaml:"media_size"`

	// Retries holds the number of attempts already spent on the current state.
	// Example: 0
	Retries int `json:"retries" yaml:"retries"`

	// LastError holds the error reported by the last failed attempt.
	LastError string `json:"last_error" yaml:"last_error"`

	// FailedState holds the state the deployment failed in.
	FailedState ServerDeploymentState `json:"failed_state" yaml:"failed_state"`

	// StartedAt is the time the deployment was requested in RFC3339 format.
	// Example: 2024-11-12T16:15:00Z
	StartedAt time.Time `json:"started_at" yaml:"started_at"`

	// StateEnteredAt is the time the current state was entered in RFC3339 format.
	// Example: 2024-11-12T16:15:00Z
	StateEnteredAt time.Time `json:"state_entered_at" yaml:"state_entered_at"`

	// FinishedAt is the time the deployment reached a terminal state in RFC3339
	// format. It is zero for a deployment still in progress.
	// Example: 2024-11-12T16:15:00Z
	FinishedAt time.Time `json:"finished_at" yaml:"finished_at"`

	// History holds the states the deployment has gone through.
	History []ServerDeploymentStep `json:"history" yaml:"history"`
}

// ServerDeploymentStep is a single state an automated deployment has gone through.
//
// swagger:model
type ServerDeploymentStep struct {
	// State holds the state of the deployment.
	// Example: attach-media
	State ServerDeploymentState `json:"state" yaml:"state"`

	// EnteredAt is the time the state was entered in RFC3339 format.
	// Example: 2024-11-12T16:15:00Z
	EnteredAt time.Time `json:"entered_at" yaml:"entered_at"`

	// Retries holds the number of attempts, that have been spent on the state.
	// Example: 1
	Retries int `json:"retries" yaml:"retries"`

	// Error holds the error reported by the last failed attempt on the state.
	Error string `json:"error" yaml:"error"`
}
