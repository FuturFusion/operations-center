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
