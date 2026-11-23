package config

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/environment/mock"
	"github.com/FuturFusion/operations-center/internal/lifecycle"
	"github.com/FuturFusion/operations-center/shared/api/system"
)

// Test_store_update_concurrent ensures a config update is a proper read modify
// write cycle. Concurrent updates of unrelated sections used to lose each other,
// because the lock was released while validation was delegated.
func Test_store_update_concurrent(t *testing.T) {
	InitTest(t, &mock.EnvironmentMock{
		ClusteredFunc: func() bool { return false },
		IsIncusOSFunc: func() bool { return false },
	}, nil)

	// Stand in for the real validators, which do DB queries and HTTP requests and
	// therefore keep an update busy long enough for a second one to interleave.
	lifecycle.UpdatesValidateSignal.AddListenerWithErr(func(_ context.Context, _ system.Updates) error {
		time.Sleep(time.Millisecond)

		return nil
	}, t.Name())
	defer lifecycle.UpdatesValidateSignal.RemoveListener(t.Name())

	const rounds = 50

	// Errors are collected instead of asserted, since require must only be used
	// from the goroutine running the test.
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		firstErr error
	)

	collect := func(err error) {
		mu.Lock()
		defer mu.Unlock()

		if firstErr == nil {
			firstErr = err
		}
	}

	wg.Add(2)

	go func() {
		defer wg.Done()

		for range rounds {
			collect(UpdateLocalNetwork(t.Context(), system.NetworkLocalPut{
				RestServerAddress: "127.0.0.1:8443",
			}))
		}
	}()

	go func() {
		defer wg.Done()

		for range rounds {
			collect(UpdateSecurity(t.Context(), system.SecurityPut{
				TrustedTLSClientCertFingerprints: []string{"fingerprint"},
			}))
		}
	}()

	wg.Wait()

	require.NoError(t, firstErr)

	require.Equal(t, "127.0.0.1:8443", GetLocalNetwork().RestServerAddress)
	require.Equal(t, []string{"fingerprint"}, GetSecurity().TrustedTLSClientCertFingerprints)
}

// Test_store_reload_concurrent ensures a reload reads the stored configuration only after a change in progress is committed.
func Test_store_reload_concurrent(t *testing.T) {
	InitTest(t, &mock.EnvironmentMock{
		ClusteredFunc: func() bool { return false },
		IsIncusOSFunc: func() bool { return false },
	}, nil)

	var dbMu sync.Mutex
	db := "old"

	persisting := make(chan struct{})
	release := make(chan struct{})

	s := newStore(nil,
		func(_ context.Context, _ section, _ string, newCfg string) (string, publishFunc, error) {
			close(persisting)
			<-release

			dbMu.Lock()
			db = newCfg
			dbMu.Unlock()

			return newCfg, nil, nil
		},
		func(context.Context, section, string, string) error { return nil },
		func(cfg string) (string, error) { return cfg, nil },
		func(context.Context, localConfig, string, string, enver) error { return nil },
		"old",
	)

	updated := make(chan error)
	go func() {
		updated <- s.Update(t.Context(), sectionChanged, func(context.Context, string) string { return "new" })
	}()

	<-persisting

	reloaded := make(chan error)
	go func() {
		reloaded <- s.reload(t.Context(), func(context.Context) (string, error) {
			dbMu.Lock()
			defer dbMu.Unlock()

			return db, nil
		})
	}()

	// Give the reload the chance to read before the change is committed.
	time.Sleep(50 * time.Millisecond)
	close(release)

	require.NoError(t, <-updated)
	require.NoError(t, <-reloaded)
	require.Equal(t, "new", s.Get())
}

// Test_store_reload_during_publish ensures a reload, as requested by another member, doesn't wait for this member's publish.
func Test_store_reload_during_publish(t *testing.T) {
	InitTest(t, &mock.EnvironmentMock{
		ClusteredFunc: func() bool { return false },
		IsIncusOSFunc: func() bool { return false },
	}, nil)

	publishing := make(chan struct{})
	release := make(chan struct{})

	s := newStore(nil,
		func(_ context.Context, _ section, _ string, newCfg string) (string, publishFunc, error) {
			return newCfg, func(context.Context) error {
				close(publishing)
				<-release

				return nil
			}, nil
		},
		func(context.Context, section, string, string) error { return nil },
		func(cfg string) (string, error) { return cfg, nil },
		func(context.Context, localConfig, string, string, enver) error { return nil },
		"old",
	)

	updated := make(chan error)
	go func() {
		updated <- s.Update(t.Context(), sectionChanged, func(context.Context, string) string { return "new" })
	}()

	<-publishing
	require.Equal(t, "new", s.Get())

	err := s.reload(t.Context(), func(context.Context) (string, error) { return "other", nil })
	require.NoError(t, err)
	require.Equal(t, "other", s.Get())

	close(release)
	require.NoError(t, <-updated)
}

func Test_store_reload_notifyError(t *testing.T) {
	t.Parallel()

	notified := 0
	s := newStore(nil,
		func(_ context.Context, _ section, _ string, newCfg string) (string, publishFunc, error) {
			return newCfg, nil, nil
		},
		func(_ context.Context, _ section, _ string, newCfg string) error {
			notified++
			if newCfg == "broken" {
				return errors.New("listener failed")
			}

			return nil
		},
		func(cfg string) (string, error) { return cfg, nil },
		func(context.Context, localConfig, string, string, enver) error { return nil },
		"initial",
	)

	// A listener failing to apply a reloaded config is reported, so the publishing member reverts the change.
	err := s.reload(t.Context(), func(context.Context) (string, error) { return "broken", nil })
	require.ErrorContains(t, err, "listener failed")
	require.Equal(t, "broken", s.Get(), "the reloaded config is committed before the listeners run")
	require.Equal(t, 1, notified)

	// A change of this member only logs a failing listener.
	err = s.Update(t.Context(), sectionChanged, func(context.Context, string) string { return "broken" })
	require.NoError(t, err)
	require.Equal(t, 2, notified)

	require.NoError(t, s.reload(t.Context(), func(context.Context) (string, error) { return "fine", nil }))
	require.Equal(t, "fine", s.Get())
}
