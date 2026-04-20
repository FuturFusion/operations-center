package provisioning

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"sync"
	"time"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/lifecycle"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/internal/util/ptr"
	"github.com/FuturFusion/operations-center/shared/api"
)

//
//generate-expr: Server

type Server struct {
	ID                   int64                  `json:"-"`
	Cluster              *string                `json:"cluster"                db:"leftjoin=clusters.name"`
	Name                 string                 `json:"name"                   db:"primary=yes"`
	Type                 api.ServerType         `json:"type"`
	ConnectionURL        string                 `json:"connection_url"`
	PublicConnectionURL  string                 `json:"public_connection_url"`
	Certificate          string                 `json:"certificate"`
	Fingerprint          string                 `json:"fingerprint"            db:"ignore"`
	ClusterCertificate   *string                `json:"cluster_certificate"    db:"omit=create,update&leftjoin=clusters.certificate"`
	ClusterConnectionURL *string                `json:"cluster_connection_url" db:"omit=create,update&leftjoin=clusters.connection_url"`
	HardwareData         api.HardwareData       `json:"hardware_data"`
	OSData               api.OSData             `json:"os_data"`
	VersionData          api.ServerVersionData  `json:"version_data"`
	Channel              string                 `json:"channel"                db:"join=channels.name"`
	Status               api.ServerStatus       `json:"status"`
	StatusDetail         api.ServerStatusDetail `json:"status_detail"`
	Description          string                 `json:"description"`
	Properties           api.ConfigMap          `json:"properties"`
	LastUpdated          time.Time              `json:"last_updated"           db:"update_timestamp"`
	LastSeen             time.Time              `json:"last_seen"`
}

func (s Server) GetConnectionURL() string {
	return s.ConnectionURL
}

func (s Server) GetCertificate() string {
	if s.Cluster != nil {
		if s.ClusterCertificate != nil {
			return *s.ClusterCertificate
		}

		return ""
	}

	return s.Certificate
}

func (s Server) GetServerName() (string, error) {
	targetURL := s.ConnectionURL
	if s.ClusterConnectionURL != nil {
		targetURL = *s.ClusterConnectionURL
	}

	connectionURL, err := url.Parse(targetURL)
	if err != nil {
		return "", fmt.Errorf("Failed to get server name from connection URL %q: %w", targetURL, err)
	}

	return connectionURL.Hostname(), nil
}

func (s Server) GetName() string {
	return s.Name
}

func (s Server) Clone() Server {
	var server Server

	b, _ := json.Marshal(s)
	_ = json.Unmarshal(b, &server)

	return server
}

func (s Server) Validate() error {
	if s.Name == "" {
		return domain.NewValidationErrf("Invalid server, name can not be empty")
	}

	if s.Name == ":self" {
		return domain.NewValidationErrf(`Invalid server, ":self" is reserved for internal use and not allowed as server name`)
	}

	if s.Type != api.ServerTypeOperationsCenter && s.ConnectionURL == "" {
		return domain.NewValidationErrf("Invalid server, connection URL can not be empty for server type %s", s.Type)
	}

	_, err := url.Parse(s.ConnectionURL)
	if err != nil {
		return domain.NewValidationErrf("Invalid server, connection URL is not valid: %v", err)
	}

	if s.PublicConnectionURL != "" {
		_, err := url.Parse(s.PublicConnectionURL)
		if err != nil {
			return domain.NewValidationErrf("Invalid server, public connection URL is not valid: %v", err)
		}
	}

	if s.Certificate == "" {
		return domain.NewValidationErrf("Invalid server, certificate can not be empty")
	}

	var serverType api.ServerType
	err = serverType.UnmarshalText([]byte(s.Type))
	if s.Type == "" || err != nil {
		return domain.NewValidationErrf("Invalid server, validation of type failed: %v", err)
	}

	var serverStatus api.ServerStatus
	err = serverStatus.UnmarshalText([]byte(s.Status))
	if s.Status == "" || err != nil {
		return domain.NewValidationErrf("Invalid server, validation of status failed: %v", err)
	}

	var serverStatusDetail api.ServerStatusDetail
	err = serverStatusDetail.UnmarshalText([]byte(s.StatusDetail))
	if err != nil {
		return domain.NewValidationErrf("Invalid server, validation of status detail failed: %v", err)
	}

	if s.Channel == "" {
		return domain.NewValidationErrf("Invalid server, channel can not be empty")
	}

	return nil
}

func (s Server) UpdateState() api.ServerUpdateState {
	return api.Server{
		Cluster:      ptr.From(s.Cluster),
		Status:       s.Status,
		StatusDetail: s.StatusDetail,
		VersionData:  s.VersionData,
	}.UpdateState()
}

