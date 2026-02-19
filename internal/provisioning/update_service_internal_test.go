package provisioning

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lxc/incus-os/incus-osd/api/images"
	"github.com/stretchr/testify/require"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	envMock "github.com/FuturFusion/operations-center/internal/environment/mock"
	"github.com/FuturFusion/operations-center/internal/util/testing/uuidgen"
	"github.com/FuturFusion/operations-center/shared/api"
)

func Test_updateService_determineToDeleteAndToDownloadUpdates(t *testing.T) {
	dateTime1 := time.Date(2025, 8, 21, 13, 4, 0, 0, time.UTC)
	dateTime2 := time.Date(2025, 8, 22, 13, 4, 0, 0, time.UTC)
	dateTime3 := time.Date(2025, 8, 23, 13, 4, 0, 0, time.UTC)
	dateTime4 := time.Date(2025, 8, 24, 13, 4, 0, 0, time.UTC)
	dateTime5 := time.Date(2025, 8, 25, 13, 4, 0, 0, time.UTC)
	dateTime6 := time.Date(2025, 8, 26, 13, 4, 0, 0, time.UTC)
	dateTime7 := time.Date(2025, 8, 27, 13, 4, 0, 0, time.UTC)

	allComponents := make([]images.UpdateFileComponent, 0, len(images.UpdateFileComponents))
	for component := range images.UpdateFileComponents {
		allComponents = append(allComponents, component)
	}

	tests := []struct {
		name          string
		dbUpdates     Updates
		originUpdates Updates

		wantToDeleteIDs   []string
		wantToDownloadIDs []string
	}{
		{
			name:          "nothing",
			dbUpdates:     Updates{},
			originUpdates: Updates{},

			wantToDeleteIDs:   []string{},
			wantToDownloadIDs: []string{},
		},
		{
			// This case has been caused by changing the definition of the UUID,
			// which caused new UUID for all updates. So the DB contained the same
			// updates as the origin, but they had different UUID.
			name: "updates with same date but different UUID",
			dbUpdates: Updates{
				makeUpdate(t,
					"01",
					dateTime1,
					api.UpdateStatusReady,
					[]string{"stable"},
					allComponents,
				),
				makeUpdate(t,
					"02",
					dateTime2,
					api.UpdateStatusReady,
					[]string{"stable"},
					allComponents,
				),
				makeUpdate(t,
					"03",
					dateTime3,
					api.UpdateStatusReady,
					[]string{"stable"},
					allComponents,
				),
			},
			originUpdates: Updates{
				makeUpdate(t,
					"04",
					dateTime1,
					api.UpdateStatusUnknown,
					[]string{"stable"},
					allComponents,
				),
				makeUpdate(t,
					"05",
					dateTime2,
					api.UpdateStatusUnknown,
					[]string{"stable"},
					allComponents,
				),
				makeUpdate(t,
					"06",
					dateTime3,
					api.UpdateStatusUnknown,
					[]string{"stable"},
					allComponents,
				),
			},

			wantToDeleteIDs: []string{
				"01",
			},
			wantToDownloadIDs: []string{
				"06",
			},
		},
		{
			name: "all updates from origin are newer",
			dbUpdates: Updates{
				makeUpdate(t,
					"01",
					dateTime1,
					api.UpdateStatusReady,
					[]string{"stable"},
					allComponents,
				),
				makeUpdate(t,
					"02",
					dateTime2,
					api.UpdateStatusReady,
					[]string{"stable"},
					allComponents,
				),
				// Even though update 4 is newer, we always keep the most recent
				// update from DB.
				makeUpdate(t,
					"03",
					dateTime3,
					api.UpdateStatusReady,
					[]string{"stable"},
					allComponents,
				),
			},
			originUpdates: Updates{
				// Even though update 4 is newer than update 3, we always keep 1 update
				// from DB and therefore update 4 is not downloaded.
				makeUpdate(t,
					"04",
					dateTime4,
					api.UpdateStatusUnknown,
					[]string{"stable"},
					allComponents,
				),
				makeUpdate(t,
					"05",
					dateTime5,
					api.UpdateStatusUnknown,
					[]string{"stable"},
					allComponents,
				),
				makeUpdate(t,
					"06",
					dateTime6,
					api.UpdateStatusUnknown,
					[]string{"stable"},
					allComponents,
				),
			},

			wantToDeleteIDs: []string{
				"01",
				"02",
			},
			wantToDownloadIDs: []string{
				"05",
				"06",
			},
		},
		{
			name: "all updates from origin are already present in db",
			dbUpdates: Updates{
				makeUpdate(t,
					"01",
					dateTime1,
					api.UpdateStatusReady,
					[]string{"stable"},
					allComponents,
				),
				makeUpdate(t,
					"02",
					dateTime2,
					api.UpdateStatusReady,
					[]string{"stable"},
					allComponents,
				),
				makeUpdate(t,
					"03",
					dateTime3,
					api.UpdateStatusReady,
					[]string{"stable"},
					allComponents,
				),
			},
			originUpdates: Updates{
				makeUpdate(t,
					"01",
					dateTime1,
					api.UpdateStatusReady,
					[]string{"stable"},
					allComponents,
				),
				makeUpdate(t,
					"02",
					dateTime2,
					api.UpdateStatusReady,
					[]string{"stable"},
					allComponents,
				),
				makeUpdate(t,
					"03",
					dateTime3,
					api.UpdateStatusReady,
					[]string{"stable"},
					allComponents,
				),
			},

			wantToDeleteIDs:   []string{},
			wantToDownloadIDs: []string{},
		},
		{
			name: "one pending update in DB for longer than grace time",
			dbUpdates: Updates{
				makeUpdate(t,
					"01",
					dateTime1,
					api.UpdateStatusReady,
					[]string{"stable"},
					allComponents,
				),
				makeUpdate(t,
					"02",
					time.Now().Add(-25*time.Hour), // more than pending grace time
					api.UpdateStatusPending,
					[]string{"stable"},
					allComponents,
				),
			},
			originUpdates: Updates{},

			wantToDeleteIDs: []string{
				"02",
			},
			wantToDownloadIDs: []string{},
		},
		{
			name: "one pending update in DB for shorter than grace time",
			dbUpdates: Updates{
				makeUpdate(t,
					"01",
					dateTime1,
					api.UpdateStatusReady,
					[]string{"stable"},
					allComponents,
				),
				makeUpdate(t,
					"02",
					time.Now().Add(-1*time.Hour), // less than pending grace time
					api.UpdateStatusPending,
					[]string{"stable"},
					allComponents,
				),
			},
			originUpdates: Updates{},

			wantToDeleteIDs:   []string{},
			wantToDownloadIDs: []string{},
		},
		{
			name: "updates with multiple channels",
			// For channel "stable" the two most recent updates (07, 06) are downloaded
			// and one update from the DB is kept (05). Since update 03 is only
			// used by channel "stable" and we already have 3 updates, it is deleted.
			// For channel "prod" the oldest update (01) is deleted, since we still
			// have 3 updates for this channel.
			dbUpdates: Updates{
				makeUpdate(t,
					"01",
					dateTime1,
					api.UpdateStatusReady,
					[]string{"prod"},
					allComponents,
				),
				makeUpdate(t,
					"02",
					dateTime2,
					api.UpdateStatusReady,
					[]string{"stable", "prod"},
					allComponents,
				),
				makeUpdate(t,
					"03",
					dateTime3,
					api.UpdateStatusReady,
					[]string{"stable"},
					allComponents,
				),
				makeUpdate(t,
					"04",
					dateTime4,
					api.UpdateStatusReady,
					[]string{"stable", "prod"},
					allComponents,
				),
				makeUpdate(t,
					"05",
					dateTime5,
					api.UpdateStatusReady,
					[]string{"stable", "prod"},
					allComponents,
				),
			},
			originUpdates: Updates{
				makeUpdate(t,
					"04",
					dateTime4,
					api.UpdateStatusUnknown,
					[]string{"stable"},
					allComponents,
				),
				makeUpdate(t,
					"05",
					dateTime5,
					api.UpdateStatusUnknown,
					[]string{"stable"},
					allComponents,
				),
				makeUpdate(t,
					"06",
					dateTime6,
					api.UpdateStatusUnknown,
					[]string{"stable"},
					allComponents,
				),
				makeUpdate(t,
					"07",
					dateTime7,
					api.UpdateStatusUnknown,
					[]string{"stable"},
					allComponents,
				),
			},

			wantToDeleteIDs: []string{
				"01", // supernumerary update, only used by channel "prod".
				"03", // supernumerary update, only used by channel "stable".
			},
			wantToDownloadIDs: []string{
				"06", // 2nd fresh update for channel "stable".
				"07", // 1st fresh update for channel "stable".
			},
		},
		{
			name: "updates with different components",
			// For channel "stable" the two most recent updates (07, 06) are downloaded.
			// At this point we have two updated for component "incus", but only one
			// update for component "os".
			// Update 05 provides an other component for "incus" as well as "os".
			// Update 04 provides only component "incus", but at this point we already
			// have 3 component "incus", so this update is deleted.
			// Update 03 adds the missing component for "os", so we have enough updates
			// for each component and therefore updates 02 and 01 are deleted.
			dbUpdates: Updates{
				makeUpdate(t,
					"01",
					dateTime1,
					api.UpdateStatusReady,
					[]string{"stable"},
					allComponents,
				),
				makeUpdate(t,
					"02",
					dateTime2,
					api.UpdateStatusReady,
					[]string{"stable"},
					[]images.UpdateFileComponent{
						images.UpdateFileComponentIncus,
					},
				),
				makeUpdate(t,
					"03",
					dateTime3,
					api.UpdateStatusReady,
					[]string{"stable"},
					allComponents,
				),
				makeUpdate(t,
					"04",
					dateTime4,
					api.UpdateStatusReady,
					[]string{"stable"},
					[]images.UpdateFileComponent{
						images.UpdateFileComponentIncus,
					},
				),
				makeUpdate(t,
					"05",
					dateTime5,
					api.UpdateStatusReady,
					[]string{"stable"},
					allComponents,
				),
			},
			originUpdates: Updates{
				makeUpdate(t,
					"06",
					dateTime6,
					api.UpdateStatusUnknown,
					[]string{"stable"},
					[]images.UpdateFileComponent{
						images.UpdateFileComponentIncus,
					},
				),
				makeUpdate(t,
					"07",
					dateTime7,
					api.UpdateStatusUnknown,
					[]string{"stable"},
					allComponents,
				),
			},

			wantToDeleteIDs: []string{
				"01", // supernumerary update
				"02", // supernumerary update
				"04", // supernumerary update, does only provide component Incus
			},
			wantToDownloadIDs: []string{
				"06", // 2nd fresh update for channel "stable".
				"07", // 1st fresh update for channel "stable".
			},
		},
	}

	// Make sure, we have UpdatesDefaultChannel populated correctly in config.
	config.InitTest(t, &envMock.EnvironmentMock{}, nil)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			updateSvc := updateService{
				latestLimit:        3,
				pendingGracePeriod: 24 * time.Hour,
			}

			wantToDeleteUpdateIDs := make([]uuid.UUID, 0, len(tc.wantToDeleteIDs))
			for _, id := range tc.wantToDeleteIDs {
				wantToDeleteUpdateIDs = append(wantToDeleteUpdateIDs, uuidgen.FromPattern(t, id))
			}

			wantToDownloadUpdateIDs := make([]uuid.UUID, 0, len(tc.wantToDownloadIDs))
			for _, id := range tc.wantToDownloadIDs {
				wantToDownloadUpdateIDs = append(wantToDownloadUpdateIDs, uuidgen.FromPattern(t, id))
			}

			// Run test
			toDeleteUpdates, toDownloadUpdates := updateSvc.determineToDeleteAndToDownloadUpdates(tc.dbUpdates, tc.originUpdates)

			toDeleteUpdateIDs := make([]uuid.UUID, 0, len(toDeleteUpdates))
			for _, update := range toDeleteUpdates {
				toDeleteUpdateIDs = append(toDeleteUpdateIDs, update.UUID)
			}

			toDownloadUpdateIDs := make([]uuid.UUID, 0, len(toDownloadUpdates))
			for _, update := range toDownloadUpdates {
				toDownloadUpdateIDs = append(toDownloadUpdateIDs, update.UUID)
			}

			// Assert
			require.ElementsMatch(t, wantToDeleteUpdateIDs, toDeleteUpdateIDs)
			require.ElementsMatch(t, wantToDownloadUpdateIDs, toDownloadUpdateIDs)
		})
	}
}

