package warning_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/util/testing/uuidgen"
	"github.com/FuturFusion/operations-center/internal/warning"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestWarning_Validate(t *testing.T) {
	tests := []struct {
		name    string
		warning warning.Warning

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "valid",
			warning: warning.Warning{
				UUID:       uuidgen.FromPattern(t, "1"),
				Type:       api.WarningTypeUnreachable,
				Scope:      "scope",
				EntityType: "entity_type",
				Entity:     "entity",
				Status:     api.WarningStatusNew,
				Messages:   []string{"message0"},
				Count:      1,
			},

			assertErr: require.NoError,
		},
		{
			name: "error - invalid UUID",
			warning: warning.Warning{
				UUID:       uuid.Nil, // nil UUID
				Type:       api.WarningTypeUnreachable,
				Scope:      "scope",
				EntityType: "entity_type",
				Entity:     "entity",
				Status:     api.WarningStatusNew,
				Messages:   []string{"message0"},
				Count:      1,
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(tt, err, "Warning has invalid UUID")
			},
		},
		{
			name: "error - invalid warning type",
			warning: warning.Warning{
				UUID:       uuidgen.FromPattern(t, "1"),
				Type:       api.WarningType(""), // invalid warning type
				Scope:      "scope",
				EntityType: "entity_type",
				Entity:     "entity",
				Status:     api.WarningStatusNew,
				Messages:   []string{"message0"},
				Count:      1,
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(tt, err, "cannot have empty type")
			},
		},
		{
			name: "error - invalid warning scope",
			warning: warning.Warning{
				UUID:       uuidgen.FromPattern(t, "1"),
				Type:       api.WarningTypeUnreachable,
				Scope:      "", // invalid warning scope
				EntityType: "entity_type",
				Entity:     "entity",
				Status:     api.WarningStatusNew,
				Messages:   []string{"message0"},
				Count:      1,
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(tt, err, "cannot have empty scope")
			},
		},
		{
			name: "error - invalid entity type",
			warning: warning.Warning{
				UUID:       uuidgen.FromPattern(t, "1"),
				Type:       api.WarningTypeUnreachable,
				Scope:      "scope",
				EntityType: "", // invalid entity type
				Entity:     "entity",
				Status:     api.WarningStatusNew,
				Messages:   []string{"message0"},
				Count:      1,
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(tt, err, "cannot have empty entity type")
			},
		},
		{
			name: "error - invalid warning entity",
			warning: warning.Warning{
				UUID:       uuidgen.FromPattern(t, "1"),
				Type:       api.WarningTypeUnreachable,
				Scope:      "scope",
				EntityType: "entity_type",
				Entity:     "",
				Status:     api.WarningStatusNew,
				Messages:   []string{"message0"},
				Count:      1,
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(tt, err, "cannot have empty entity")
			},
		},
		{
			name: "error - invalid warning status",
			warning: warning.Warning{
				UUID:       uuidgen.FromPattern(t, "1"),
				Type:       api.WarningTypeUnreachable,
				Scope:      "scope",
				EntityType: "entity_type",
				Entity:     "entity",
				Status:     api.WarningStatus(""), // invalid status
				Messages:   []string{"message0"},
				Count:      1,
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(tt, err, "cannot have empty status")
			},
		},
		{
			name: "error - invalid messages",
			warning: warning.Warning{
				UUID:       uuidgen.FromPattern(t, "1"),
				Type:       api.WarningTypeUnreachable,
				Scope:      "scope",
				EntityType: "entity_type",
				Entity:     "entity",
				Status:     api.WarningStatusNew,
				Messages:   []string{}, // invalid messages
				Count:      1,
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(tt, err, "cannot have empty message")
			},
		},
		{
			name: "error - invalid count",
			warning: warning.Warning{
				UUID:       uuidgen.FromPattern(t, "1"),
				Type:       api.WarningTypeUnreachable,
				Scope:      "scope",
				EntityType: "entity_type",
				Entity:     "entity",
				Status:     api.WarningStatusNew,
				Messages:   []string{"message0"},
				Count:      0, // invalid count
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var verr domain.ErrValidation
				require.ErrorAs(tt, err, &verr, a...)
				require.ErrorContains(tt, err, "count is 0")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.warning.Validate()

			tc.assertErr(t, err)
		})
	}
}

func TestNewWarningFromError(t *testing.T) {
	scope := api.WarningScope{Scope: "test", EntityType: "server", Entity: "one"}

	tests := []struct {
		name   string
		cause  error
		format string
		args   []any

		wantMessage string
	}{
		{
			name:   "domain error",
			cause:  domain.NewErrorf(domain.ErrNotFound, "", "Server %q not found", "one").WithCause(errors.New("no such column: name")),
			format: "Server poll failed",

			wantMessage: `Server poll failed: Server "one" not found`,
		},
		{
			name:  "domain error without context",
			cause: domain.NewErrorf(domain.ErrNotFound, "", "Server %q not found", "one"),

			wantMessage: `Server "one" not found`,
		},
		{
			name:   "wrapped kind",
			cause:  fmt.Errorf("Server %q is clustered: %w", "one", domain.ErrOperationNotPermitted),
			format: "Server rename failed",

			wantMessage: `Server rename failed: Server "one" is clustered`,
		},
		{
			name:   "formatted context",
			cause:  errors.New("boom!"),
			format: "Failed to resync %q",
			args:   []any{"instances"},

			wantMessage: `Failed to resync "instances": boom!`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := warning.NewWarningFromError(api.WarningTypeUnreachable, scope, tc.cause, tc.format, tc.args...)

			require.Equal(t, []string{tc.wantMessage}, w.Messages, "only the message for the user is stored")
			require.Equal(t, tc.cause, w.Cause, "the cause is kept for the log")
			require.Equal(t, api.WarningTypeUnreachable, w.Type)
			require.Equal(t, scope.Entity, w.Entity)
		})
	}
}
