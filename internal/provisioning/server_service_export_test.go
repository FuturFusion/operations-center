package provisioning

import (
	"net/http"

	"github.com/maniartech/signals"
)

func ServerServiceWithSelfUpdateSignal(signal signals.Signal[Server]) ServerServiceOption {
	return func(s *serverService) {
		s.selfUpdateSignal = signal
	}
}

func ServerServiceWithHTTPClient(httpClient *http.Client) ServerServiceOption {
	return func(s *serverService) {
		s.httpClient = httpClient
	}
}
