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

// sectionChanged notifies only about the sections that actually changed.
const sectionChanged section = -1

const (
	sectionNetwork section = iota
	sectionSecurity
	sectionSettings
	sectionUpdates
	sectionCluster
)

// store owns one configuration value and everything needed to change it.
//
// Locking: updateMu serializes writers for the entire duration of an update,
// which covers validation, persistence and notification. The committed value is
// held in an atomic pointer and is never mutated after it has been published, so
// readers neither block nor take a lock.
//
// Reentrancy: validators and notification listeners MAY read the configuration
// through the package level getters. They MUST NOT write it, doing so deadlocks
// on updateMu.
type store[T any] struct {
	updateMu sync.Mutex
	current  atomic.Pointer[T]

	env       enver
	persist   persistFunc[T]
	notify    notifyFunc[T]
	validate  validateFunc[T]
	normalize normalizeFunc[T]
}

type (
	persistFunc[T any]   func(ctx context.Context, cfg T) error
	notifyFunc[T any]    func(ctx context.Context, update section, oldCfg, newCfg T)
	validateFunc[T any]  func(ctx context.Context, currentLocalCfg localConfig, oldCfg, newCfg T, env enver) error
	normalizeFunc[T any] func(T) (T, error)
)

func newStore[T any](env enver, persist persistFunc[T], notify notifyFunc[T], normalize normalizeFunc[T], validate validateFunc[T], initial T) *store[T] {
	s := &store[T]{
		env:       env,
		persist:   persist,
		notify:    notify,
		validate:  validate,
		normalize: normalize,
	}

	s.current.Store(&initial)

	return s
}

// newLocalStore returns the store of the local config persisted to the config file of env.
func newLocalStore(env enver, persist persistFunc[localConfig], initial localConfig) *store[localConfig] {
	return newStore(env, persist, notifyLocal, normalize, localConfigValidator, initial)
}

// diskPersister persists the local config to the config file of env.
func diskPersister(env enver) persistFunc[localConfig] {
	return func(_ context.Context, cfg localConfig) error {
		return saveToDisk(env, cfg)
	}
}

func localConfigValidator(ctx context.Context, _ localConfig, oldCfg, newCfg localConfig, env enver) error {
	err := validate(oldCfg, newCfg, env.IsIncusOS(), env.Clustered())
	if err != nil {
		return err
	}

	return validateLocalDelegated(ctx, newCfg)
}

// Get returns the committed configuration.
func (s *store[T]) Get() T {
	return *s.current.Load()
}

// Update runs the phases of a configuration change in order: snapshot, mutate,
// normalize, validate, persist, commit, notify.
//
// mutate receives the committed configuration by value and returns the desired
// new one. It can not fail: deriving values is normalize's job and rejecting
// them is validate's.
func (s *store[T]) Update(ctx context.Context, update section, mutate func(ctx context.Context, cfg T) T) error {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()

	oldCfg := s.Get()

	newCfg, err := s.normalize(mutate(ctx, oldCfg))
	if err != nil {
		return err
	}

	err = s.validate(ctx, defaultStore.Load().Get(), oldCfg, newCfg, s.env)
	if err != nil {
		return fmt.Errorf("Failed to validate configuration: %w", err)
	}

	err = s.persist(ctx, newCfg)
	if err != nil {
		return err
	}

	s.current.Store(&newCfg)

	// Detached from the request context on purpose: the configuration is
	// committed at this point, so a client that went away must not keep the
	// subsystems from picking the new configuration up.
	s.notify(logger.DetachedContext(ctx), update, oldCfg, newCfg)

	return nil
}

// replace commits cfg without validation and notifies about the differences
// to the previous value. A non nil persist replaces the persist function.
func (s *store[T]) replace(ctx context.Context, cfg T, persist persistFunc[T]) {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()

	oldCfg := s.Get()

	if persist != nil {
		s.persist = persist
	}

	s.current.Store(&cfg)

	s.notify(logger.DetachedContext(ctx), sectionChanged, oldCfg, cfg)
}

// set runs mutate with the committed configuration without validation and notification.
// Persisting also commits the new configuration.
func (s *store[T]) set(ctx context.Context, mutate func(oldCfg T, persist func(newCfg T) error) error) error {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()

	return mutate(s.Get(), func(newCfg T) error {
		err := s.persist(ctx, newCfg)
		if err != nil {
			return err
		}

		s.current.Store(&newCfg)

		return nil
	})
}
