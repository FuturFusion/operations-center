package server

import (
	"github.com/maniartech/signals"

	"github.com/FuturFusion/operations-center/internal/provisioning"
)

func WithSelfUpdateSignal(signal signals.Signal[provisioning.Server]) Option {
	return func(s *serverService) {
		s.selfUpdateSignal = signal
	}
}

// WaitBackgroundTasks waits for all currently running background tasks of the
// service to complete.
func (s *serverService) WaitBackgroundTasks() {
	s.background.running.Wait()
}
