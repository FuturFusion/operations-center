package config

import (
	"context"
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
			collect(UpdateNetwork(t.Context(), system.NetworkPut{
				RestServerAddress:       "127.0.0.1:7443",
				OperationsCenterAddress: "https://localhost:7443",
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

	require.Equal(t, "127.0.0.1:7443", GetNetwork().RestServerAddress)
	require.Equal(t, []string{"fingerprint"}, GetSecurity().TrustedTLSClientCertFingerprints)
}
