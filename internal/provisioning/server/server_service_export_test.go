package server

import (
	"net/http"

	"github.com/maniartech/signals"

	"github.com/FuturFusion/operations-center/internal/provisioning"
)

func WithSelfUpdateSignal(signal signals.Signal[provisioning.Server]) Option {
	return func(s *serverService) {
		s.selfUpdateSignal = signal
	}
}

func WithHTTPClient(httpClient *http.Client) Option {
	return func(s *serverService) {
		s.httpClient = httpClient
	}
}

// WaitBackgroundTasks waits for all currently running background tasks of the
// service to complete.
func (s *serverService) WaitBackgroundTasks() {
	s.backgroundTasks.Wait()
}
