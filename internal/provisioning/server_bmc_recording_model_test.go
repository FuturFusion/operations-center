package provisioning_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestBMCRecordingID(t *testing.T) {
	t.Parallel()

	zurich, err := time.LoadLocation("Europe/Zurich")
	require.NoError(t, err)

	startedAt := time.Date(2026, 10, 27, 8, 15, 0, 0, time.UTC)

	tests := []struct {
		name      string
		startedAt time.Time

		wantID string
	}{
		{
			name:      "UTC",
			startedAt: startedAt,

			wantID: "20261027T081500Z",
		},
		{
			name:      "non UTC",
			startedAt: startedAt.In(zurich),

			wantID: "20261027T081500Z",
		},
		{
			name:      "sub second precision",
			startedAt: startedAt.Add(500 * time.Millisecond),

			wantID: "20261027T081500Z",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			id := provisioning.BMCRecordingID(tc.startedAt)

			require.Equal(t, tc.wantID, id)
			require.NoError(t, provisioning.ValidateBMCRecordingID(id), "a derived ID must be valid")
		})
	}
}

func TestValidateBMCRecordingID(t *testing.T) {
	t.Parallel()

	assertValidationErr := func(tt require.TestingT, err error, a ...any) {
		var verr domain.ErrValidation
		require.ErrorAs(tt, err, &verr, a...)
	}

	tests := []struct {
		name string
		id   string

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "valid",
			id:   "20261027T081500Z",

			assertErr: require.NoError,
		},
		{
			name: "error - empty",
			id:   "",

			assertErr: assertValidationErr,
		},
		{
			name: "error - parent directory",
			id:   "..",

			assertErr: assertValidationErr,
		},
		{
			name: "error - parent directory prefix",
			id:   "../20261027T081500Z",

			assertErr: assertValidationErr,
		},
		{
			name: "error - path separator suffix",
			id:   "20261027T081500Z/x",

			assertErr: assertValidationErr,
		},
		{
			name: "error - too short",
			id:   "20261027T0815Z",

			assertErr: assertValidationErr,
		},
		{
			name: "error - lower case",
			id:   "20261027t081500z",

			assertErr: assertValidationErr,
		},
		{
			name: "error - invalid month",
			id:   "20261327T081500Z",

			assertErr: assertValidationErr,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := provisioning.ValidateBMCRecordingID(tc.id)

			tc.assertErr(t, err)
		})
	}
}

type fakeBMCRecorder struct{}

func (fakeBMCRecorder) Record(context.Context, api.BMCRecordingExchange) {}

func (fakeBMCRecorder) Stats() provisioning.BMCRecordingStats {
	return provisioning.BMCRecordingStats{}
}

func TestBMCRecordingScopeFromContext(t *testing.T) {
	t.Parallel()

	scope := provisioning.BMCRecordingScope{
		Recorder: fakeBMCRecorder{},
		State:    api.ServerDeploymentStateApplyBIOS,
		Retries:  2,
	}

	tests := []struct {
		name string
		ctx  context.Context

		wantScope provisioning.BMCRecordingScope
		wantOK    bool
	}{
		{
			name: "scope found",
			ctx:  provisioning.ContextWithBMCRecordingScope(context.Background(), scope),

			wantScope: scope,
			wantOK:    true,
		},
		{
			name: "plain context",
			ctx:  context.Background(),

			wantScope: provisioning.BMCRecordingScope{},
			wantOK:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			gotScope, gotOK := provisioning.BMCRecordingScopeFromContext(tc.ctx)

			require.Equal(t, tc.wantOK, gotOK, "scope presence")
			require.Equal(t, tc.wantScope, gotScope)
		})
	}
}

