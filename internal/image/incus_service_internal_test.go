package image

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/shared/api"
)

func Test_filterImageVersionFilesByFilterExpression(t *testing.T) {
	tests := []struct {
		name             string
		in               IncusImages
		filterExpression string

		assertErr require.ErrorAssertionFunc
		want      IncusImages
	}{
		{
			name: "success - no filter expression",
			in: IncusImages{
				IncusImage{
					Versions: api.IncusImageVersions{
						"1": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"file": {},
							},
						},
					},
				},
			},
			filterExpression: "", // empty filter expression

			assertErr: require.NoError,
			want:      IncusImages{},
		},
		{
			name:             "success - empty",
			filterExpression: "true",

			assertErr: require.NoError,
		},
		{
			name: "success - without filtering",
			in: IncusImages{
				IncusImage{
					Versions: api.IncusImageVersions{
						"1": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"file": {},
							},
						},
					},
				},
			},
			filterExpression: "true",

			assertErr: require.NoError,
			want: IncusImages{
				IncusImage{
					Versions: api.IncusImageVersions{
						"1": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"file": {},
							},
						},
					},
				},
			},
		},
		{
			name: "success - with filtering for architecture, version and file_type",
			in: IncusImages{
				IncusImage{
					Architecture: "amd64",
					Versions: api.IncusImageVersions{
						"1": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"root.squashfs": {
									FileType: "squashfs",
								},
								// filtered, since file_type != "squashfs"
								"disk.qcow2": {
									FileType: "disk-kvm.img",
								},
							},
						},
						// filtered, since version != "1"
						"2": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"root.squashfs": {
									FileType: "squashfs",
								},
							},
						},
					},
				},
				// filtered, since architecture != "amd64"
				IncusImage{
					Architecture: "aarch64",
					Versions: api.IncusImageVersions{
						"1": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"root.squashfs": {
									FileType: "squashfs",
								},
							},
						},
					},
				},
			},
			filterExpression: `architecture == "amd64" and version == "1" and file_type == "squashfs"`,

			assertErr: require.NoError,
			want: IncusImages{
				IncusImage{
					Architecture: "amd64",
					Versions: api.IncusImageVersions{
						"1": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"root.squashfs": {
									FileType: "squashfs",
								},
							},
						},
					},
				},
			},
		},

		{
			name:             "error - invalid filter expression",
			filterExpression: `"string"`, // invalid, does not return boolean result

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "expected bool, but got string")
			},
		},
		{
			name: "error - filter expression run",
			in: IncusImages{
				IncusImage{
					Versions: api.IncusImageVersions{
						"1": api.IncusImageVersion{
							Items: map[string]api.IncusImageVersionItem{
								"file": {},
							},
						},
					},
				},
			},
			filterExpression: `fromBase64("~invalid") == ""`, // invalid, returns runtime error during evauluation of the expression.

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "illegal base64 data")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := filterImageVersionFilesByFilterExpression(tc.in, tc.filterExpression)
			tc.assertErr(t, err)

			require.Equal(t, tc.want, got)
		})
	}
}
