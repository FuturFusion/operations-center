package incus

import (
	"encoding/json"
	"testing"
	"time"

	incusapi "github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
)

func Test_mapIncusEventToLifecycleEvent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		event incusapi.Event

		assertErr            require.ErrorAssertionFunc
		wantIsLifecycleEvent bool
		wantEvent            domain.LifecycleEvent
	}{
		{
			name: "success - create image",
			event: func() incusapi.Event {
				data, err := json.Marshal(incusapi.EventLifecycle{
					Action: incusapi.EventLifecycleImageCreated,
					Source: "/1.0/images/7ca66bd33c15ced9c300c76438e8c7d126ee4d114c66de65c59d04ca2cc818b7",
					Context: map[string]any{
						"type": "container",
					},
					Name:    "",
					Project: "",
				})
				require.NoError(t, err)

				return incusapi.Event{
					Type:      incusapi.EventTypeLifecycle,
					Timestamp: time.Date(2025, 10, 30, 17, 5, 0, 0, time.UTC),
					Metadata:  json.RawMessage(data),
					Location:  "none",
					Project:   "default",
				}
			}(),

			assertErr:            require.NoError,
			wantIsLifecycleEvent: true,
			wantEvent: domain.LifecycleEvent{
				Operation:            domain.LifecycleOperationCreate,
				ResourceType:         domain.ResourceTypeImage,
				LifecycleEventAction: "image-created",
				Source: domain.LifecycleSource{
					Name:        "7ca66bd33c15ced9c300c76438e8c7d126ee4d114c66de65c59d04ca2cc818b7",
					ProjectName: "default",
				},
			},
		},
		{
			name: "success - rename instance",
			event: func() incusapi.Event {
				data, err := json.Marshal(incusapi.EventLifecycle{
					Action: incusapi.EventLifecycleInstanceRenamed,
					Source: "/1.0/instances/name-new",
					Context: map[string]any{
						"old_name": "name-old",
					},
					Name:    "name-new",
					Project: "default",
				})
				require.NoError(t, err)

				return incusapi.Event{
					Type:      incusapi.EventTypeLifecycle,
					Timestamp: time.Date(2025, 10, 30, 17, 5, 0, 0, time.UTC),
					Metadata:  json.RawMessage(data),
					Location:  "none",
					Project:   "default",
				}
			}(),

			assertErr:            require.NoError,
			wantIsLifecycleEvent: true,
			wantEvent: domain.LifecycleEvent{
				Operation:            domain.LifecycleOperationRename,
				ResourceType:         domain.ResourceTypeInstance,
				LifecycleEventAction: "instance-renamed",
				Source: domain.LifecycleSource{
					Name:        "name-new",
					ProjectName: "default",
					OldName:     "name-old",
				},
			},
		},
		{
			name: "success - delete storage-volume",
			event: func() incusapi.Event {
				data, err := json.Marshal(incusapi.EventLifecycle{
					Action:  incusapi.EventLifecycleStorageVolumeDeleted,
					Source:  "/1.0/storage-pools/default/volumes/images/7ca66bd33c15ced9c300c76438e8c7d126ee4d114c66de65c59d04ca2cc818b7",
					Name:    "",
					Project: "",
				})
				require.NoError(t, err)

				return incusapi.Event{
					Type:      incusapi.EventTypeLifecycle,
					Timestamp: time.Date(2025, 10, 30, 17, 5, 0, 0, time.UTC),
					Metadata:  json.RawMessage(data),
					Location:  "none",
					Project:   "default",
				}
			}(),

			assertErr:            require.NoError,
			wantIsLifecycleEvent: true,
			wantEvent: domain.LifecycleEvent{
				Operation:            domain.LifecycleOperationDelete,
				ResourceType:         domain.ResourceTypeStorageVolume,
				LifecycleEventAction: "storage-volume-deleted",
				Source: domain.LifecycleSource{
					Name:        "7ca66bd33c15ced9c300c76438e8c7d126ee4d114c66de65c59d04ca2cc818b7",
					ParentType:  "storage-pool",
					ParentName:  "default",
					Type:        "images",
					ProjectName: "default",
				},
			},
		},
		{
			name: "success - not a lifecycle event",
			event: func() incusapi.Event {
				return incusapi.Event{
					Type:      incusapi.EventTypeLogging,
					Timestamp: time.Date(2025, 10, 30, 17, 5, 0, 0, time.UTC),
					Metadata:  json.RawMessage(nil),
					Location:  "none",
					Project:   "default",
				}
			}(),

			assertErr:            require.NoError,
			wantIsLifecycleEvent: false,
			wantEvent:            domain.LifecycleEvent{},
		},
		{
			name: "error - invalid lifecycle metadata",
			event: func() incusapi.Event {
				return incusapi.Event{
					Type:      incusapi.EventTypeLifecycle,
					Timestamp: time.Date(2025, 10, 30, 17, 5, 0, 0, time.UTC),
					Metadata:  json.RawMessage(`[]`), // array is invalid for event lifecycle metadata.
					Location:  "none",
					Project:   "default",
				}
			}(),

			assertErr:            require.Error,
			wantIsLifecycleEvent: false,
			wantEvent:            domain.LifecycleEvent{},
		},
		{
			name: "success - not mapped lifecycle action",
			event: func() incusapi.Event {
				data, err := json.Marshal(incusapi.EventLifecycle{
					Action: "warning-reset", // not mapped lifecycle action
				})
				require.NoError(t, err)

				return incusapi.Event{
					Type:      incusapi.EventTypeLifecycle,
					Timestamp: time.Date(2025, 10, 30, 17, 5, 0, 0, time.UTC),
					Metadata:  json.RawMessage(data),
					Location:  "none",
					Project:   "default",
				}
			}(),

			assertErr:            require.NoError,
			wantIsLifecycleEvent: false,
			wantEvent:            domain.LifecycleEvent{},
		},
		{
			name: "error - invalid source URL",
			event: func() incusapi.Event {
				data, err := json.Marshal(incusapi.EventLifecycle{
					Action: incusapi.EventLifecycleInstanceCreated,
					Source: ":|//", // invalid URL
				})
				require.NoError(t, err)

				return incusapi.Event{
					Type:      incusapi.EventTypeLifecycle,
					Timestamp: time.Date(2025, 10, 30, 17, 5, 0, 0, time.UTC),
					Metadata:  json.RawMessage(data), // array is invalid for event lifecycle metadata.
					Location:  "none",
					Project:   "default",
				}
			}(),

			assertErr:            require.Error,
			wantIsLifecycleEvent: false,
			wantEvent:            domain.LifecycleEvent{},
		},
		{
			name: "success - delete storage-volume - with invalid type form context",
			event: func() incusapi.Event {
				data, err := json.Marshal(incusapi.EventLifecycle{
					Action: incusapi.EventLifecycleStorageVolumeDeleted,
					Source: "/1.0/storage-pools/default/volumes/images/7ca66bd33c15ced9c300c76438e8c7d126ee4d114c66de65c59d04ca2cc818b7",
					Context: map[string]any{
						"type": true, // invalid, string expected
					},
				})
				require.NoError(t, err)

				return incusapi.Event{
					Type:      incusapi.EventTypeLifecycle,
					Timestamp: time.Date(2025, 10, 30, 17, 5, 0, 0, time.UTC),
					Metadata:  json.RawMessage(data),
					Location:  "none",
					Project:   "default",
				}
			}(),

			assertErr:            require.NoError,
			wantIsLifecycleEvent: true,
			wantEvent: domain.LifecycleEvent{
				Operation:            domain.LifecycleOperationDelete,
				ResourceType:         domain.ResourceTypeStorageVolume,
				LifecycleEventAction: "storage-volume-deleted",
				Source: domain.LifecycleSource{
					Name:        "7ca66bd33c15ced9c300c76438e8c7d126ee4d114c66de65c59d04ca2cc818b7",
					ParentType:  "storage-pool",
					ParentName:  "default",
					Type:        "images",
					ProjectName: "default",
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Run test
			event, isLifecycleEvent, err := mapIncusEventToLifecycleEvent(t.Context(), tc.event)

			tc.assertErr(t, err)
			require.Equal(t, tc.wantIsLifecycleEvent, isLifecycleEvent)
			require.Equal(t, tc.wantEvent, event)
		})
	}
}

func Test_firstNonEmpty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		inputs []string

		want string
	}{
		{
			name: "first",
			inputs: []string{
				"first",
				"",
				"last",
			},

			want: "first",
		},
		{
			name: "last",
			inputs: []string{
				"",
				"last",
			},

			want: "last",
		},
		{
			name:   "default from nil",
			inputs: nil,

			want: "",
		},
		{
			name: "default from only empty string",
			inputs: []string{
				"",
				"",
			},

			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := firstNonEmpty(tc.inputs...)

			require.Equal(t, tc.want, got)
		})
	}
}
