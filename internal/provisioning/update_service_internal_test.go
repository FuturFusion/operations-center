package provisioning

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/testing/uuidgen"
	"github.com/FuturFusion/operations-center/shared/api"
)

func Test_updateService_determineToDeleteAndToDownloadUpdates(t *testing.T) {
	dateTime1 := time.Date(2025, 8, 21, 13, 4, 0, 0, time.UTC)
	dateTime2 := time.Date(2025, 8, 22, 13, 4, 0, 0, time.UTC)
	dateTime3 := time.Date(2025, 8, 23, 13, 4, 0, 0, time.UTC)
	dateTime4 := time.Date(2025, 8, 24, 13, 4, 0, 0, time.UTC)
	dateTime5 := time.Date(2025, 8, 25, 13, 4, 0, 0, time.UTC)
	dateTime6 := time.Date(2025, 8, 26, 13, 4, 0, 0, time.UTC)

	tests := []struct {
		name          string
		dbUpdates     Updates
		originUpdates Updates

		wantToDeleteUpdates   []Update
		wantToDownloadUpdates []Update
	}{
		{
			name:          "nothing",
			dbUpdates:     Updates{},
			originUpdates: Updates{},

			wantToDeleteUpdates:   []Update{},
			wantToDownloadUpdates: []Update{},
		},
		{
			// This case has been caused by changing the definition of the UUID,
			// which caused new UUID for all updates. So the DB contained the same
			// updates as the origin, but they had different UUID.
			name: "updates with same date but different UUID",
			dbUpdates: Updates{
				{
					UUID:        uuidgen.FromPattern(t, "01"),
					PublishedAt: dateTime1,
					Status:      api.UpdateStatusReady,
				},
				{
					UUID:        uuidgen.FromPattern(t, "02"),
					PublishedAt: dateTime2,
					Status:      api.UpdateStatusReady,
				},
				{
					UUID:        uuidgen.FromPattern(t, "03"),
					PublishedAt: dateTime3,
					Status:      api.UpdateStatusReady,
				},
			},
			originUpdates: Updates{
				{
					UUID:        uuidgen.FromPattern(t, "04"),
					PublishedAt: dateTime1,
					Status:      api.UpdateStatusUnknown,
				},
				{
					UUID:        uuidgen.FromPattern(t, "05"),
					PublishedAt: dateTime2,
					Status:      api.UpdateStatusUnknown,
				},
				{
					UUID:        uuidgen.FromPattern(t, "06"),
					PublishedAt: dateTime3,
					Status:      api.UpdateStatusUnknown,
				},
			},

			wantToDeleteUpdates: []Update{
				{
					UUID:        uuidgen.FromPattern(t, "01"),
					PublishedAt: dateTime1,
					Status:      api.UpdateStatusReady,
				},
			},
			wantToDownloadUpdates: []Update{
				{
					UUID:        uuidgen.FromPattern(t, "06"),
					PublishedAt: dateTime3,
					Status:      api.UpdateStatusUnknown,
				},
			},
		},
		{
			name: "all updates from origin are newer",
			dbUpdates: Updates{
				{
					UUID:        uuidgen.FromPattern(t, "01"),
					PublishedAt: dateTime1,
					Status:      api.UpdateStatusReady,
				},
				{
					UUID:        uuidgen.FromPattern(t, "02"),
					PublishedAt: dateTime2,
					Status:      api.UpdateStatusReady,
				},
				// Even though update 4 is newer, we always keep the most recent
				// update from DB.
				{
					UUID:        uuidgen.FromPattern(t, "03"),
					PublishedAt: dateTime3,
					Status:      api.UpdateStatusReady,
				},
			},
			originUpdates: Updates{
				// Even though update 4 is newer than update 3, we always keep 1 update
				// from DB and therefore update 4 is not downloaded.
				{
					UUID:        uuidgen.FromPattern(t, "04"),
					PublishedAt: dateTime4,
					Status:      api.UpdateStatusUnknown,
				},
				{
					UUID:        uuidgen.FromPattern(t, "05"),
					PublishedAt: dateTime5,
					Status:      api.UpdateStatusUnknown,
				},
				{
					UUID:        uuidgen.FromPattern(t, "06"),
					PublishedAt: dateTime6,
					Status:      api.UpdateStatusUnknown,
				},
			},

			wantToDeleteUpdates: []Update{
				{
					UUID:        uuidgen.FromPattern(t, "01"),
					PublishedAt: dateTime1,
					Status:      api.UpdateStatusReady,
				},
				{
					UUID:        uuidgen.FromPattern(t, "02"),
					PublishedAt: dateTime2,
					Status:      api.UpdateStatusReady,
				},
			},
			wantToDownloadUpdates: []Update{
				{
					UUID:        uuidgen.FromPattern(t, "05"),
					PublishedAt: dateTime5,
					Status:      api.UpdateStatusUnknown,
				},
				{
					UUID:        uuidgen.FromPattern(t, "06"),
					PublishedAt: dateTime6,
					Status:      api.UpdateStatusUnknown,
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			updateSvc := updateService{
				latestLimit:        3,
				pendingGracePeriod: 24 * time.Hour,
			}

			// Run test
			toDeleteUpdates, toDownloadUpdates := updateSvc.determineToDeleteAndToDownloadUpdates(tc.dbUpdates, tc.originUpdates)

			// Assert
			require.ElementsMatch(t, tc.wantToDeleteUpdates, toDeleteUpdates)
			require.ElementsMatch(t, tc.wantToDownloadUpdates, toDownloadUpdates)
		})
	}
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
