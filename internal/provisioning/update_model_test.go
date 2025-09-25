package provisioning_test

import (
	"database/sql/driver"
	"sort"
	"testing"

	"github.com/lxc/incus-os/incus-osd/api/images"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/ptr"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestUpdate_Validate(t *testing.T) {
	tests := []struct {
		name   string
		server provisioning.Update

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "valid",
			server: provisioning.Update{
				Severity: api.UpdateSeverityLow,
				Status:   api.UpdateStatusReady,
			},

			assertErr: require.NoError,
		},
		{
			name: "error - severity empty",
			server: provisioning.Update{
				Severity: "", // empty
				Status:   api.UpdateStatusReady,
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - severity invalid",
			server: provisioning.Update{
				Severity: "invalid", // invalid
				Status:   api.UpdateStatusReady,
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - status empty",
			server: provisioning.Update{
				Severity: api.UpdateSeverityLow,
				Status:   "", // empty
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
		{
			name: "error - status invalid",
			server: provisioning.Update{
				Severity: api.UpdateSeverityLow,
				Status:   "invalid", // invalid
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.server.Validate()

			tc.assertErr(t, err)
		})
	}
}

func TestUpdate_Filter(t *testing.T) {
	tests := []struct {
		name   string
		filter provisioning.UpdateFilter

		want string
	}{
		{
			name:   "empty filter",
			filter: provisioning.UpdateFilter{},

			want: ``,
		},
		{
			name: "complete filter",
			filter: provisioning.UpdateFilter{
				Channel: ptr.To("channel"),
				Origin:  ptr.To("origin"),
				Status:  ptr.To(api.UpdateStatusReady),
			},

			want: `channel=channel&origin=origin&status=ready`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.filter.String())
		})
	}
}

func TestUpdatesSort(t *testing.T) {
	tests := []struct {
		name string
		in   provisioning.Updates

		want provisioning.Updates
	}{
		{
			name: "pre-sorted",
			in: provisioning.Updates{
				provisioning.Update{
					ID:      "3",
					Version: "3",
				},
				provisioning.Update{
					ID:      "2",
					Version: "2",
				},
				provisioning.Update{
					ID:      "1",
					Version: "1",
				},
			},
			want: provisioning.Updates{
				provisioning.Update{
					ID:      "3",
					Version: "3",
				},
				provisioning.Update{
					ID:      "2",
					Version: "2",
				},
				provisioning.Update{
					ID:      "1",
					Version: "1",
				},
			},
		},
		{
			name: "sort",
			in: provisioning.Updates{
				provisioning.Update{
					ID:      "2",
					Version: "2",
				},
				provisioning.Update{
					ID:      "1",
					Version: "1",
				},
				provisioning.Update{
					ID:      "3",
					Version: "3",
				},
			},
			want: provisioning.Updates{
				provisioning.Update{
					ID:      "3",
					Version: "3",
				},
				provisioning.Update{
					ID:      "2",
					Version: "2",
				},
				provisioning.Update{
					ID:      "1",
					Version: "1",
				},
			},
		},
		{
			name: "sort dns serial",
			in: provisioning.Updates{
				provisioning.Update{
					ID:      "2",
					Version: "202503010000",
				},
				provisioning.Update{
					ID:      "1",
					Version: "202501010000",
				},
				provisioning.Update{
					ID:      "3",
					Version: "202506010000",
				},
			},
			want: provisioning.Updates{
				provisioning.Update{
					ID:      "3",
					Version: "202506010000",
				},
				provisioning.Update{
					ID:      "2",
					Version: "202503010000",
				},
				provisioning.Update{
					ID:      "1",
					Version: "202501010000",
				},
			},
		},
		{
			name: "not numeric version",
			in: provisioning.Updates{
				provisioning.Update{
					ID:      "2",
					Version: "not numberic",
				},
				provisioning.Update{
					ID:      "1",
					Version: "1",
				},
				provisioning.Update{
					ID:      "3",
					Version: "3",
				},
			},
			want: provisioning.Updates{
				provisioning.Update{
					ID:      "3",
					Version: "3",
				},
				provisioning.Update{
					ID:      "1",
					Version: "1",
				},
				provisioning.Update{
					ID:      "2",
					Version: "not numberic",
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sort.Sort(tc.in)

			require.Equal(t, tc.want, tc.in)
		})
	}
}

func TestUpdateFiles_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name string

		input []byte

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success - empty",

			input: []byte(`[]`),

			assertErr: require.NoError,
		},
		{
			name: "success",

			input: []byte(`[{"filename": "dummy.txt"}]`),

			assertErr: require.NoError,
		},
		{
			name: "error - invalid json",

			input: []byte(`"not an array"`),

			assertErr: require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			updateFiles := provisioning.UpdateFiles{}

			err := updateFiles.UnmarshalJSON(tc.input)

			tc.assertErr(t, err)
		})
	}
}

