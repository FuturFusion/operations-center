package provisioning

import (
	"net/url"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/lxc/incus-os/incus-osd/api/images"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/shared/api"
)

// serverDeploymentHistoryLimit bounds the number of steps kept on a deployment,
// so a deployment retrying forever can not grow the server record without end.
const serverDeploymentHistoryLimit = 200

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
	if !ok || r.Architecture == images.UpdateFileArchitectureUndefined {
		return domain.NewValidationErrf("Invalid deployment request, architecture %q is not valid", r.Architecture)
	}

	return nil
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
	if request.Architecture == "" {
		architecture = images.UpdateFileArchitecture64BitX86
	}

	return ServerDeploymentRequest{
		TokenUUID:                  tokenUUID,
		Seed:                       request.Seed,
		ImageType:                  imageType,
		Architecture:               architecture,
		Channel:                    request.Channel,
		VirtualMediaID:             request.VirtualMediaID,
		Force:                      request.Force,
		SkipSecureBootCertificates: request.SkipSecureBootCertificates,
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

	// BIOSPending and BIOSDeferredPending report, whether the respective BIOS
	// pass still has anything to apply. A server, that reports the attributes at
	// their target values already, spares the deployment the whole pass.
	BIOSPending         bool `json:"bios_pending"`
	BIOSDeferredPending bool `json:"bios_deferred_pending"`

	// SecureBootPending reports, whether the enrollment of the secure boot
	// certificates has written anything, in which case the firmware has to be
	// given a boot to pick them up, before the installation is started.
	SecureBootPending bool `json:"secure_boot_pending"`

	// MediaURL is the installation media, as it is handed to the BMC, while
	// ImageCacheID and ImageFingerprintID address the generated media, so the
	// read progress recorded for it can be looked up.
	MediaURL           string `json:"media_url"`
	ImageCacheID       string `json:"image_cache_id"`
	ImageFingerprintID string `json:"image_fingerprint_id"`

	// BIOSTaskMonitor holds the URI of the BMC task monitor of the application
	// of the BIOS attributes. It is kept, since the server is powered on before
	// the outcome of the application is known.
	BIOSTaskMonitor string `json:"bios_task_monitor"`

	// FallbackAttempts counts, how often a step has routed the deployment back to
	// an earlier state, instead of being retried in place.
	FallbackAttempts int `json:"fallback_attempts"`

	// MediaBytesRead holds how much of the installation media of size MediaSize
	// the BMC had read when the deployment looked last, counting every byte once,
	// no matter how often it was requested, or -1, if no progress is available.
	MediaBytesRead int64 `json:"media_bytes_read"`
	MediaSize      int64 `json:"media_size"`

	// SecureBootSnapshot and InstallSnapshot hold the reboot relevant BMC
	// properties, as they were observed on the boot, that lets the firmware pick
	// the enrolled certificates up, respectively on entering the install wait.
	SecureBootSnapshot ServerDeploymentBMCSnapshot `json:"secure_boot_snapshot"`
	InstallSnapshot    ServerDeploymentBMCSnapshot `json:"install_snapshot"`

	// Retries counts the attempts already spent on the current state.
	Retries         int                       `json:"retries"`
	LastError       string                    `json:"last_error"`
	FailedState     api.ServerDeploymentState `json:"failed_state"`
	CancelRequested bool                      `json:"cancel_requested"`

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

// SeedImageID returns the identity of the generated installation media of the
// deployment. It is only set once the media has been attached.
func (d ServerDeployment) SeedImageID() SeedImageID {
	return SeedImageID{
		CacheID:       d.ImageCacheID,
		FingerprintID: d.ImageFingerprintID,
	}
}

// BMCSource returns the address, the BMC of the server is expected to read the
// installation media from.
func (s Server) BMCSource() string {
	endpoint := s.BMCConfig.Endpoint
	if endpoint == "" {
		return ""
	}

	endpointURL, err := url.Parse(endpoint)
	if err != nil || endpointURL.Host == "" {
		return SeedImageSource(endpoint)
	}

	return SeedImageSource(endpointURL.Host)
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
		MediaURL:               d.MediaURL,
		MediaBytesRead:         d.MediaBytesRead,
		MediaSize:              d.MediaSize,
		Retries:                d.Retries,
		LastError:              d.LastError,
		FailedState:            d.FailedState,
		StartedAt:              d.StartedAt,
		StateEnteredAt:         d.StateEnteredAt,
		FinishedAt:             d.FinishedAt,
		History:                d.History,
	}
}
