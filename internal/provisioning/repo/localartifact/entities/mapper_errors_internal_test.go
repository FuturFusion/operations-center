package entities

import (
	"fmt"
	"testing"

	"github.com/cowsql/go-cowsql/driver"
	"github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
)

func TestClusterMapErr(t *testing.T) {
	cowsqlBusy := driver.Error{Code: int(sqlite3.ErrBusy), Message: "database is locked"}

	tests := []struct {
		name string
		err  error

		wantErrIs  error
		wantErrMsg string
	}{
		{
			name: "not found",
			err:  ErrNotFound,

			wantErrIs:  domain.ErrNotFound,
			wantErrMsg: "test entity not found",
		},
		{
			name: "conflict",
			err:  ErrConflict,

			wantErrIs:  domain.ErrConstraintViolation,
			wantErrMsg: "test entity already exists",
		},
		{
			name: "sqlite unique constraint",
			err:  fmt.Errorf("Create test_entity: %w", sqlite3.Error{Code: sqlite3.ErrConstraint, ExtendedCode: sqlite3.ErrConstraintUnique}),

			wantErrIs:  domain.ErrConstraintViolation,
			wantErrMsg: "test entity already exists",
		},
		{
			name: "cowsql unique constraint",
			err:  fmt.Errorf("Create test_entity: %w", driver.Error{Code: int(sqlite3.ErrConstraintUnique), Message: "UNIQUE constraint failed"}),

			wantErrIs:  domain.ErrConstraintViolation,
			wantErrMsg: "test entity already exists",
		},
		{
			name: "cowsql primary key constraint",
			err:  fmt.Errorf("Create test_entity: %w", driver.Error{Code: int(sqlite3.ErrConstraintPrimaryKey), Message: "UNIQUE constraint failed"}),

			wantErrIs:  domain.ErrConstraintViolation,
			wantErrMsg: "test entity already exists",
		},
		{
			name: "cowsql foreign key constraint on delete",
			err:  fmt.Errorf("Delete test_entity: %w", driver.Error{Code: int(sqlite3.ErrConstraintForeignKey), Message: "FOREIGN KEY constraint failed"}),

			wantErrIs:  domain.ErrConstraintViolation,
			wantErrMsg: "test entity is still referenced by another entity",
		},
		{
			name: "cowsql foreign key constraint on create",
			err:  fmt.Errorf("Create test_entity: %w", driver.Error{Code: int(sqlite3.ErrConstraintForeignKey), Message: "FOREIGN KEY constraint failed"}),

			wantErrIs:  domain.ErrConstraintViolation,
			wantErrMsg: "test entity references an entity, which does not exist",
		},
		{
			name: "cowsql other constraint",
			err:  fmt.Errorf("Create test_entity: %w", driver.Error{Code: int(sqlite3.ErrConstraintCheck), Message: "CHECK constraint failed"}),

			wantErrIs:  domain.ErrConstraintViolation,
			wantErrMsg: "test entity violates a constraint of the database",
		},
		{
			name: "sqlite primary key constraint",
			err:  fmt.Errorf("Create test_entity: %w", sqlite3.Error{Code: sqlite3.ErrConstraint, ExtendedCode: sqlite3.ErrConstraintPrimaryKey}),

			wantErrIs:  domain.ErrConstraintViolation,
			wantErrMsg: "test entity already exists",
		},
		{
			name: "sqlite foreign key constraint on delete",
			err:  fmt.Errorf("Delete test_entity: %w", sqlite3.Error{Code: sqlite3.ErrConstraint, ExtendedCode: sqlite3.ErrConstraintForeignKey}),

			wantErrIs:  domain.ErrConstraintViolation,
			wantErrMsg: "test entity is still referenced by another entity",
		},
		{
			name: "cowsql busy error",
			err:  fmt.Errorf("Create test_entity: %w", cowsqlBusy),

			wantErrIs:  cowsqlBusy,
			wantErrMsg: "database is locked",
		},
		{
			name: "other error",
			err:  boom.Error,

			wantErrIs:  boom.Error,
			wantErrMsg: boom.Error.Error(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := clusterMapErr(tc.err, "test_entity")

			require.ErrorIs(t, err, tc.wantErrIs, "the mapped error must be of the expected kind")
			require.ErrorContains(t, err, tc.wantErrMsg, "the mapped error must have the expected message")
		})
	}
}