func TestUpdateFiles_Value(t *testing.T) {
	tests := []struct {
		name string

		updateFiles provisioning.UpdateFiles

		assertErr require.ErrorAssertionFunc
		wantValue driver.Value
	}{
		{
			name: "success",

			updateFiles: provisioning.UpdateFiles{
				{
					Filename:  "dummy.txt",
					Size:      5,
					Component: images.UpdateFileComponentDebug,
				},
			},

			assertErr: require.NoError,
			wantValue: []byte(`[{"filename":"dummy.txt","size":5,"sha256":"","component":"debug","type":"","architecture":""}]`),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.updateFiles.Value()

			tc.assertErr(t, err)
			require.Equal(t, tc.wantValue, got)
		})
	}
}

func TestUpdateFiles_Scan(t *testing.T) {
	tests := []struct {
		name string

		value any

		assertErr require.ErrorAssertionFunc
		want      provisioning.UpdateFiles
	}{
		{
			name: "success - []byte",

			value: []byte(`[{"filename":"dummy.txt","size":5}]`),

			assertErr: require.NoError,
			want: provisioning.UpdateFiles{
				{
					Filename: "dummy.txt",
					Size:     5,
				},
			},
		},
		{
			name: "success - string",

			value: `[{"filename":"dummy.txt","size":5}]`,

			assertErr: require.NoError,
			want: provisioning.UpdateFiles{
				{
					Filename: "dummy.txt",
					Size:     5,
				},
			},
		},
		{
			name: "error - nil",

			assertErr: require.Error,
			want:      provisioning.UpdateFiles{},
		},
		{
			name: "error - unsupported type",

			value: 1, // not supported for UpdateFiles

			assertErr: require.Error,
			want:      provisioning.UpdateFiles{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			updateFiles := provisioning.UpdateFiles{}

			err := updateFiles.Scan(tc.value)

			tc.assertErr(t, err)
			require.Equal(t, tc.want, updateFiles)
		})
	}
}

func TestUpdateChannels_Value(t *testing.T) {
	tests := []struct {
		name string

		updateFiles provisioning.UpdateChannels

		assertErr require.ErrorAssertionFunc
		wantValue driver.Value
	}{
		{
			name: "success",

			updateFiles: provisioning.UpdateChannels{"stable", "daily"},

			assertErr: require.NoError,
			wantValue: `stable,daily`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.updateFiles.Value()

			tc.assertErr(t, err)
			require.Equal(t, tc.wantValue, got)
		})
	}
}

func TestUpdateChannels_Scan(t *testing.T) {
	tests := []struct {
		name string

		value any

		assertErr require.ErrorAssertionFunc
		want      provisioning.UpdateChannels
	}{
		{
			name: "success - []byte",

			value: []byte(`stable,daily`),

			assertErr: require.NoError,
			want:      provisioning.UpdateChannels{"stable", "daily"},
		},
		{
			name: "success - string",

			value: `stable,daily`,

			assertErr: require.NoError,
			want:      provisioning.UpdateChannels{"stable", "daily"},
		},
		{
			name: "error - nil",

			assertErr: require.Error,
			want:      provisioning.UpdateChannels{},
		},
		{
			name: "error - unsupported type",

			value: 1, // not supported for UpdateFiles

			assertErr: require.Error,
			want:      provisioning.UpdateChannels{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			updateChannels := provisioning.UpdateChannels{}

			err := updateChannels.Scan(tc.value)

			tc.assertErr(t, err)
			require.Equal(t, tc.want, updateChannels)
		})
	}
}
