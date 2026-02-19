package flaky_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
	"github.com/FuturFusion/operations-center/internal/util/testing/flaky"
)

func TestSkipOnFail(t *testing.T) {
	tests := []struct {
		name            string
		flakySkipOnFail string
		justification   string

		wantCalledFatal   bool
		wantCalledSkipNow bool
		wantCalledSkipf   bool
	}{
		{
			name:            "success - enabled",
			flakySkipOnFail: "true",
			justification:   "boom always fails",

			wantCalledSkipf:   true,
			wantCalledSkipNow: true,
		},
		{
			name:            "success - disabled",
			flakySkipOnFail: "false",
			justification:   "boom always fails",
		},
		{
			name:            "error - justification too short",
			flakySkipOnFail: "1",

			wantCalledFatal: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := os.Setenv("FLAKY_SKIP_ON_FAIL", tc.flakySkipOnFail)
			require.NoError(t, err)
			t.Cleanup(func() {
				err := os.Unsetenv("FLAKY_SKIP_ON_FAIL")
				require.NoError(t, err)
			})

			mockt := &mockT{}

			err = boom.Error
			require.NoError(flaky.SkipOnFail(mockt, tc.justification), err)

			require.Equal(t, tc.wantCalledFatal, mockt.calledFatal)
			require.Equal(t, tc.wantCalledSkipf, mockt.calledSkipf)
			require.Equal(t, tc.wantCalledSkipNow, mockt.calledSkipNow)
		})
	}
}

type mockT struct {
	calledFatal   bool
	calledSkipNow bool
	calledSkipf   bool
}

func (m mockT) Helper() {}

func (m *mockT) Fatal(args ...any) {
	m.calledFatal = true
}

func (m *mockT) SkipNow() {
	m.calledSkipNow = true
}

func (m *mockT) Skipf(format string, args ...any) {
	m.calledSkipf = true
}

func (m mockT) Errorf(format string, args ...any) {
}

func (m mockT) FailNow() {
}
