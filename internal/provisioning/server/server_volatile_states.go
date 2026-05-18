package server

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/FuturFusion/operations-center/internal/util/logger"
)

type operation int

func (o operation) String() string {
	switch o {
	case operationNone:
		return "None"

	case operationEvacuation:
		return "Evacuation"

	case operationReboot:
		return "Reboot"

	case operationRestore:
		return "Restore"

	default:
		return fmt.Sprintf("Undefined %d", o)
	}
}

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
func (v *volatileServerStates) start(ctx context.Context, serverName string, op operation) bool {
	slog.DebugContext(ctx, "volatile server state start", slog.String("server", serverName), slog.String("operation", op.String()))

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
func (v *volatileServerStates) done(ctx context.Context, serverName string, op operation, err error) {
	slog.DebugContext(ctx, "volatile server state done", slog.String("server", serverName), slog.String("operation", op.String()), logger.Err(err))

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
func (v *volatileServerStates) resetAll(ctx context.Context, serverName string) {
	slog.DebugContext(ctx, "volatile server state reset all", slog.String("server", serverName))

	v.mu.Lock()
	defer v.mu.Unlock()

	s, ok := v.servers[serverName]
	if !ok {
		s = volatileServerState{}
	}

	s.inFlightOperation = operationNone
	s.operationRetryCount = 0
	s.operationLastErr = nil

	v.servers[serverName] = s
}

// reset resets all operation related state if the operation matches.
func (v *volatileServerStates) reset(ctx context.Context, serverName string, op operation) {
	slog.DebugContext(ctx, "volatile server state reset", slog.String("server", serverName), slog.String("operation", op.String()))

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
