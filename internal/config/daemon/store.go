package config

import (
	"context"
	"fmt"
	"log/slog"
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

// sectionLoaded notifies only about the sections that changed by loading the
// stored config. Loading does not change the config, so it triggers no ACME renewal.
const sectionLoaded section = -2

const (
	sectionNetwork section = iota
	sectionSecurity
	sectionSettings
	sectionUpdates
	sectionCluster
)

// store owns one configuration value and everything needed to change it.
//
// Locking: updateMu serializes writers for the duration of an update, which
// covers validation, persistence and notification. Publishing the change to the
// other cluster members runs once updateMu is released, since they reload under
// their own updateMu. The committed value is held in an atomic pointer and is
// never mutated after it has been published, so readers neither block nor take a lock.
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
	// persistFunc returns the persisted config and, for a config shared by the cluster members, the step publishing it to them.
	persistFunc[T any] func(ctx context.Context, update section, oldCfg T, newCfg T) (T, publishFunc, error)
	publishFunc        func(ctx context.Context) error
	// notifyFunc applies a committed config and returns the errors of the subsystems failing to.
	notifyFunc[T any]    func(ctx context.Context, update section, oldCfg, newCfg T) error
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
	return func(_ context.Context, _ section, _ localConfig, cfg localConfig) (localConfig, publishFunc, error) {
		err := saveToDisk(env, cfg)
		if err != nil {
			return localConfig{}, nil, err
		}

		return cfg, nil, nil
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
// normalize, validate, persist, commit, notify, and publish once unlocked.
//
// mutate receives the committed configuration by value and returns the desired
// new one. It can not fail: deriving values is normalize's job and rejecting
// them is validate's.
func (s *store[T]) Update(ctx context.Context, update section, mutate func(ctx context.Context, cfg T) T) error {
	publish, err := s.commit(ctx, update, mutate)
	if err != nil || publish == nil {
		return err
	}

	// The other members reload under their own updateMu, which a change in progress on them holds.
	return publish(ctx)
}

// commit runs the phases of Update up to the notification while holding updateMu.
func (s *store[T]) commit(ctx context.Context, update section, mutate func(ctx context.Context, cfg T) T) (publishFunc, error) {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()

	oldCfg := s.Get()

	newCfg, err := s.normalize(mutate(ctx, oldCfg))
	if err != nil {
		return nil, err
	}

	err = s.validate(ctx, defaultStore.Load().Get(), oldCfg, newCfg, s.env)
	if err != nil {
		return nil, fmt.Errorf("Failed to validate configuration: %w", err)
	}

	// The persisted configuration can hold changes of other cluster members merged in.
	newCfg, publish, err := s.persist(ctx, update, oldCfg, newCfg)
	if err != nil {
		return nil, err
	}

	s.current.Store(&newCfg)

	// Detached from the request context on purpose: the configuration is
	// committed at this point, so a client that went away must not keep the
	// subsystems from picking the new configuration up.
	err = s.notify(logger.DetachedContext(ctx), update, oldCfg, newCfg)
	if err != nil {
		// The change is committed, a subsystem failing to apply it is applied again by re-submitting the configuration.
		slog.ErrorContext(logger.ContextWithComponent(ctx, componentConfig), "Failed to apply the updated config", logger.Err(err))
	}

	return publish, nil
}

// reload commits the configuration returned by fetch without validation and notifies about the differences to the previous value.
// fetch runs while no change of this store is in progress, so it can't return a value older than a change committed meanwhile.
func (s *store[T]) reload(ctx context.Context, fetch func(ctx context.Context) (T, error)) error {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()

	cfg, err := fetch(ctx)
	if err != nil {
		return err
	}

	cfg, err = s.normalize(cfg)
	if err != nil {
		return err
	}

	oldCfg := s.Get()
	s.current.Store(&cfg)

	// The member publishing the change reverts it, if this member fails to apply it.
	return s.notify(logger.DetachedContext(ctx), sectionChanged, oldCfg, cfg)
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

	err := s.notify(logger.DetachedContext(ctx), sectionLoaded, oldCfg, cfg)
	if err != nil {
		slog.ErrorContext(logger.ContextWithComponent(ctx, componentConfig), "Failed to apply the loaded config", logger.Err(err))
	}
}

// set runs mutate with the committed configuration without validation and notification.
// Persisting also commits the new configuration.
func (s *store[T]) set(ctx context.Context, mutate func(oldCfg T, persist func(newCfg T) error) error) error {
	var publish publishFunc

	err := func() error {
		s.updateMu.Lock()
		defer s.updateMu.Unlock()

		oldCfg := s.Get()

		return mutate(oldCfg, func(newCfg T) error {
			var err error
			newCfg, publish, err = s.persist(ctx, sectionChanged, oldCfg, newCfg)
			if err != nil {
				return err
			}

			s.current.Store(&newCfg)

			return nil
		})
	}()
	if err != nil || publish == nil {
		return err
	}

	return publish(ctx)
}