var signalLifecycleEventDelay = 3 * time.Second

func (s Server) signalLifecycleEvent() {
	go func() {
		// Defer lifecycle signal a bit, let the triggering event complete first.
		time.Sleep(signalLifecycleEventDelay)

		// Use a detached context in order to make sure, no existing DB transaction is inherited.
		ctx := context.Background()

		slm := lifecycle.ServerLifecycleMessage{
			Server:            s.Name,
			Cluster:           s.Cluster,
			ServerUpdateState: s.UpdateState(),
		}

		err := lifecycle.ServerLifecycleSignal.TryEmit(ctx, slm)
		if err != nil {
			slog.ErrorContext(ctx, "Signal lifecycle event failed", logger.Err(err), slog.Any("server_lifecycle_message", slm))
		}
	}()
}

type Servers []Server

type ServerFilter struct {
	ID           *int
	Name         *string
	Cluster      *string
	Status       *api.ServerStatus
	StatusDetail *api.ServerStatusDetail
	Certificate  *string
	Type         *api.ServerType
	Expression   *string `db:"ignore"`
}

func (f ServerFilter) AppendToURLValues(query url.Values) url.Values {
	if f.Cluster != nil {
		query.Add("cluster", *f.Cluster)
	}

	if f.Status != nil {
		query.Add("status", string(*f.Status))
	}

	if f.Certificate != nil {
		query.Add("certificate", *f.Certificate)
	}

	if f.Type != nil {
		query.Add("type", f.Type.String())
	}

	if f.Expression != nil {
		query.Add("filter", *f.Expression)
	}

	return query
}

func (f ServerFilter) String() string {
	return f.AppendToURLValues(url.Values{}).Encode()
}

type ServerSelfUpdate struct {
	ConnectionURL             string
	AuthenticationCertificate *x509.Certificate

	// Self is set to true, if the self update API has been called through
	// unix socket. This is the case, when IncusOS is serving Operations Center
	// and triggers a self update on its self.
	Self bool
}

type ServerSystemNetwork = api.ServerSystemNetwork

type ServerSystemNetworkVLAN = api.ServerSystemNetworkVLAN

type ServerSystemStorage = api.ServerSystemStorage

type ServerSystemProvider = api.ServerSystemProvider

type ServerSystemUpdate = api.ServerSystemUpdate

type ServerSystemKernel = api.ServerSystemKernel

type ServerSystemLogging = api.ServerSystemLogging

type operation int

const (
	operationNone operation = iota
	operationEvacuation
	operationReboot
	operationRestore
)

type volatileServerStates struct {
	mu      sync.Mutex
	servers map[string]volatileServerState
}

type volatileServerState struct {
	inFlightOperation   operation
	operationRetryCount int
	operationLastErr    error
}

func (v *volatileServerStates) retryCount(serverName string) int {
	v.mu.Lock()
	defer v.mu.Unlock()

	s, ok := v.servers[serverName]
	if !ok {
		s = volatileServerState{}
	}

	return s.operationRetryCount
}

// start sets the volatile server state for the given server to in flight
// with the given operation.
func (v *volatileServerStates) start(serverName string, op operation) bool {
	v.mu.Lock()
	defer v.mu.Unlock()

	s, ok := v.servers[serverName]
	if !ok {
		s = volatileServerState{}
	}

	if s.inFlightOperation != operationNone {
		return false
	}

	s.inFlightOperation = op
	s.operationRetryCount++
	s.operationLastErr = nil

	v.servers[serverName] = s

	return true
}

// done sets the volatile server state for the given server and marks the
// previously in flight operation as completed.
func (v *volatileServerStates) done(serverName string, op operation, err error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	s, ok := v.servers[serverName]
	if !ok {
		s = volatileServerState{}
	}

	if s.inFlightOperation != op {
		return
	}

	s.inFlightOperation = operationNone
	s.operationLastErr = err

	v.servers[serverName] = s
}

// reset resets all operation related state.
func (v *volatileServerStates) reset(serverName string, op operation) {
	v.mu.Lock()
	defer v.mu.Unlock()

	s, ok := v.servers[serverName]
	if !ok {
		s = volatileServerState{}
	}

	if s.inFlightOperation != op {
		return
	}

	s.inFlightOperation = operationNone
	s.operationRetryCount = 0
	s.operationLastErr = nil

	v.servers[serverName] = s
}

// lastErr returns the last recorded error for a given server operation.
func (v *volatileServerStates) lastErr(serverName string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	s, ok := v.servers[serverName]
	if !ok {
		return nil
	}

	return s.operationLastErr
}
