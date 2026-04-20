package warning_test

import (
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