func TestBMCRecordingRewrite_Apply(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		rewrite func() *provisioning.BMCRecordingRewrite
		line    string

		want string
	}{
		{
			name: "nil receiver",
			rewrite: func() *provisioning.BMCRecordingRewrite {
				return nil
			},
			line: "serial ABC123",

			want: "serial ABC123",
		},
		{
			name: "no replacements",
			rewrite: func() *provisioning.BMCRecordingRewrite {
				return &provisioning.BMCRecordingRewrite{}
			},
			line: "serial ABC123",

			want: "serial ABC123",
		},
		{
			name: "no match",
			rewrite: func() *provisioning.BMCRecordingRewrite {
				return &provisioning.BMCRecordingRewrite{
					Replacements: []provisioning.BMCRecordingReplacement{
						{Old: "XYZ", New: "SERIAL-1"},
					},
				}
			},
			line: "serial ABC123",

			want: "serial ABC123",
		},
		{
			name: "one replacement",
			rewrite: func() *provisioning.BMCRecordingRewrite {
				return &provisioning.BMCRecordingRewrite{
					Replacements: []provisioning.BMCRecordingReplacement{
						{Old: "ABC123", New: "SERIAL-1"},
					},
				}
			},
			line: "serial ABC123",

			want: "serial SERIAL-1",
		},
		{
			name: "multiple occurrences",
			rewrite: func() *provisioning.BMCRecordingRewrite {
				return &provisioning.BMCRecordingRewrite{
					Replacements: []provisioning.BMCRecordingReplacement{
						{Old: "ABC123", New: "SERIAL-1"},
					},
				}
			},
			line: `{"a":"ABC123","b":"ABC123"}`,

			want: `{"a":"SERIAL-1","b":"SERIAL-1"}`,
		},
		{
			name: "overlapping values, longer wins",
			rewrite: func() *provisioning.BMCRecordingRewrite {
				return &provisioning.BMCRecordingRewrite{
					Replacements: []provisioning.BMCRecordingReplacement{
						{Old: "bmc", New: "host-1"},
						{Old: "bmc.example.com", New: "bmc.invalid"},
					},
				}
			},
			line: "https://bmc.example.com/redfish/v1 bmc",

			want: "https://bmc.invalid/redfish/v1 host-1",
		},
		{
			name: "case insensitive",
			rewrite: func() *provisioning.BMCRecordingRewrite {
				return &provisioning.BMCRecordingRewrite{
					Replacements: []provisioning.BMCRecordingReplacement{
						{Old: "AA:BB:CC:DD:EE:FF", New: "00:00:5E:00:53:01", CaseInsensitive: true},
					},
				}
			},
			line: "mac aa:bb:cc:dd:ee:ff AA:BB:CC:DD:EE:FF",

			want: "mac 00:00:5E:00:53:01 00:00:5E:00:53:01",
		},
		{
			name: "case sensitive",
			rewrite: func() *provisioning.BMCRecordingRewrite {
				return &provisioning.BMCRecordingRewrite{
					Replacements: []provisioning.BMCRecordingReplacement{
						{Old: "ABC123", New: "SERIAL-1"},
					},
				}
			},
			line: "serial abc123",

			want: "serial abc123",
		},
		{
			name: "special characters are literal",
			rewrite: func() *provisioning.BMCRecordingRewrite {
				return &provisioning.BMCRecordingRewrite{
					Replacements: []provisioning.BMCRecordingReplacement{
						{Old: "10.0.0.1", New: "192.0.2.1"},
					},
				}
			},
			line: "10.0.0.1 10x0x0x1",

			want: "192.0.2.1 10x0x0x1",
		},
		{
			name: "empty value ignored",
			rewrite: func() *provisioning.BMCRecordingRewrite {
				return &provisioning.BMCRecordingRewrite{
					Replacements: []provisioning.BMCRecordingReplacement{
						{Old: "", New: "EMPTY"},
						{Old: "ABC123", New: "SERIAL-1"},
					},
				}
			},
			line: "serial ABC123",

			want: "serial SERIAL-1",
		},
		{
			name: "only empty values",
			rewrite: func() *provisioning.BMCRecordingRewrite {
				return &provisioning.BMCRecordingRewrite{
					Replacements: []provisioning.BMCRecordingReplacement{
						{Old: "", New: "EMPTY"},
					},
				}
			},
			line: "serial ABC123",

			want: "serial ABC123",
		},
		{
			name: "replaced text is not rewritten again",
			rewrite: func() *provisioning.BMCRecordingRewrite {
				return &provisioning.BMCRecordingRewrite{
					Replacements: []provisioning.BMCRecordingReplacement{
						{Old: "a", New: "b"},
						{Old: "b", New: "c"},
					},
				}
			},
			line: "ab",

			want: "bc",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rewrite := tc.rewrite()

			got := rewrite.Apply(tc.line)
			gotAgain := rewrite.Apply(tc.line)

			require.Equal(t, tc.want, got)
			require.Equal(t, tc.want, gotAgain, "a second call must use the same compiled rewrite")
		})
	}
}
