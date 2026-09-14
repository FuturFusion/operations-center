package config

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/FuturFusion/operations-center/internal/util/logger"
)

// section names the part of the configuration an update is addressed to. It
// tells notify which subsystem asked for the update, so that subsystem is
// informed even if the submitted values turn out to be the ones already in
// effect.
type section int

const (
	sectionNetwork section = iota
	sectionSecurity
	sectionSettings
	sectionUpdates
)

// store owns one configuration value and everything needed to change it.
//
// Locking: updateMu serializes writers for the entire duration of an update,
// which covers validation, persistence and notification. The committed value is
// held in an atomic pointer and is never mutated after it has been published, so
// readers neither block nor take a lock. These are the only two synchronization
// primitives in the package.
//
// Reentrancy: validators and notification listeners MAY read the configuration
// through the package level getters. They MUST NOT write it, doing so deadlocks
// on updateMu.
type store struct {
	updateMu sync.Mutex
	current  atomic.Pointer[config]

	env     enver
	persist func(env enver, cfg config) error
}

func newStore(env enver, persist func(enver, config) error, initial config) *store {
	s := &store{
		env:     env,
		persist: persist,
	}

	s.current.Store(&initial)

	return s
}

// get returns the committed configuration.
func (s *store) get() config {
	return *s.current.Load()
}

// update is the only writer. It runs the phases of a configuration change in
// order: snapshot, mutate, normalize, validate, delegated validate, persist,
// commit, notify.
//
// mutate receives the committed configuration by value and returns the desired
// new one. It can not fail: deriving values is normalize's job and rejecting
// them is validate's.
func (s *store) update(ctx context.Context, updated section, mutate func(cfg config) config) error {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()

	oldCfg := s.get()

	newCfg, err := normalize(mutate(oldCfg))
	if err != nil {
		return err
	}

	err = validate(oldCfg, newCfg, s.env.IsIncusOS())
	if err != nil {
		return fmt.Errorf("Failed to validate configuration: %w", err)
	}

	err = validateDelegated(ctx, oldCfg, newCfg)
	if err != nil {
		return fmt.Errorf("Failed to validate configuration: %w", err)
	}

	err = s.persist(s.env, newCfg)
	if err != nil {
		return err
	}

	s.current.Store(&newCfg)

	// Detached from the request context on purpose: the configuration is
	// committed at this point, so a client that went away must not keep the
	// subsystems from picking the new configuration up.
	notify(logger.DetachedContext(ctx), updated, oldCfg, newCfg)

	return nil
}
