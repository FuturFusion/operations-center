package api

import (
	"fmt"
	"slices"
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

	// Architecture holds the CPU architecture of the images to generate, both
	// the installation media and, where it is used, the secure boot enrollment
	// media. Possible values: x86_64, aarch64. Optional, it is taken from what
	// the BMC reports about the server, when it is not provided, and is only
	// accepted where the BMC does not contradict it.
	// Example: x86_64
	Architecture string `json:"architecture" yaml:"architecture"`

	// Channel holds the channel the most recent update should be taken from to
	// generate the image. Optional, defaults to the configured default channel.
	// Example: stable
	Channel string `json:"channel" yaml:"channel"`

	// VirtualMediaID identifies the virtual media device the installation media
	// is attached to, using the "<service>:<bmc-id>" notation (e.g. "system:1").
	// Optional, the first virtual media device advertising support for the
	// requested image type is picked automatically, if it is left empty (CD or
	// DVD for an ISO image, USB stick or floppy for a raw one), the ones offered
	// by the system taking precedence over the ones offered by the manager.
	// Example: system:1
	VirtualMediaID string `json:"virtual_media_id" yaml:"virtual_media_id"`

	// Force requests, that a token seed, which does not reboot the server upon
	// completion of the installation ("force_reboot"), is accepted. The
	// deployment then relies on the read progress of the installation media
	// alone to tell, when the first stage of the installation is done, so it
	// needs a virtual media device, that streams the media rather than uploading
	// it before the server boots.
	// Example: false
	Force bool `json:"force" yaml:"force"`

	// SkipSecureBootCertificates requests, that the enrollment of the secure
	// boot certificates of IncusOS is skipped, which is required for a BMC,
	// whose Redfish API does not support the modification of the UEFI key
	// databases. The certificates are then expected to have been enrolled by an
	// operator before the deployment is triggered.
	// Example: false
	SkipSecureBootCertificates bool `json:"skip_secure_boot_certificates" yaml:"skip_secure_boot_certificates"`

	// SecureBootEnrollmentMedia requests, that the secure boot certificates of
	// IncusOS are enrolled by booting a generated enrollment media instead of
	// through the Redfish API. Operations Center resets the key databases of
	// the server first, which puts it into the secure boot setup mode the
	// enrollment media needs.
	// Example: false
	SecureBootEnrollmentMedia bool `json:"secure_boot_enrollment_media" yaml:"secure_boot_enrollment_media"`

	// BIOSProfiles holds the BIOS profiles the BIOS configuration of the server
	// is resolved from, by match and priority, instead of the BIOS profiles
	// shipped with Operations Center. The request is rejected, if none of them
	// matches the server. Optional, intended for the development of BIOS
	// profiles.
	BIOSProfiles []BIOSProfile `json:"bios_profiles,omitempty" yaml:"bios_profiles,omitempty"`

	// SecureBootCertificates holds additional PEM encoded secure boot
	// certificates, that the secure boot enrollment media enrolls, where the
	// provided BIOS profiles keep their SHA256 fingerprint in a secure boot
	// database. Optional, requires BIOS profiles and the secure boot
	// enrollment media.
	SecureBootCertificates []string `json:"secure_boot_certificates,omitempty" yaml:"secure_boot_certificates,omitempty"`
}

// SecureBootMediaPathSegments returns the path segments addressing one generated
// secure boot enrollment media, e.g. "1.0/provisioning/secure-boot-media/a1B2c3D4e5F6.iso".
//
// The extension names the image type, since it is what a BMC derives the kind of
// media to emulate from.
func SecureBootMediaPathSegments(imageType ImageType, mediaID string) []string {
	return []string{"1.0", "provisioning", "secure-boot-media", mediaID + imageType.FileExt()}
}

