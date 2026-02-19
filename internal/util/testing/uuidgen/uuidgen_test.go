package uuidgen_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/testing/uuidgen"
)

func TestFromPattern(t *testing.T) {
	tests := []struct {
		name    string
		pattern string

		want       uuid.UUID
		wantFailed bool
	}{
		{
			name:    "success - 1",
			pattern: "1",

			want: uuid.MustParse(`11111111-1111-1111-1111-111111111111`),
		},
		{
			name:    "success - 23",
			pattern: "23",

			want: uuid.MustParse(`23232323-2323-2323-2323-232323232323`),
		},
		{
			name:    "success - beef",
			pattern: "beef",

			want: uuid.MustParse(`beefbeef-beef-beef-beef-beefbeefbeef`),
		},
		{
			name:    "success - fee1900d",
			pattern: "fee1900d",

			want: uuid.MustParse(`fee1900d-fee1-900d-fee1-900dfee1900d`),
		},
		{
			name:    "error - empty pattern",
			pattern: "",

			want:       uuid.UUID{},
			wantFailed: true,
		},
		{
			name:    "error - pattern length not divisor of 32",
			pattern: "123",

			want:       uuid.UUID{},
			wantFailed: true,
		},
		{
			name:    "error - pattern length > 32",
			pattern: strings.Repeat("1", 33),

			want:       uuid.UUID{},
			wantFailed: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dummyT := &dummyTestingT{}

			var id uuid.UUID
			var failed bool

			func() {
				defer func() {
					err := recover()
					if err != nil {
						failed = true
					}
				}()

				id = uuidgen.FromPattern(dummyT, tc.pattern)
			}()

			require.Equal(t, tc.want, id)
			require.Equal(t, tc.wantFailed, failed)
		})
	}
}

type dummyTestingT struct{}

func (d *dummyTestingT) Helper()                           {}
func (d *dummyTestingT) Errorf(format string, args ...any) {}
func (d *dummyTestingT) FailNow() {
	panic("fail now")
}
