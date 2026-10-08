package provisioning

import (
	"bytes"
	"encoding/asn1"
	"encoding/pem"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lxc/incus-os/incus-osd/api/images"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/util/certificate"
	"github.com/FuturFusion/operations-center/shared/api"
)

// serverDeploymentHistoryLimit bounds the number of steps kept on a deployment,
// so a deployment retrying forever can not grow the server record without end.
const serverDeploymentHistoryLimit = 200

// serverDeploymentSecureBootCertificatesLimit bounds the additional secure boot certificates of a deployment request.
const serverDeploymentSecureBootCertificatesLimit = 16

// ServerDeploymentRequest is what an operator asked for, when the automated
// deployment of a server was triggered.
type ServerDeploymentRequest struct {
	TokenUUID                  uuid.UUID                     `json:"token_uuid"`
	Seed                       string                        `json:"seed"`
	ImageType                  api.ImageType                 `json:"image_type"`
	Architecture               images.UpdateFileArchitecture `json:"architecture"`
	Channel                    string                        `json:"channel"`
	VirtualMediaID             string                        `json:"virtual_media_id"`
	Force                      bool                          `json:"force"`
	SkipSecureBootCertificates bool                          `json:"skip_secure_boot_certificates"`
	SecureBootEnrollmentMedia  bool                          `json:"secure_boot_enrollment_media"`
	BIOSProfiles               BIOSProfiles                  `json:"-"`
	SecureBootCertificates     []string                      `json:"-"`
}

func (r ServerDeploymentRequest) Validate() error {
	if r.TokenUUID == uuid.Nil {
		return domain.NewValidationErrf("Invalid deployment request, token UUID can not be empty")
	}

	if r.Seed == "" {
		return domain.NewValidationErrf("Invalid deployment request, token seed can not be empty")
	}

	if !r.ImageType.IsValid() {
		return domain.NewValidationErrf("Invalid deployment request, image type %q is not valid", r.ImageType)
	}

	_, ok := images.UpdateFileArchitectures[r.Architecture]
	if !ok {
		return domain.NewValidationErrf("Invalid deployment request, architecture %q is not valid", r.Architecture)
	}

	if r.SkipSecureBootCertificates && r.SecureBootEnrollmentMedia {
		return domain.NewValidationErrf("Invalid deployment request, the secure boot certificates can not be enrolled from an enrollment media and be skipped at the same time")
	}

	err := r.BIOSProfiles.ValidateForDeployment()
	if err != nil {
		return err
	}

	_, err = r.SecureBootCertificatesByFingerprint()
	if err != nil {
		return err
	}

	return nil
}

// SecureBootCertificatesByFingerprint returns the additional secure boot certificates keyed by their SHA256 fingerprint.
func (r ServerDeploymentRequest) SecureBootCertificatesByFingerprint() (map[string]string, error) {
	if len(r.SecureBootCertificates) == 0 {
		return nil, nil
	}

	if len(r.BIOSProfiles) == 0 {
		return nil, domain.NewValidationErrf("Invalid deployment request, secure boot certificates can only be provided together with BIOS profiles")
	}

	if !r.SecureBootEnrollmentMedia {
		return nil, domain.NewValidationErrf("Invalid deployment request, secure boot certificates can only be enrolled from the secure boot enrollment media")
	}

	if len(r.SecureBootCertificates) > serverDeploymentSecureBootCertificatesLimit {
		return nil, domain.NewValidationErrf("Invalid deployment request, at most %d secure boot certificates can be provided", serverDeploymentSecureBootCertificatesLimit)
	}

	kept := map[string]bool{}
	for _, profile := range r.BIOSProfiles {
		for _, database := range []BIOSSecureBootDatabase{profile.SecureBoot.DB, profile.SecureBoot.DBX, profile.SecureBoot.KEK} {
			for fingerprint, keep := range database.Certificates {
				if keep != nil && *keep {
					kept[strings.ToLower(strings.TrimSpace(fingerprint))] = true
				}
			}
		}
	}

	certificates := make(map[string]string, len(r.SecureBootCertificates))
	for i, pemCertificate := range r.SecureBootCertificates {
		block, rest := pem.Decode([]byte(pemCertificate))
		if block == nil || block.Type != "CERTIFICATE" || len(bytes.TrimSpace(rest)) > 0 {
			return nil, domain.NewValidationErrf("Invalid deployment request, secure boot certificate %d has to hold exactly one PEM encoded certificate", i+1)
		}

		// The DER is not parsed as a certificate, see certificate.DERFingerprint.
		var der asn1.RawValue

		rest, err := asn1.Unmarshal(block.Bytes, &der)
		if err != nil || len(rest) > 0 || der.Tag != asn1.TagSequence {
			return nil, domain.NewValidationErrf("Invalid deployment request, secure boot certificate %d is not DER encoded", i+1)
		}

		pemCertificate = certificate.EncodeToPEM(block.Bytes)

		fingerprint, err := certificate.DERFingerprint(strconv.Itoa(i+1), []byte(pemCertificate))
		if err != nil {
			return nil, domain.NewValidationErrf("Invalid deployment request, secure boot certificate %d is not valid: %v", i+1, err)
		}

		_, ok := certificates[fingerprint]
		if ok {
			return nil, domain.NewValidationErrf("Invalid deployment request, secure boot certificate %q is provided more than once", fingerprint)
		}

		if !kept[fingerprint] {
			return nil, domain.NewValidationErrf("Invalid deployment request, secure boot certificate %q is not kept by any of the provided BIOS profiles", fingerprint)
		}

		certificates[fingerprint] = pemCertificate
	}

	return certificates, nil
}

