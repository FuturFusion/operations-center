package boom_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
)

func TestErr(t *testing.T) {
	tests := []struct {
		name string

		err error

		wantFailNowCalled bool
	}{
		{
			name: "is boom error",

			err: boom.Error,

			wantFailNowCalled: false,
		},
		{
			name: "is not boom error",

			err: errors.New("not a boom err"),

			wantFailNowCalled: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tt := &mockTestingT{}

			boom.ErrorIs(tt, tc.err)

			require.Equal(t, tc.wantFailNowCalled, tt.failNowCalled)
		})
	}
}

type mockTestingT struct {
	failNowCalled bool
}

var _ require.TestingT = &mockTestingT{}

func (mockTestingT) Errorf(format string, args ...any) {}
func (m *mockTestingT) FailNow() {
	m.failNowCalled = true
}
