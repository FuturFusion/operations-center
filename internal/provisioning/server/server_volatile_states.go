package server

import "sync"

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
