package terraform

import (
	"testing"

	incusapi "github.com/lxc/incus/v6/shared/api"
	"github.com/stretchr/testify/require"
)

func Test_incusPreseedWithDefaults(t *testing.T) {
	tests := []struct {
		name   string
		config map[string]any

		assertErr require.ErrorAssertionFunc
		want      incusapi.InitLocalPreseed
	}{
		{
			name:   "success - nil",
			config: nil,

			assertErr: require.NoError,
			want: incusapi.InitLocalPreseed{
				ServerPut: incusapi.ServerPut{
					Config: incusapi.ConfigMap{
						"storage.backups_volume": "local/backups",
						"storage.images_volume":  "local/images",
					},
				},

				Networks: []incusapi.InitNetworksProjectPost{
					{
						NetworksPost: incusapi.NetworksPost{
							NetworkPut: incusapi.NetworkPut{
								Description: "Local network bridge (NAT)",
							},
							Name: "incusbr0",
							Type: "bridge",
						},
					},
				},
				StoragePools: []incusapi.StoragePoolsPost{
					{
						StoragePoolPut: incusapi.StoragePoolPut{
							Config: incusapi.ConfigMap{
								"source": "local/incus",
							},
							Description: "Local storage pool (on system drive)",
						},
						Name:   "local",
						Driver: "zfs",
					},
				},
				StorageVolumes: []incusapi.InitStorageVolumesProjectPost{
					{
						StorageVolumesPost: incusapi.StorageVolumesPost{
							StorageVolumePut: incusapi.StorageVolumePut{
								Description: "Volume holding system backups",
							},
							Name:        "backups",
							Type:        "custom",
							ContentType: "filesystem",
						},
						Pool: "local",
					},
					{
						StorageVolumesPost: incusapi.StorageVolumesPost{
							StorageVolumePut: incusapi.StorageVolumePut{
								Description: "Volume holding system images",
							},
							Name:        "images",
							Type:        "custom",
							ContentType: "filesystem",
						},
						Pool: "local",
					},
				},
				Profiles: []incusapi.InitProfileProjectPost{
					{
						ProfilesPost: incusapi.ProfilesPost{
							ProfilePut: incusapi.ProfilePut{
								Devices: map[string]map[string]string{
									"eth0": {
										"network": "incusbr0",
										"type":    "nic",
									},
									"root": {
										"path": "/",
										"pool": "local",
										"type": "disk",
									},
								},
							},
							Name: "default",
						},
					},
					{
						ProfilesPost: incusapi.ProfilesPost{
							ProfilePut: incusapi.ProfilePut{
								Devices: map[string]map[string]string{
									"eth0": {
										"network": "meshbr0",
										"type":    "nic",
									},
									"root": {
										"path": "/",
										"pool": "local",
										"type": "disk",
									},
								},
							},
							Name: "default",
						},
						Project: "internal",
					},
				},
				Projects: []incusapi.ProjectsPost{
					{
						ProjectPut: incusapi.ProjectPut{
							Description: "Internal project to isolate fully managed resources.",
						},
						Name: "internal",
					},
				},
			},
		},
		{
			name: "success - with config",
			config: map[string]any{
				"storage_pools": []any{
					map[string]any{
						"name":   "local",
						"driver": "zfs",
					},
				},
				"projects": []any{
					map[string]any{
						"name": "internal",
					},
				},
				"networks": []any{
					map[string]any{
						"name": "incusbr0",
						"type": "bridge",
					},
					map[string]any{
						"name": "meshbr0",
					},
				},
				"storage_volumes": []any{
					map[string]any{
						"pool":         "local",
						"name":         "backups",
						"type":         "custom",
						"content_type": "filesystem",
					},
					map[string]any{
						"pool":         "local",
						"name":         "images",
						"type":         "custom",
						"content_type": "filesystem",
					},
				},
				"profiles": []any{
					map[string]any{
						"project": "",
						"name":    "default",
					},
					map[string]any{
						"project": "internal",
						"name":    "default",
					},
				},
			},

			assertErr: require.NoError,
			want: incusapi.InitLocalPreseed{
				ServerPut: incusapi.ServerPut{
					Config: incusapi.ConfigMap{
						"storage.backups_volume": "local/backups",
						"storage.images_volume":  "local/images",
					},
				},

				Networks: []incusapi.InitNetworksProjectPost{
					{
						NetworksPost: incusapi.NetworksPost{
							NetworkPut: incusapi.NetworkPut{
								Description: "Local network bridge (NAT)",
							},
							Name: "incusbr0",
							Type: "bridge",
						},
					},
				},
				StoragePools: []incusapi.StoragePoolsPost{
					{
						StoragePoolPut: incusapi.StoragePoolPut{
							Config: incusapi.ConfigMap{
								"source": "local/incus",
							},
							Description: "Local storage pool (on system drive)",
						},
						Name:   "local",
						Driver: "zfs",
					},
				},
				StorageVolumes: []incusapi.InitStorageVolumesProjectPost{
					{
						StorageVolumesPost: incusapi.StorageVolumesPost{
							StorageVolumePut: incusapi.StorageVolumePut{
								Description: "Volume holding system backups",
							},
							Name:        "backups",
							Type:        "custom",
							ContentType: "filesystem",
						},
						Pool: "local",
					},
					{
						StorageVolumesPost: incusapi.StorageVolumesPost{
							StorageVolumePut: incusapi.StorageVolumePut{
								Description: "Volume holding system images",
							},
							Name:        "images",
							Type:        "custom",
							ContentType: "filesystem",
						},
						Pool: "local",
					},
				},
				Profiles: []incusapi.InitProfileProjectPost{
					{
						ProfilesPost: incusapi.ProfilesPost{
							ProfilePut: incusapi.ProfilePut{
								Devices: map[string]map[string]string{
									"eth0": {
										"network": "incusbr0",
										"type":    "nic",
									},
									"root": {
										"path": "/",
										"pool": "local",
										"type": "disk",
									},
								},
							},
							Name: "default",
						},
					},
					{
						ProfilesPost: incusapi.ProfilesPost{
							ProfilePut: incusapi.ProfilePut{
								Devices: map[string]map[string]string{
									"eth0": {
										"network": "meshbr0",
										"type":    "nic",
									},
									"root": {
										"path": "/",
										"pool": "local",
										"type": "disk",
									},
								},
							},
							Name: "default",
						},
						Project: "internal",
					},
				},
				Projects: []incusapi.ProjectsPost{
					{
						ProjectPut: incusapi.ProjectPut{
							Description: "Internal project to isolate fully managed resources.",
						},
						Name: "internal",
					},
				},
			},
		},
		{
			name: "error - invalid config",
			config: map[string]any{
				"func": func() {},
			},

			assertErr: require.Error,
			want:      incusapi.InitLocalPreseed{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := incusPreseedWithDefaults(tc.config)

			tc.assertErr(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