// NewServerDeploymentRequest builds a deployment request from its API
// representation, applying the defaults for the optional fields.
func NewServerDeploymentRequest(request api.ServerDeploymentPost) (ServerDeploymentRequest, error) {
	tokenUUID, err := uuid.Parse(request.TokenUUID)
	if err != nil {
		return ServerDeploymentRequest{}, domain.NewValidationErrf("Invalid deployment request, token UUID %q is not valid: %v", request.TokenUUID, err)
	}

	imageType := api.ImageType(request.Type)
	if request.Type == "" {
		imageType = api.ImageTypeISO
	}

	architecture := images.UpdateFileArchitecture(request.Architecture)

	var biosProfiles BIOSProfiles
	for _, profile := range request.BIOSProfiles {
		biosProfiles = append(biosProfiles, NewBIOSProfileFromAPI(profile))
	}

	return ServerDeploymentRequest{
		BIOSProfiles:               biosProfiles,
		SecureBootCertificates:     slices.Clone(request.SecureBootCertificates),
		TokenUUID:                  tokenUUID,
		Seed:                       request.Seed,
		ImageType:                  imageType,
		Architecture:               architecture,
		Channel:                    request.Channel,
		VirtualMediaID:             request.VirtualMediaID,
		Force:                      request.Force,
		SkipSecureBootCertificates: request.SkipSecureBootCertificates,
		SecureBootEnrollmentMedia:  request.SecureBootEnrollmentMedia,
	}, nil
}

func (r ServerDeploymentRequest) ToAPI() api.ServerDeploymentPost {
	return api.ServerDeploymentPost{
		TokenUUID:                  r.TokenUUID.String(),
		Seed:                       r.Seed,
		Type:                       r.ImageType.String(),
		Architecture:               r.Architecture.String(),
		Channel:                    r.Channel,
		VirtualMediaID:             r.VirtualMediaID,
		Force:                      r.Force,
		SkipSecureBootCertificates: r.SkipSecureBootCertificates,
		SecureBootEnrollmentMedia:  r.SecureBootEnrollmentMedia,
	}
}

// ServerDeploymentBMCSnapshot holds the properties of the BMC data, that tell
// whether a server has rebooted, as they were observed at a given point in time.
type ServerDeploymentBMCSnapshot struct {
	Taken         time.Time           `json:"taken"`
	LastResetTime time.Time           `json:"last_reset_time"`
	BootProgress  api.BMCBootProgress `json:"boot_progress"`
}

// NewServerDeploymentBMCSnapshot takes a snapshot of the reboot relevant
// properties of a BMC data record.
func NewServerDeploymentBMCSnapshot(now time.Time, data api.BMCData) ServerDeploymentBMCSnapshot {
	return ServerDeploymentBMCSnapshot{
		Taken:         now,
		LastResetTime: data.ServerLastResetTime,
		BootProgress:  data.ServerBootProgress,
	}
}

// BMCData returns the snapshot in the shape api.BMCHasRebootedSince compares
// against.
func (s ServerDeploymentBMCSnapshot) BMCData() api.BMCData {
	return api.BMCData{
		ServerLastResetTime: s.LastResetTime,
		ServerBootProgress:  s.BootProgress,
	}
}

// HasRebootedSince reports, what the given BMC data says about the server having
// rebooted since the snapshot was taken.
func (s ServerDeploymentBMCSnapshot) HasRebootedSince(current api.BMCData) api.BMCRebootState {
	return api.BMCHasRebootedSince(s.BMCData(), current, s.Taken)
}

