package api_test

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/testing/errassert"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestUpdateFilter_URLValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		filter api.UpdateFilter

		want string
	}{
		{
			name:   "empty filter",
			filter: api.UpdateFilter{},

			want: ``,
		},
		{
			name: "complete filter",
			filter: api.UpdateFilter{
				UpstreamChannel: new("channel"),
				Channel:         new("update-channel"),
				Origin:          new("origin"),
				Status:          new(api.UpdateStatusReady),
			},

			want: `channel=channel&origin=origin&status=ready&update_channel=update-channel`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			query := tc.filter.AppendToURLValues(url.Values{})
			filter, err := api.UpdateFilterFromURLValues(query)

			require.NoError(t, err)
			require.Equal(t, tc.want, query.Encode(), "the filter is encoded as query parameters")
			require.Equal(t, tc.filter, filter, "parsing the query returns the original filter")
		})
	}
}

func TestUpdateFilterFromURLValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		query url.Values

		assertErr require.ErrorAssertionFunc
		want      api.UpdateFilter
	}{
		{
			name:  "empty query",
			query: url.Values{},

			assertErr: require.NoError,
			want:      api.UpdateFilter{},
		},
		{
			name: "empty values are ignored",
			query: url.Values{
				"channel":        []string{""},
				"update_channel": []string{""},
				"origin":         []string{""},
				"status":         []string{""},
			},

			assertErr: require.NoError,
			want:      api.UpdateFilter{},
		},
		{
			name: "error - invalid status",
			query: url.Values{
				"status": []string{"invalid"},
			},

			assertErr: errassert.ValidationErrorContains(`Invalid value "invalid" for query parameter "status"`),
			want:      api.UpdateFilter{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			filter, err := api.UpdateFilterFromURLValues(tc.query)

			tc.assertErr(t, err)
			require.Equal(t, tc.want, filter)
		})
	}
}