// makeUpdate returns realistically initialized Update records for testing.
func makeUpdate(
	t *testing.T,
	uuidPattern string,
	publishedAndUpdatedAt time.Time,
	status api.UpdateStatus,
	channels []string,
	components []images.UpdateFileComponent,
) Update {
	t.Helper()

	update := Update{
		UUID:        uuidgen.FromPattern(t, uuidPattern),
		PublishedAt: publishedAndUpdatedAt,
		LastUpdated: publishedAndUpdatedAt,
		Status:      status,
		Channels:    channels,
	}

	for _, component := range components {
		update.Files = append(update.Files,
			UpdateFile{
				Component: component,
				Filename:  string(component) + ".raw.gz",
			},
			UpdateFile{
				Component: component,
				Filename:  string(component) + ".manifest.json.gz",
			},
		)
	}

	return update
}

func TestUpdateService_validateUpdatesConfig(t *testing.T) {
	tests := []struct {
		name                 string
		filterExpression     string
		fileFilterExpression string

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:                 "success",
			filterExpression:     "",
			fileFilterExpression: "",

			assertErr: require.NoError,
		},
		{
			name:                 "error - invalid filter expression",
			filterExpression:     `invalid`, // invalid,
			fileFilterExpression: "",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Invalid config, failed to compile filter expression`)
			},
		},
		{
			name:                 "error - invalid file filter expression",
			filterExpression:     "",
			fileFilterExpression: `invalid`, // invalid,

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Invalid config, failed to compile file filter expression`)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			updateSvc := NewUpdateService(nil, nil, nil)

			err := updateSvc.validateUpdatesConfig(t.Context(), api.SystemUpdates{
				SystemUpdatesPut: api.SystemUpdatesPut{
					FilterExpression:     tc.filterExpression,
					FileFilterExpression: tc.fileFilterExpression,
				},
			})

			tc.assertErr(t, err)
		})
	}
}