// ServerDeploymentCancelPost defines the request to cancel the automated
// deployment of a server.
//
// swagger:model
type ServerDeploymentCancelPost struct {
	// SkipCleanup requests, that the deployment is stopped without any clean up,
	// so the installation media is left attached and the server is left running,
	// the way a deployment, that failed on its own, is left untouched.
	// Example: false
	SkipCleanup bool `json:"skip_cleanup" yaml:"skip_cleanup"`
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

	// ServerDeploymentStateResetSecureBootKeys clears the UEFI key databases of
	// the server, which puts it into the secure boot setup mode the enrollment
	// media needs.
	ServerDeploymentStateResetSecureBootKeys ServerDeploymentState = "reset-secure-boot-keys"

	// ServerDeploymentStateWaitSecureBootReset waits for the BMC to be done
	// clearing the key databases, before the server is powered on again.
	ServerDeploymentStateWaitSecureBootReset ServerDeploymentState = "wait-secure-boot-reset"

	// ServerDeploymentStatePowerOnSecureBootReset powers the server on, so the
	// firmware picks the cleared key databases up.
	ServerDeploymentStatePowerOnSecureBootReset ServerDeploymentState = "power-on-secure-boot-reset"

	// ServerDeploymentStateWaitSecureBootSetupMode waits for the server to report
	// the secure boot setup mode.
	ServerDeploymentStateWaitSecureBootSetupMode ServerDeploymentState = "wait-secure-boot-setup-mode"

	// ServerDeploymentStatePowerOffSecureBootReset powers the server off again,
	// so the secure boot enrollment media can be attached.
	ServerDeploymentStatePowerOffSecureBootReset ServerDeploymentState = "power-off-secure-boot-reset"

	// ServerDeploymentStateWaitPowerOffSecureBootReset waits for the server to be powered off.
	ServerDeploymentStateWaitPowerOffSecureBootReset ServerDeploymentState = "wait-power-off-secure-boot-reset"

	// ServerDeploymentStateAttachSecureBootMedia attaches the secure boot
	// enrollment media and registers it as the boot device for the next boot.
	ServerDeploymentStateAttachSecureBootMedia ServerDeploymentState = "attach-secure-boot-media"

	// ServerDeploymentStateWaitSecureBootMediaAttached waits for the secure boot
	// enrollment media to be reported as inserted.
	ServerDeploymentStateWaitSecureBootMediaAttached ServerDeploymentState = "wait-secure-boot-media-attached"

	// ServerDeploymentStatePowerOnSecureBootMedia powers the server on, so it
	// boots the secure boot enrollment media.
	ServerDeploymentStatePowerOnSecureBootMedia ServerDeploymentState = "power-on-secure-boot-media"

	// ServerDeploymentStateWaitSecureBootEnrolled waits for the enrollment media
	// to have enrolled the certificates, which the server signals by leaving the
	// secure boot setup mode.
	ServerDeploymentStateWaitSecureBootEnrolled ServerDeploymentState = "wait-secure-boot-enrolled"

	// ServerDeploymentStatePowerOffSecureBootMedia powers the server off again,
	// so the enrollment media can be ejected and the installation media attached.
	ServerDeploymentStatePowerOffSecureBootMedia ServerDeploymentState = "power-off-secure-boot-media"

	// ServerDeploymentStateWaitPowerOffSecureBootMedia waits for the server to be powered off.
	ServerDeploymentStateWaitPowerOffSecureBootMedia ServerDeploymentState = "wait-power-off-secure-boot-media"

	// ServerDeploymentStateClearMedia ejects the media left in the virtual media
	// devices of the server.
	ServerDeploymentStateClearMedia ServerDeploymentState = "clear-media"

	// ServerDeploymentStateWaitMediaCleared waits for all the virtual media
	// devices to report no media inserted anymore.
	ServerDeploymentStateWaitMediaCleared ServerDeploymentState = "wait-media-cleared"

	// ServerDeploymentStateEnableSecureBoot switches secure boot on, which can
	// only be done once the certificates are enrolled: a server in the secure
	// boot setup mode with no certificates has nothing to enforce.
	ServerDeploymentStateEnableSecureBoot ServerDeploymentState = "enable-secure-boot"

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

	// ServerDeploymentStatePowerOnReboot powers the server on, if it stayed off
	// after the first stage of the installation.
	ServerDeploymentStatePowerOnReboot ServerDeploymentState = "power-on-reboot"

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
	ServerDeploymentStateResetSecureBootKeys:           {},
	ServerDeploymentStateWaitSecureBootReset:           {},
	ServerDeploymentStatePowerOnSecureBootReset:        {},
	ServerDeploymentStateWaitSecureBootSetupMode:       {},
	ServerDeploymentStatePowerOffSecureBootReset:       {},
	ServerDeploymentStateWaitPowerOffSecureBootReset:   {},
	ServerDeploymentStateAttachSecureBootMedia:         {},
	ServerDeploymentStateWaitSecureBootMediaAttached:   {},
	ServerDeploymentStatePowerOnSecureBootMedia:        {},
	ServerDeploymentStateWaitSecureBootEnrolled:        {},
	ServerDeploymentStatePowerOffSecureBootMedia:       {},
	ServerDeploymentStateWaitPowerOffSecureBootMedia:   {},
	ServerDeploymentStateClearMedia:                    {},
	ServerDeploymentStateWaitMediaCleared:              {},
	ServerDeploymentStateEnableSecureBoot:              {},
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
	ServerDeploymentStatePowerOnReboot:                 {},
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

// serverDeploymentTerminalStates are the states, in which no further step is
// performed for a deployment.
var serverDeploymentTerminalStates = []ServerDeploymentState{
	ServerDeploymentStateCompleted,
	ServerDeploymentStateFailed,
	ServerDeploymentStateCancelled,
}

// ServerDeploymentTerminalStates returns the states, in which no further step is
// performed for a deployment.
func ServerDeploymentTerminalStates() []ServerDeploymentState {
	return slices.Clone(serverDeploymentTerminalStates)
}

// IsTerminal reports, if no further step is performed for a deployment in this state.
func (s ServerDeploymentState) IsTerminal() bool {
	return slices.Contains(serverDeploymentTerminalStates, s)
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

	// DeploymentSettings holds the timings deviating from the defaults, that have
	// been resolved for the server when the deployment was requested.
	DeploymentSettings ServerDeploymentSettings `json:"deployment_settings,omitzero" yaml:"deployment_settings,omitempty"`

	// MediaURL holds the URL of the installation media attached to the server.
	MediaURL string `json:"media_url" yaml:"media_url"`

	// SecureBootMediaURL holds the URL of the secure boot enrollment media
	// attached to the server. It is empty for a deployment, that does not enroll
	// the secure boot certificates from an enrollment media.
	SecureBootMediaURL string `json:"secure_boot_media_url" yaml:"secure_boot_media_url"`

	// MediaBytesRead holds how much of the installation media the BMC has read so
	// far. It counts every byte of the image once, no matter how often the BMC
	// requested it, so a BMC re-requesting ranges it has fetched before does not
	// inflate it. It is -1, if no read progress is available.
	// Example: 966754304
	MediaBytesRead int64 `json:"media_bytes_read" yaml:"media_bytes_read"`

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

// ServerDeploymentSettings holds the timings of the automated deployment of a
// server, that deviate from the defaults. A setting, that is not set, leaves in
// place what the BIOS profiles with a lower priority have set, and finally the
// default. Durations are Go duration strings, e.g. "15m".
//
// swagger:model
type ServerDeploymentSettings struct {
	// DeploymentTimeout bounds the deployment as a whole. The provisioning token
	// has to stay valid for it.
	// Example: 2h
	DeploymentTimeout *Duration `json:"deployment_timeout,omitempty" yaml:"deployment_timeout,omitempty"`

	// StepTimeout bounds the waits for a power off, for the virtual media and for
	// the cancellation.
	// Example: 5m
	StepTimeout *Duration `json:"step_timeout,omitempty" yaml:"step_timeout,omitempty"`

	// BIOSAppliedTimeout bounds the waits for the firmware to apply BIOS and
	// secure boot changes.
	// Example: 10m
	BIOSAppliedTimeout *Duration `json:"bios_applied_timeout,omitempty" yaml:"bios_applied_timeout,omitempty"`

	// SecureBootEnrollTimeout bounds the wait for the enrollment media to enroll
	// the secure boot certificates.
	// Example: 15m
	SecureBootEnrollTimeout *Duration `json:"secure_boot_enroll_timeout,omitempty" yaml:"secure_boot_enroll_timeout,omitempty"`

	// InstallTimeout bounds the first stage of the installation.
	// Example: 45m
	InstallTimeout *Duration `json:"install_timeout,omitempty" yaml:"install_timeout,omitempty"`

	// RebootTimeout bounds the wait for the server to come back up after the first
	// stage of the installation.
	// Example: 15m
	RebootTimeout *Duration `json:"reboot_timeout,omitempty" yaml:"reboot_timeout,omitempty"`

	// RegistrationTimeout bounds the wait for the server to register itself.
	// Example: 30m
	RegistrationTimeout *Duration `json:"registration_timeout,omitempty" yaml:"registration_timeout,omitempty"`

	// PostSettleDelay is the time the server is granted to run through its power
	// on self test, before a wait trusts what the BMC reports.
	// Example: 1m
	PostSettleDelay *Duration `json:"post_settle_delay,omitempty" yaml:"post_settle_delay,omitempty"`

	// PowerOffSettleDelay is the time the server has to be reported powered off,
	// before the power off counts as settled.
	// Example: 1m
	PowerOffSettleDelay *Duration `json:"power_off_settle_delay,omitempty" yaml:"power_off_settle_delay,omitempty"`

	// RebootObservationWindow is the time the wait for the reboot looks for an
	// actual reboot, before it settles for the server being powered on.
	// Example: 5m
	RebootObservationWindow *Duration `json:"reboot_observation_window,omitempty" yaml:"reboot_observation_window,omitempty"`

	// SecureBootSettleDuration is the time the server is left running after the
	// secure boot certificates have been enrolled, where the firmware does not
	// reboot on its own.
	// Example: 5m
	SecureBootSettleDuration *Duration `json:"secure_boot_settle_duration,omitempty" yaml:"secure_boot_settle_duration,omitempty"`

	// InstallMinDuration is the time, that has to have passed, before the first
	// stage of the installation could be done at all.
	// Example: 5m
	InstallMinDuration *Duration `json:"install_min_duration,omitempty" yaml:"install_min_duration,omitempty"`

	// InstallRebootFallbackDelay is the time, after which a reboot counts as the
	// end of the first stage of the installation, where the BMC reports nothing
	// better.
	// Example: 10m
	InstallRebootFallbackDelay *Duration `json:"install_reboot_fallback_delay,omitempty" yaml:"install_reboot_fallback_delay,omitempty"`

	// InstallMediaIdlePeriod is the time without a read, after which the
	// installation media counts as idle.
	// Example: 2m
	InstallMediaIdlePeriod *Duration `json:"install_media_idle_period,omitempty" yaml:"install_media_idle_period,omitempty"`

	// InstallMediaMinBytesRead is the number of bytes of the installation media,
	// that have to have been read, before the idle period is taken as a signal.
	// Example: 524288000
	InstallMediaMinBytesRead *int64 `json:"install_media_min_bytes_read,omitempty" yaml:"install_media_min_bytes_read,omitempty"`

	// StepRetries is the number of retries granted to a single step, as well as
	// the number of fallbacks and reverts granted to a single wait.
	// Example: 3
	StepRetries *int `json:"step_retries,omitempty" yaml:"step_retries,omitempty"`

	// CallTimeout bounds the BMC operations of a single attempt of a step.
	// Example: 2m
	CallTimeout *Duration `json:"call_timeout,omitempty" yaml:"call_timeout,omitempty"`

	// AttachMediaCallTimeout bounds the BMC operations attaching a virtual media.
	// Example: 20m
	AttachMediaCallTimeout *Duration `json:"attach_media_call_timeout,omitempty" yaml:"attach_media_call_timeout,omitempty"`

	// SecureBootCallTimeout bounds the BMC operations changing the secure boot key
	// databases.
	// Example: 15m
	SecureBootCallTimeout *Duration `json:"secure_boot_call_timeout,omitempty" yaml:"secure_boot_call_timeout,omitempty"`
}

// IsZero reports, whether no setting is set.
func (s ServerDeploymentSettings) IsZero() bool {
	return s == ServerDeploymentSettings{}
}
