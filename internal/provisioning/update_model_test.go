package provisioning_test

import (
	"database/sql/driver"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/ptr"
	"github.com/FuturFusion/operations-center/shared/api"
)

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
			},

			want: `channel=channel&origin=origin`,
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
					Component: api.UpdateFileComponentDebug,
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
			t.Logf("%s", got)
			require.Equal(t, tc.wantValue, got)
		})
	}
}

func TestUpdateFiles_Scan(t *testing.T) {
	tests := []struct {
		name string

		value any

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success - []byte",

			value: []byte(`[{"filename":"dummy.txt","url":"http://localhost/dummy.txt","size":5}]`),

			assertErr: require.NoError,
		},
		{
			name: "success - string",

			value: `[{"filename":"dummy.txt","url":"http://localhost/dummy.txt","size":5}]`,

			assertErr: require.NoError,
		},
		{
			name: "error - nil",

			assertErr: require.Error,
		},
		{
			name: "success - string",

			value: 1, // not supported for UpdateFiles

			assertErr: require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			updateFiles := provisioning.UpdateFiles{}

			err := updateFiles.Scan(tc.value)

			tc.assertErr(t, err)
		})
	}
}
