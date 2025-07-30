package provisioning

import "github.com/maniartech/signals"

func ServerServiceWithSelfUpdateSignal(signal signals.Signal[Server]) ServerServiceOption {
	return func(s *serverService) {
		s.selfUpdateSignal = signal
	}
}
