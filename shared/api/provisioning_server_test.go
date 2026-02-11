package api_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/ptr"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestServerVersionData_Value(t *testing.T) {
	svd := api.ServerVersionData{
		OS: api.OSVersionData{
			Name:             "os",
			Version:          "123",
			VersionNext:      "234",
			AvailableVersion: ptr.To("345"),
			NeedsReboot:      true,
			NeedsUpdate:      ptr.To(true),
		},
		Applications: []api.ApplicationVersionData{
			{
				Name:             "app",
				Version:          "123",
				AvailableVersion: ptr.To("234"),
				NeedsUpdate:      ptr.To(true),
			},
		},
		UpdateChannel: "stable",
	}

	val, err := svd.Value()
	require.NoError(t, err)

	require.JSONEq(t, `{"applications":[{"in_maintenance":false,"name":"app","version":"123"}],"os":{"name":"os","version":"123","version_next":"234","needs_reboot":true},"update_channel":"stable"}`, string(val.([]byte)))

	var svdNew api.ServerVersionData
	err = svdNew.Scan(val.([]byte))
	require.NoError(t, err)

	require.Equal(t, api.ServerVersionData{
		OS: api.OSVersionData{
			Name:        "os",
			Version:     "123",
			VersionNext: "234",
			NeedsReboot: true,
		},
		Applications: []api.ApplicationVersionData{
			{
				Name:    "app",
				Version: "123",
			},
		},
		UpdateChannel: "stable",
	}, svdNew)
}