// ServerDeployment is the state of the automated deployment of a single server.
// It is persisted in Server.StatusInternal, so a daemon restart resumes exactly
// where it left off.
type ServerDeployment struct {
	State   api.ServerDeploymentState `json:"state"`
	Request ServerDeploymentRequest   `json:"request"`

	// ForceReboot reports, if the install seed reboots the server on its own
	// upon completion of the first stage of the installation. It is false only
	// for a deployment, that has been requested with force.
	ForceReboot bool `json:"force_reboot"`

	// BIOSProfiles, BIOSAttributes, BIOSDeferredAttributes and SecureBoot are
	// resolved at request time and snapshotted, so a change of the BIOS profile
	// catalog does not alter what a running deployment applies. The deferred
	// attributes are applied in a second pass, once the others are in effect.
	BIOSProfiles           []string           `json:"bios_profiles"`
	BIOSAttributes         map[string]any     `json:"bios_attributes"`
	BIOSDeferredAttributes map[string]any     `json:"bios_deferred_attributes"`
	SecureBoot             api.BIOSSecureBoot `json:"secure_boot"`

	// SecureBootCertificates holds the additional secure boot certificates of the
	// request keyed by their fingerprint, for the enrollment media to enroll.
	SecureBootCertificates map[string]string `json:"secure_boot_certificates,omitempty"`

	// Settings holds the timings deviating from the defaults, as resolved from the BIOS profiles.
	Settings api.ServerDeploymentSettings `json:"settings,omitzero"`

	// BIOSPending and BIOSDeferredPending report, whether the respective BIOS
	// pass still has anything to apply. A server, that reports the attributes at
	// their target values already, spares the deployment the whole pass.
	BIOSPending         bool `json:"bios_pending"`
	BIOSDeferredPending bool `json:"bios_deferred_pending"`

	// BIOSSecureBootPendingAttributes holds the BIOS attributes, whose
	// verification has been put off until the secure boot certificates are
	// enrolled. An attribute reflecting the secure boot state can not hold while
	// the server is in the secure boot setup mode, which is the state the
	// enrollment takes the server through, so it is only verified once the
	// enrollment has given it a chance to become true.
	BIOSSecureBootPendingAttributes []string `json:"bios_secure_boot_pending_attributes"`

	// SecureBootPending reports, whether the enrollment of the secure boot
	// certificates has written anything, in which case the firmware has to be
	// given a boot to pick them up, before the installation is started.
	SecureBootPending bool `json:"secure_boot_pending"`

	// SecureBootAttempted records, that the enrollment of the secure boot
	// certificates has been issued at least once. It is persisted before the
	// enrollment runs, since the BMC reports the key databases as applied
	// afterwards, so a re-issued enrollment can not tell an earlier attempt,
	// that wrote them, apart from a server, that held them all along.
	SecureBootAttempted bool `json:"secure_boot_attempted"`

	// SecureBootResetPending reports, whether the reset of the UEFI key
	// databases has cleared anything, in which case the firmware has to be given
	// a boot to pick it up, before the enrollment media is booted. A server,
	// that is in the secure boot setup mode already, spares the deployment that
	// boot.
	SecureBootResetPending bool `json:"secure_boot_reset_pending"`

	// SecureBootMediaURL is the secure boot enrollment media, as it is handed to
	// the BMC, while SecureBootMediaID addresses the generated media.
	SecureBootMediaURL string `json:"secure_boot_media_url"`
	SecureBootMediaID  string `json:"secure_boot_media_id"`

	// SecureBootResetTaskMonitor holds the URI of the BMC task monitor of the
	// reset of the UEFI key databases. It is kept, so the deployment waits for
	// the reset to be done before it powers the server on again.
	SecureBootResetTaskMonitor string `json:"secure_boot_reset_task_monitor"`

	// MediaURL is the installation media, as it is handed to the BMC, while
	// ImageDeploymentID names this deployment reading the generated media.
	MediaURL          string `json:"media_url"`
	ImageDeploymentID string `json:"image_deployment_id"`

	// BIOSTaskMonitor holds the URI of the BMC task monitor of the application
	// of the BIOS attributes. It is kept, since the server is powered on before
	// the outcome of the application is known.
	BIOSTaskMonitor string `json:"bios_task_monitor"`

	// FallbackAttempts counts, how often a step has routed the deployment back to
	// an earlier state, instead of being retried in place.
	FallbackAttempts int `json:"fallback_attempts"`

	// WaitRetries counts per wait, how often it has sent the deployment back to
	// an earlier step, by timing out or by reverting, since it has last been met.
	WaitRetries map[api.ServerDeploymentState]int `json:"wait_retries"`

	// PoweredOffSince records, since when the BMC reports the server powered off
	// in the power off wait the deployment is in. A single observation does not
	// establish a settled power off, since firmware, that resets the server on
	// its own, has the BMC report the power state off in the trough of that
	// reset.
	PoweredOffSince time.Time `json:"powered_off_since"`

	// LastPowerOffState names the power off, that put the server into the state
	// the steps following it rely on, so a BMC, that turns a request down
	// because the server is not settled, sends the deployment back to it.
	LastPowerOffState api.ServerDeploymentState `json:"last_power_off_state"`

	// MediaBytesRead holds how much of the installation media the BMC had read
	// when the deployment looked last, counting every byte once, no matter how
	// often it was requested, or -1, if no progress is available.
	MediaBytesRead int64 `json:"media_bytes_read"`

	// InstallOSObserved records, that the BMC has reported the server past the
	// hand over to the operating system since the install wait was anchored, so
	// the installer has been running. It is what tells the reboot at the end of
	// the first stage apart from the one the firmware performs within the POST
	// cycles of the boot, that is supposed to start the installer.
	InstallOSObserved bool `json:"install_os_observed"`

	// InstallBootProgressState holds the boot progress state the BMC reported,
	// when the install wait looked last. On a reboot, it is the state the boot
	// before it got to, which tells a boot, that never reached the installer.
	InstallBootProgressState string `json:"install_boot_progress_state"`

	// SecureBootSnapshot and InstallSnapshot hold the reboot relevant BMC
	// properties, as they were observed on the boot, that lets the firmware pick
	// the enrolled certificates up, respectively on entering the install wait.
	SecureBootSnapshot ServerDeploymentBMCSnapshot `json:"secure_boot_snapshot"`
	InstallSnapshot    ServerDeploymentBMCSnapshot `json:"install_snapshot"`

	// SecureBootEnrollSnapshot holds the same properties as they were observed
	// on the boot of the secure boot enrollment media. It is only consulted for
	// a BMC, that does not report the secure boot mode at all.
	SecureBootEnrollSnapshot ServerDeploymentBMCSnapshot `json:"secure_boot_enroll_snapshot"`

	// Retries counts the attempts already spent on the current state.
	Retries           int                       `json:"retries"`
	LastError         string                    `json:"last_error"`
	FailedState       api.ServerDeploymentState `json:"failed_state"`
	CancelRequested   bool                      `json:"cancel_requested"`
	CancelSkipCleanup bool                      `json:"cancel_skip_cleanup"`

	StartedAt      time.Time `json:"started_at"`
	StateEnteredAt time.Time `json:"state_entered_at"`
	LastAttemptAt  time.Time `json:"last_attempt_at"`
	FinishedAt     time.Time `json:"finished_at"`

	History []api.ServerDeploymentStep `json:"history"`
}

