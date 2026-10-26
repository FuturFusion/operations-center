package incus_test

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	incusapi "github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/adapter/incus"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/internal/util/testing/log"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func TestClientServer_SubscribeLifecycleEvents(t *testing.T) {
	t.Parallel()

	caPool, certPEM, keyPEM := setupCerts(t)

	readyContext := func(ctx context.Context) (context.Context, func()) {
		return context.WithCancel(ctx)
	}

	cancelledContext := func(ctx context.Context) (context.Context, func()) {
		ctx, cancel := context.WithCancel(ctx)
		cancel() // cancel the context already now.
		return ctx, cancel
	}

	tests := []struct {
		name              string
		getCtx            func(ctx context.Context) (context.Context, func())
		blockEventChannel bool
		handler           func(ready, done chan struct{}) func(http.ResponseWriter, *http.Request)
		clientCertPEM     string
		clientKeyPEM      string

		assertErr        require.ErrorAssertionFunc
		assertErrChanErr require.ErrorAssertionFunc
		wantEvent        domain.LifecycleEvent
		assertLog        log.MatcherFunc
	}{
		{
			name:   "success one event",
			getCtx: readyContext,
			handler: func(ready, done chan struct{}) func(w http.ResponseWriter, r *http.Request) {
				return func(w http.ResponseWriter, r *http.Request) {
					conn, err := upgrader.Upgrade(w, r, nil)
					if err != nil {
						t.Errorf("Failed to upgrade websocket connection: %v", err)
						return
					}

					defer conn.Close()

					<-ready

					err = conn.WriteJSON(createLifecycleEvent(t, incusapi.EventLifecycleImageCreated))
					if err != nil {
						t.Errorf("Failed to write event: %v", err)
						return
					}

					<-done
				}
			},
			clientCertPEM: certPEM,
			clientKeyPEM:  keyPEM,

			assertErr:        require.NoError,
			assertErrChanErr: require.NoError,
			wantEvent: domain.LifecycleEvent{
				Operation:            domain.LifecycleOperationCreate,
				ResourceType:         domain.ResourceTypeImage,
				LifecycleEventAction: "image-created",
				Source: domain.LifecycleSource{
					Name:        "7ca66bd33c15ced9c300c76438e8c7d126ee4d114c66de65c59d04ca2cc818b7",
					ProjectName: "default",
				},
			},
			assertLog: log.Empty,
		},
		{
			name:   "error - getClient",
			getCtx: readyContext,
			handler: func(ready, done chan struct{}) func(w http.ResponseWriter, r *http.Request) {
				return func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				}
			},
			clientCertPEM: certPEM,
			clientKeyPEM:  certPEM, // invalid value

			assertErr: require.Error,
			assertLog: log.Empty,
		},
		{
			name:   "error - GetEventsAllProjects",
			getCtx: readyContext,
			handler: func(ready, done chan struct{}) func(w http.ResponseWriter, r *http.Request) {
				return func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				}
			},
			clientCertPEM: certPEM,
			clientKeyPEM:  keyPEM,

			assertErr: require.Error,
			assertLog: log.Empty,
		},
		{
			name:   "error - invalid lifecycle event",
			getCtx: readyContext,
			handler: func(ready, done chan struct{}) func(w http.ResponseWriter, r *http.Request) {
				return func(w http.ResponseWriter, r *http.Request) {
					conn, err := upgrader.Upgrade(w, r, nil)
					if err != nil {
						t.Errorf("Failed to upgrade websocket connection: %v", err)
						return
					}

					defer conn.Close()

					<-ready

					err = conn.WriteJSON(incusapi.Event{
						Type:      incusapi.EventTypeLifecycle,
						Timestamp: time.Date(2025, 10, 30, 17, 5, 0, 0, time.UTC),
						Metadata:  json.RawMessage([]byte("[]")), // invalid, object expected.
						Location:  "none",
						Project:   "default",
					}) // not a valid lifecycle event.
					if err != nil {
						t.Errorf("Failed to write event: %v", err)
						return
					}

					err = conn.WriteJSON(createLifecycleEvent(t, incusapi.EventLifecycleImageCreated))
					if err != nil {
						t.Errorf("Failed to write event: %v", err)
						return
					}

					<-done
				}
			},
			clientCertPEM: certPEM,
			clientKeyPEM:  keyPEM,

			assertErr:        require.NoError,
			assertErrChanErr: require.NoError,
			wantEvent: domain.LifecycleEvent{
				Operation:            domain.LifecycleOperationCreate,
				ResourceType:         domain.ResourceTypeImage,
				LifecycleEventAction: "image-created",
				Source: domain.LifecycleSource{
					Name:        "7ca66bd33c15ced9c300c76438e8c7d126ee4d114c66de65c59d04ca2cc818b7",
					ProjectName: "default",
				},
			},
			assertLog: log.Contains("Failed to map incus event to lifecycle event"),
		},
		{
			name:   "error - unsupported lifecycle event resource type",
			getCtx: readyContext,
			handler: func(ready, done chan struct{}) func(w http.ResponseWriter, r *http.Request) {
				return func(w http.ResponseWriter, r *http.Request) {
					conn, err := upgrader.Upgrade(w, r, nil)
					if err != nil {
						t.Errorf("Failed to upgrade websocket connection: %v", err)
						return
					}

					defer conn.Close()

					<-ready

					err = conn.WriteJSON(createLifecycleEvent(t, "unsupported-action")) // unsupported action
					if err != nil {
						t.Errorf("Failed to write event: %v", err)
						return
					}

					err = conn.WriteJSON(createLifecycleEvent(t, incusapi.EventLifecycleImageCreated))
					if err != nil {
						t.Errorf("Failed to write event: %v", err)
						return
					}

					<-done
				}
			},
			clientCertPEM: certPEM,
			clientKeyPEM:  keyPEM,

			assertErr:        require.NoError,
			assertErrChanErr: require.NoError,
			wantEvent: domain.LifecycleEvent{
				Operation:            domain.LifecycleOperationCreate,
				ResourceType:         domain.ResourceTypeImage,
				LifecycleEventAction: "image-created",
				Source: domain.LifecycleSource{
					Name:        "7ca66bd33c15ced9c300c76438e8c7d126ee4d114c66de65c59d04ca2cc818b7",
					ProjectName: "default",
				},
			},
			assertLog: log.Empty,
		},
		{
			name:              "handle event - cancelled context",
			getCtx:            cancelledContext,
			blockEventChannel: true,
			handler: func(ready, done chan struct{}) func(w http.ResponseWriter, r *http.Request) {
				return func(w http.ResponseWriter, r *http.Request) {
					conn, err := upgrader.Upgrade(w, r, nil)
					if err != nil {
						t.Errorf("Failed to upgrade websocket connection: %v", err)
						return
					}

					defer conn.Close()

					<-ready

					// Since channel read operations in select pick a random case, we might
					// need multiple attempts until hitting the ctx.Done() case.
				loop:
					for {
						err = conn.WriteJSON(createLifecycleEvent(t, incusapi.EventLifecycleImageCreated))
						if err != nil {
							t.Errorf("Failed to write event: %v", err)
							return
						}

						select {
						case <-done:
							break loop

						default:
						}
					}
				}
			},
			clientCertPEM: certPEM,
			clientKeyPEM:  keyPEM,

			assertErr:        require.NoError,
			assertErrChanErr: require.NoError,
			assertLog:        log.Empty,
		},
		{
			name:   "error - webserver disconnect websocket immediately",
			getCtx: readyContext,
			handler: func(ready, done chan struct{}) func(w http.ResponseWriter, r *http.Request) {
				return func(w http.ResponseWriter, r *http.Request) {
					conn, err := upgrader.Upgrade(w, r, nil)
					if err != nil {
						t.Errorf("Failed to upgrade websocket connection: %v", err)
						return
					}

					defer conn.Close()

					// End the websocket connection right after it is established.
				}
			},
			clientCertPEM: certPEM,
			clientKeyPEM:  keyPEM,

			assertErr:        require.NoError,
			assertErrChanErr: require.Error,
			assertLog:        log.Empty,
		},
	}

	var httpHandler http.HandlerFunc

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpHandler(w, r)
	}))
	server.TLS = &tls.Config{
		NextProtos: []string{"h2", "http/1.1"},
		ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs:  caPool,
	}

	server.StartTLS()
	defer server.Close()

	serverCert := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: server.Certificate().Raw,
	})

	target := provisioning.Server{
		ConnectionURL: server.URL,
		Certificate:   new(string(serverCert)),
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			ready := make(chan struct{})
			done := make(chan struct{})

			logBuf := &log.SyncBuffer{}
			ctx, cancel := tc.getCtx(logger.ContextWithTestLogger(t, logBuf, false, false))
			defer cancel()

			httpHandler = tc.handler(ready, done)

			client := incus.New(tc.clientCertPEM, tc.clientKeyPEM, incus.WithSkipGetServer(true))

			// Run test
			events, errChan, err := client.SubscribeLifecycleEvents(ctx, target)
			tc.assertErr(t, err)
			if err != nil {
				// we don't have any websocket connection at this point, so skip the rest of the assertions.
				return
			}

			if tc.blockEventChannel {
				events = nil
			}

			var event domain.LifecycleEvent
			var errChanErr error

			close(ready)

			tick := time.NewTicker(200 * time.Millisecond)
			defer tick.Stop()

			select {
			case event = <-events:
			case errChanErr = <-errChan:
			case <-ctx.Done():
			case <-t.Context().Done():
				t.Fatal("Test context cancelled before test ended")

			case <-tick.C:
				t.Error("Test timeout reached before test ended")
			}

			close(done)

			// Assert
			tc.assertErrChanErr(t, errChanErr)
			require.Equal(t, tc.wantEvent, event)
			tc.assertLog(t, logBuf)
		})
	}
}

func createLifecycleEvent(t *testing.T, action string) incusapi.Event {
	t.Helper()

	data, err := json.Marshal(incusapi.EventLifecycle{
		Action: action,
		Source: "/1.0/images/7ca66bd33c15ced9c300c76438e8c7d126ee4d114c66de65c59d04ca2cc818b7",
		Context: map[string]any{
			"type": "container",
		},
		Requestor: nil,
		Name:      "",
		Project:   "",
	})
	require.NoError(t, err)

	return incusapi.Event{
		Type:      incusapi.EventTypeLifecycle,
		Timestamp: time.Date(2025, 10, 30, 17, 5, 0, 0, time.UTC),
		Metadata:  json.RawMessage(data),
		Location:  "none",
		Project:   "default",
	}
}
