package dbschema_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/sql/dbschema"
)

func TestMapDBError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "nil",
			err:  nil,

			assertErr: require.NoError,
		},
		{
			name: "unique constraint",
			err:  errors.New("UNIQUE constraint failed: channels.name"),

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrConstraintViolation, a...)
				require.ErrorContains(tt, err, "The entry already exists", a...)
			},
		},
		{
			name: "foreign key constraint",
			err:  errors.New("FOREIGN KEY constraint failed"),

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrConstraintViolation, a...)
				require.ErrorContains(tt, err, "The entry references an entity, which does not exist", a...)
			},
		},
		{
			name: "other error",
			err:  errors.New("other"),

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.EqualError(tt, err, "other", a...)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Run test
			err := dbschema.MapDBError(tc.err)

			// Assert
			tc.assertErr(t, err, "expect the error of the database to be mapped to a domain error")
		})
	}
}

func TestMapForeignKeyError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error

		wantMessage string
	}{
		{
			name: "delete",
			err:  fmt.Errorf(`Delete "channels": %w`, errors.New("FOREIGN KEY constraint failed")),

			wantMessage: "Channel is still referenced by another entity",
		},
		{
			name: "create",
			err:  fmt.Errorf(`Failed to create "tokens" entry: %w`, errors.New("FOREIGN KEY constraint failed")),

			wantMessage: "Channel references an entity, which does not exist",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Run test
			err := dbschema.MapForeignKeyError(tc.err, "Channel")

			// Assert
			require.ErrorIs(t, err, domain.ErrConstraintViolation, "expect a constraint violation")
			require.ErrorIs(t, err, tc.err, "expect the error of the database to be kept as cause")
			require.ErrorContains(t, err, tc.wantMessage, "expect the message to name the side of the reference, which failed")
		})
	}
}