// IsActive reports, if the deployment still has steps left to perform.
func (d *ServerDeployment) IsActive() bool {
	return d != nil && !d.State.IsTerminal()
}

// EnterState moves the deployment to the given state, records the state left
// behind in the history and resets the retry budget.
func (d *ServerDeployment) EnterState(now time.Time, state api.ServerDeploymentState) {
	d.transition(now, state)

	d.Retries = 0
	d.LastError = ""
}

// FallBackTo moves the deployment back to an earlier state, keeping the retry
// budget, so a step, that keeps timing out or keeps being rejected, is not
// re-tried forever.
func (d *ServerDeployment) FallBackTo(now time.Time, state api.ServerDeploymentState) {
	d.transition(now, state)
}

func (d *ServerDeployment) transition(now time.Time, state api.ServerDeploymentState) {
	d.History = append(d.History, api.ServerDeploymentStep{
		State:     d.State,
		EnteredAt: d.StateEnteredAt,
		Retries:   d.Retries,
		Error:     d.LastError,
	})

	if len(d.History) > serverDeploymentHistoryLimit {
		d.History = slices.Delete(d.History, 0, len(d.History)-serverDeploymentHistoryLimit)
	}

	d.State = state
	d.StateEnteredAt = now

	if state.IsTerminal() {
		d.FinishedAt = now
	}
}

func (d ServerDeployment) ToAPI() *api.ServerDeploymentStatus {
	return &api.ServerDeploymentStatus{
		State:                  d.State,
		Request:                d.Request.ToAPI(),
		ForceReboot:            d.ForceReboot,
		BIOSProfiles:           d.BIOSProfiles,
		BIOSAttributes:         d.BIOSAttributes,
		BIOSDeferredAttributes: d.BIOSDeferredAttributes,
		SecureBoot:             d.SecureBoot,
		DeploymentSettings:     d.Settings,
		MediaURL:               d.MediaURL,
		SecureBootMediaURL:     d.SecureBootMediaURL,
		MediaBytesRead:         d.MediaBytesRead,
		Retries:                d.Retries,
		LastError:              d.LastError,
		FailedState:            d.FailedState,
		StartedAt:              d.StartedAt,
		StateEnteredAt:         d.StateEnteredAt,
		FinishedAt:             d.FinishedAt,
		History:                d.History,
	}
}
