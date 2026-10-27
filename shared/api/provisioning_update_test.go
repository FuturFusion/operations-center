package api_test

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/shared/api"
)

func TestUpdateFilter_AppendToURLValues(t *testing.T) {
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

			require.Equal(t, tc.want, query.Encode())
		})
	}
}
