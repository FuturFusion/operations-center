package terraform

import (
	"testing"

	incusapi "github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/require"

	securitytls "github.com/FuturFusion/operations-center/internal/security/tls"
	"github.com/FuturFusion/operations-center/internal/util/testing/testcert"
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
						"storage.logs_volume":    "local/logs",
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
					{
						StorageVolumesPost: incusapi.StorageVolumesPost{
							StorageVolumePut: incusapi.StorageVolumePut{
								Description: "Volume holding system logs",
							},
							Name:        "logs",
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
					map[string]any{
						"pool":         "local",
						"name":         "logs",
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
						"storage.logs_volume":    "local/logs",
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
					{
						StorageVolumesPost: incusapi.StorageVolumesPost{
							StorageVolumePut: incusapi.StorageVolumePut{
								Description: "Volume holding system logs",
							},
							Name:        "logs",
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
			got, _, err := incusPreseedWithDefaults(tc.config, nil, nil)

			tc.assertErr(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func Test_incusPreseedWithDefaults_trustedClientCertificates(t *testing.T) {
	tests := []struct {
		name                           string
		config                         map[string]any
		trustedClientCertificates      []string
		knownTrustedClientCertificates []string

		assertErr             require.ErrorAssertionFunc
		wantCertificates      []incusapi.CertificatesPost
		wantKnownCertificates []securitytls.TrustedClientCertificate
	}{
		{
			name:                      "no trusted client certificates",
			config:                    nil,
			trustedClientCertificates: nil,

			assertErr:        require.NoError,
			wantCertificates: nil,
		},
		{
			name:                      "trusted client certificates are added",
			config:                    nil,
			trustedClientCertificates: []string{testcert.ClientCertificate},

			assertErr: require.NoError,
			wantCertificates: []incusapi.CertificatesPost{
				{
					CertificatePut: incusapi.CertificatePut{
						Name:        "oc-trusted-" + testcert.ClientCertificateFingerprint[:12],
						Description: "Client trusted by Operations Center",
						Type:        "client",
						Projects:    []string{},
						Certificate: testcert.ClientCertificate,
					},
				},
			},
		},
		{
			name:                           "already trusted client certificates are returned separately",
			config:                         nil,
			trustedClientCertificates:      []string{testcert.ClientCertificate},
			knownTrustedClientCertificates: []string{testcert.SecondClientCertificate},

			assertErr: require.NoError,
			wantCertificates: []incusapi.CertificatesPost{
				{
					CertificatePut: incusapi.CertificatePut{
						Name:        "oc-trusted-" + testcert.ClientCertificateFingerprint[:12],
						Description: "Client trusted by Operations Center",
						Type:        "client",
						Projects:    []string{},
						Certificate: testcert.ClientCertificate,
					},
				},
			},
			wantKnownCertificates: []securitytls.TrustedClientCertificate{
				{
					CertificatesPost: incusapi.CertificatesPost{
						CertificatePut: incusapi.CertificatePut{
							Name:        "oc-trusted-" + testcert.SecondClientCertificateFingerprint[:12],
							Description: "Client trusted by Operations Center",
							Type:        "client",
							Projects:    []string{},
							Certificate: testcert.SecondClientCertificate,
						},
					},
					Fingerprint: testcert.SecondClientCertificateFingerprint,
				},
			},
		},
		{
			name: "user provided certificates take precedence",
			config: map[string]any{
				"certificates": []any{
					map[string]any{
						"name":        "cert1",
						"type":        "metrics",
						"certificate": testcert.ClientCertificate,
					},
				},
			},
			trustedClientCertificates:      []string{testcert.ClientCertificate},
			knownTrustedClientCertificates: []string{testcert.SecondClientCertificate},

			assertErr: require.NoError,
			wantCertificates: []incusapi.CertificatesPost{
				{
					CertificatePut: incusapi.CertificatePut{
						Name:        "cert1",
						Type:        "metrics",
						Certificate: testcert.ClientCertificate,
					},
				},
			},
			wantKnownCertificates: nil,
		},
		{
			name:                      "error - invalid trusted client certificate",
			config:                    nil,
			trustedClientCertificates: []string{"not a certificate"},

			assertErr:        require.Error,
			wantCertificates: nil,
		},
		{
			name:                           "error - invalid already trusted client certificate",
			config:                         nil,
			trustedClientCertificates:      []string{testcert.ClientCertificate},
			knownTrustedClientCertificates: []string{"not a certificate"},

			assertErr:        require.Error,
			wantCertificates: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, gotKnown, err := incusPreseedWithDefaults(tc.config, tc.trustedClientCertificates, tc.knownTrustedClientCertificates)

			tc.assertErr(t, err)
			require.Equal(t, tc.wantCertificates, got.Certificates)
			require.Equal(t, tc.wantKnownCertificates, gotKnown)
		})
	}
}
