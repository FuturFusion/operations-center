package warning_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
	"github.com/FuturFusion/operations-center/internal/util/testing/log"
	"github.com/FuturFusion/operations-center/internal/util/testing/queue"
	"github.com/FuturFusion/operations-center/internal/util/testing/uuidgen"
	"github.com/FuturFusion/operations-center/internal/warning"
	"github.com/FuturFusion/operations-center/internal/warning/repo/mock"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestWarningService_DeleteByUUID(t *testing.T) {
	tests := []struct {
		name                string
		repoDeleteByUUIDErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",

			assertErr: require.NoError,
		},
		{
			name:                "error - repo.DeleteByUUID",
			repoDeleteByUUIDErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.WarningRepoMock{
				DeleteByUUIDFunc: func(ctx context.Context, id uuid.UUID) error {
					return tc.repoDeleteByUUIDErr
				},
			}

			warningSvc := warning.NewWarningService(repo)

			// Run test.
			err := warningSvc.DeleteByUUID(t.Context(), uuidgen.FromPattern(t, "1"))

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestWarningService_Emit(t *testing.T) {
	fixedNow := time.Date(2026, 4, 20, 14, 29, 22, 0, time.UTC)
	fixedPast := time.Date(2026, 4, 19, 13, 28, 21, 0, time.UTC)

	testWarning := func(dbID int64, wType api.WarningType, srcName string, count int, messages ...string) warning.Warning {
		w := newTestWarning(t, fmt.Sprintf("%d", dbID), wType, srcName, "")
		w.Messages = messages
		now := fixedPast
		w.ID = dbID
		w.LastOccurrence = now
		w.FirstOccurrence = now
		w.LastUpdated = now
		w.Count = count

		return w
	}

	tests := []struct {
		name    string
		warning warning.Warning

		repoGetByScopeWarnings warning.Warnings
		repoGetByScopeErr      error
		repoUpsertErr          error

		assertLog   log.MatcherFunc
		wantWarning warning.Warning
	}{
		{
			name:    "success - new warning",
			warning: newTestWarning(t, "1", api.WarningTypeUnreachable, "src1", "message1"),

			assertLog: log.Contains(`message1 uuid=11111111-1111-1111-1111-111111111111 type="Server unreachable" scope=test entity_type=test entity=src1`),
			wantWarning: warning.Warning{
				UUID:            uuidgen.FromPattern(t, "1"),
				Type:            api.WarningTypeUnreachable,
				Scope:           "test",
				EntityType:      "test",
				Entity:          "src1",
				Status:          api.WarningStatusNew,
				FirstOccurrence: fixedNow,
				LastOccurrence:  fixedNow,
				LastUpdated:     fixedNow,
				Messages:        []string{"message1"},
				Count:           1,
			},
		},
		{
			name:    "success - merged",
			warning: newTestWarning(t, "1", api.WarningTypeUnreachable, "src1", "message2"),

			repoGetByScopeWarnings: warning.Warnings{
				testWarning(99, api.WarningTypeUnreachable, "src1", 10, "message0", "message1"),
			},

			assertLog: log.Contains(`message2 uuid=11111111-1111-1111-1111-111111111111 type="Server unreachable" scope=test entity_type=test entity=src1`),
			wantWarning: warning.Warning{
				UUID:            uuidgen.FromPattern(t, "99"),
				Type:            api.WarningTypeUnreachable,
				Scope:           "test",
				EntityType:      "test",
				Entity:          "src1",
				Status:          api.WarningStatusNew,
				FirstOccurrence: fixedPast,
				LastOccurrence:  fixedNow,
				LastUpdated:     fixedNow,
				Messages:        []string{"message0", "message1", "message2"},
				Count:           11,
			},
		},
		{
			name:    "success - merged, reordered message",
			warning: newTestWarning(t, "1", api.WarningTypeUnreachable, "src1", "message0"),
			repoGetByScopeWarnings: warning.Warnings{
				testWarning(99, api.WarningTypeUnreachable, "src1", 10, "message0", "message1"),
			},

			assertLog: log.Contains(`message0 uuid=11111111-1111-1111-1111-111111111111 type="Server unreachable" scope=test entity_type=test entity=src1`),
			wantWarning: warning.Warning{
				UUID:            uuidgen.FromPattern(t, "99"),
				Type:            api.WarningTypeUnreachable,
				Scope:           "test",
				EntityType:      "test",
				Entity:          "src1",
				Status:          api.WarningStatusNew,
				FirstOccurrence: fixedPast,
				LastOccurrence:  fixedNow,
				LastUpdated:     fixedNow,
				Messages:        []string{"message1", "message0"},
				Count:           11,
			},
		},
		{
			name:    "error - validation",
			warning: newTestWarning(t, "1", api.WarningTypeUnreachable, "", "message0"), // no source.

			assertLog: log.Contains(`Failed to record warning uuid=11111111-1111-1111-1111-111111111111 err="Warning \"11111111-1111-1111-1111-111111111111\" cannot have empty entity"`),
		},
		{
			name:    "error - invalid warning state",
			warning: newTestWarning(t, "1", api.WarningTypeUnreachable, "src1", "message0"),

			repoGetByScopeWarnings: warning.Warnings{
				testWarning(99, api.WarningTypeUnreachable, "src1", 10, "message0", "message1"),
				testWarning(98, api.WarningTypeUnreachable, "src2", 10, "message0", "message1"),
			},

			assertLog: log.Contains(`Failed to record warning uuid=11111111-1111-1111-1111-111111111111 err="Invalid warning state for scope {test test src1}"`),
		},
		{
			name:    "error - repoGetByScope",
			warning: newTestWarning(t, "1", api.WarningTypeUnreachable, "src1", "message0"),

			repoGetByScopeErr: boom.Error,

			assertLog: log.Contains(`Failed to record warning uuid=11111111-1111-1111-1111-111111111111 err=boom!`),
		},
		{
			name:    "error - repoUpsert",
			warning: newTestWarning(t, "1", api.WarningTypeUnreachable, "src1", "message0"),

			repoUpsertErr: boom.Error,

			assertLog: log.Contains(`Failed to record warning uuid=11111111-1111-1111-1111-111111111111 err=boom!`),
			wantWarning: warning.Warning{
				UUID:            uuidgen.FromPattern(t, "1"),
				Type:            api.WarningTypeUnreachable,
				Scope:           "test",
				EntityType:      "test",
				Entity:          "src1",
				Status:          api.WarningStatusNew,
				FirstOccurrence: fixedNow,
				LastOccurrence:  fixedNow,
				LastUpdated:     fixedNow,
				Messages:        []string{"message0"},
				Count:           1,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			logBuf := &bytes.Buffer{}
			err := logger.InitLogger(logBuf, "", false, true, true)
			require.NoError(t, err)

			repo := &mock.WarningRepoMock{
				GetByScopeAndTypeFunc: func(ctx context.Context, scope api.WarningScope, wType api.WarningType) (warning.Warnings, error) {
					return tc.repoGetByScopeWarnings, tc.repoGetByScopeErr
				},
				UpsertFunc: func(ctx context.Context, w warning.Warning) (int64, error) {
					require.Equal(t, tc.wantWarning, w)

					return 0, tc.repoUpsertErr
				},
			}

			// Run test
			warningSvc := warning.NewWarningService(repo, warning.WithWarningServiceNow(func() time.Time { return fixedNow }))

			// Assert
			warningSvc.Emit(t.Context(), tc.warning)
			tc.assertLog(t, logBuf)
		})
	}
}

func TestWarningService_GetAll(t *testing.T) {
	tests := []struct {
		name          string
		repoGetAllErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",

			assertErr: require.NoError,
		},
		{
			name:          "error - repo.GetAll",
			repoGetAllErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.WarningRepoMock{
				GetAllFunc: func(ctx context.Context) (warning.Warnings, error) {
					return nil, tc.repoGetAllErr
				},
			}

			warningSvc := warning.NewWarningService(repo)

			// Run test.
			_, err := warningSvc.GetAll(t.Context())

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestWarningService_GetByScopeAndType(t *testing.T) {
	tests := []struct {
		name                     string
		repoGetByScopeAndTypeErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",

			assertErr: require.NoError,
		},
		{
			name:                     "error - repo.GetByScopeAndType",
			repoGetByScopeAndTypeErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.WarningRepoMock{
				GetByScopeAndTypeFunc: func(ctx context.Context, scope api.WarningScope, wType api.WarningType) (warning.Warnings, error) {
					return nil, tc.repoGetByScopeAndTypeErr
				},
			}

			warningSvc := warning.NewWarningService(repo)

			// Run test.
			_, err := warningSvc.GetByScopeAndType(t.Context(), api.WarningScope{}, api.WarningTypeUnreachable)

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestWarningService_GetByUUID(t *testing.T) {
	tests := []struct {
		name             string
		repoGetByUUIDErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",

			assertErr: require.NoError,
		},
		{
			name:             "error - repo.GetByUUID",
			repoGetByUUIDErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := &mock.WarningRepoMock{
				GetByUUIDFunc: func(ctx context.Context, id uuid.UUID) (*warning.Warning, error) {
					return nil, tc.repoGetByUUIDErr
				},
			}

			warningSvc := warning.NewWarningService(repo)

			// Run test.
			_, err := warningSvc.GetByUUID(t.Context(), uuidgen.FromPattern(t, "1"))

			// Assert
			tc.assertErr(t, err)
		})
	}
}

func TestWarningService_RemoveStale(t *testing.T) {
	testWarning := func(uuidPattern string, srcName string, messages ...string) warning.Warning {
		warn := newTestWarning(t, uuidPattern, api.WarningTypeUnreachable, srcName, "")
		warn.Messages = messages
		return warn
	}

	testScope := func(srcName string) api.WarningScope {
		scope := api.WarningScope{
			Scope:      "test",
			EntityType: "test",
		}
		scope.Entity = srcName
		return scope
	}

	q := func(messages ...string) queue.Item[[]string] {
		return queue.Item[[]string]{Value: messages}
	}

	type list []queue.Item[[]string]
	tests := []struct {
		name     string
		warnings warning.Warnings
		scope    api.WarningScope

		repoGetAllWarnings warning.Warnings

		repoUpdate       []queue.Item[[]string]
		repoDeleteByUUID []queue.Item[[]string]

		repoGetAllErr error
		assertLog     log.MatcherFunc
	}{
		{
			name:               "success - no change in messages",
			scope:              testScope(""),
			warnings:           warning.Warnings{testWarning("1", "a", "a"), testWarning("2", "b", "b"), testWarning("3", "c", "c")},
			repoGetAllWarnings: warning.Warnings{testWarning("1", "a", "a"), testWarning("2", "b", "b"), testWarning("3", "c", "c")},
			assertLog:          log.Empty,
		},
		{
			name:               "success - no new messages, clear all matching broad scope",
			scope:              testScope(""),
			repoGetAllWarnings: warning.Warnings{testWarning("1", "a", "a"), testWarning("2", "b", "b"), testWarning("3", "c", "c")},
			repoDeleteByUUID:   list{q("a"), q("b"), q("c")},
			assertLog:          log.Empty,
		},
		{
			name:               "success - no new messages, clear all matching narrow scope",
			scope:              testScope("a"),
			repoGetAllWarnings: warning.Warnings{testWarning("1", "a", "a"), testWarning("2", "b", "b"), testWarning("3", "c", "c")},
			repoDeleteByUUID:   list{q("a")},
			assertLog:          log.Empty,
		},
		{
			name:               "success - new messages, remove old messages matching broad scope",
			scope:              testScope(""),
			warnings:           warning.Warnings{testWarning("1", "a", "a"), testWarning("2", "b", "b"), testWarning("3", "c", "c")},
			repoGetAllWarnings: warning.Warnings{testWarning("1", "a", "a"), testWarning("2", "b", "b", "b2"), testWarning("3", "c", "c", "c2")},
			repoUpdate:         list{q("b"), q("c")},
			assertLog:          log.Empty,
		},
		{
			name:               "success - new messages, remove old messages matching broad scope, and clear empty warnings",
			scope:              testScope(""),
			warnings:           warning.Warnings{testWarning("1", "a", "a2"), testWarning("2", "b", "b"), testWarning("3", "c", "c")},
			repoGetAllWarnings: warning.Warnings{testWarning("1", "a", "a"), testWarning("2", "b", "b", "b2"), testWarning("3", "c", "c", "c2")},
			repoDeleteByUUID:   list{q("a")},
			repoUpdate:         list{q("b"), q("c")},
			assertLog:          log.Empty,
		},
		{
			name:               "success - new messages, with duplicate scope",
			scope:              testScope(""),
			warnings:           warning.Warnings{testWarning("1", "a", "a"), testWarning("2", "a", "a2"), testWarning("3", "b", "b"), testWarning("4", "c", "c")},
			repoGetAllWarnings: warning.Warnings{testWarning("1", "a", "a", "a2"), testWarning("2", "b", "b", "b2"), testWarning("4", "c", "c", "c2")},
			repoUpdate:         list{q("b"), q("c")},
			assertLog:          log.Empty,
		},
		{
			name:               "success - new messages, remove old messages matching broad scope, and clear empty warnings",
			scope:              testScope("b"),
			warnings:           warning.Warnings{testWarning("1", "a", "a2"), testWarning("2", "b", "b"), testWarning("3", "c", "c")},
			repoGetAllWarnings: warning.Warnings{testWarning("1", "a", "a"), testWarning("2", "b", "b", "b2"), testWarning("3", "c", "c", "c2")},
			repoUpdate:         list{q("b")},
			assertLog:          log.Empty,
		},
		{
			name:               "success - new messages, duplicate cross-scope messages, scope is broad",
			scope:              testScope(""),
			warnings:           warning.Warnings{testWarning("1", "a", "a"), testWarning("2", "b", "b")},
			repoGetAllWarnings: warning.Warnings{testWarning("1", "a", "a", "a2"), testWarning("2", "b", "a", "a2"), testWarning("3", "c", "a", "a2")},
			repoUpdate:         list{q("a"), q("a"), q("a")},
			assertLog:          log.Empty,
		},
		{
			name:               "success - new messages, duplicate cross-scope messages, scope is narrow",
			scope:              testScope("a"),
			warnings:           warning.Warnings{testWarning("1", "a", "a"), testWarning("2", "b", "b")},
			repoGetAllWarnings: warning.Warnings{testWarning("1", "a", "a", "a2"), testWarning("2", "b", "a", "a2"), testWarning("3", "c", "a", "a2")},
			repoUpdate:         list{q("a")},
			assertLog:          log.Empty,
		},
		{
			name:               "success - new messages, unmatched duplicate cross-scope messages, scope is narrow",
			scope:              testScope("b"),
			warnings:           warning.Warnings{testWarning("1", "a", "a"), testWarning("2", "b", "b")},
			repoGetAllWarnings: warning.Warnings{testWarning("1", "a", "a", "a2"), testWarning("2", "b", "a", "a2"), testWarning("3", "c", "a", "a2")},
			repoDeleteByUUID:   list{q("a", "a2")},
			assertLog:          log.Empty,
		},
		{
			name:          "error - repo.GetAll",
			scope:         testScope("b"),
			warnings:      warning.Warnings{testWarning("1", "a", "a"), testWarning("2", "b", "b")},
			repoGetAllErr: boom.Error,
			assertLog:     log.Contains(`Failed to remove stale warnings scope=test entity_type=test entity=b err="Failed to get all warnings: boom!"`),
		},
		{
			name:               "error - repo.DeleteByUUID",
			scope:              testScope("a"),
			repoGetAllWarnings: warning.Warnings{testWarning("1", "a", "a"), testWarning("2", "b", "b")},
			repoDeleteByUUID: list{
				{
					Value: []string{"a"},
					Err:   boom.Error,
				},
			},
			assertLog: log.Contains(`Failed to remove stale warnings scope=test entity_type=test entity=a err="Failed to delete stale warning: boom!"`),
		},
		{
			name:               "error - repo.Update",
			scope:              testScope(""),
			warnings:           warning.Warnings{testWarning("1", "a", "a"), testWarning("2", "a", "a2"), testWarning("3", "b", "b"), testWarning("4", "c", "c")},
			repoGetAllWarnings: warning.Warnings{testWarning("1", "a", "a", "a2"), testWarning("2", "b", "b", "b2"), testWarning("4", "c", "c", "c2")},
			repoUpdate: []queue.Item[[]string]{
				{
					Err: boom.Error,
				},
			},
			assertLog: log.Contains(`Failed to remove stale warnings scope=test entity_type=test entity="" err="Failed to prune stale warning messages: boom!"`),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			logBuf := &bytes.Buffer{}
			err := logger.InitLogger(logBuf, "", false, true, true)
			require.NoError(t, err)

			repo := &mock.WarningRepoMock{
				DeleteByUUIDFunc: func(ctx context.Context, id uuid.UUID) error {
					var warningMessages []string
					for _, w := range tc.repoGetAllWarnings {
						if w.UUID == id {
							warningMessages = w.Messages
							break
						}
					}

					messages, err := queue.Pop(t, &tc.repoDeleteByUUID)
					if err == nil {
						require.Equal(t, messages, warningMessages)
					}

					return err
				},
				UpdateFunc: func(ctx context.Context, id uuid.UUID, w warning.Warning) error {
					messages, err := queue.Pop(t, &tc.repoUpdate)
					if err == nil {
						require.Equal(t, messages, w.Messages)
					}

					return err
				},
				GetAllFunc: func(ctx context.Context) (warning.Warnings, error) {
					return tc.repoGetAllWarnings, tc.repoGetAllErr
				},
			}

			warningSvc := warning.NewWarningService(repo)

			// Run tst
			ctx := context.Background()
			warningSvc.RemoveStale(ctx, tc.scope, tc.warnings)

			// Assert
			tc.assertLog(t, logBuf)
			require.Empty(t, tc.repoUpdate)
			require.Empty(t, tc.repoDeleteByUUID)
		})
	}
}

func TestWarningService_UpdateStatusByUUID(t *testing.T) {
	testWarning := func() *warning.Warning {
		now := time.Now().UTC()
		w := newTestWarning(t, "1", api.WarningTypeUnreachable, "src", "msg")
		w.LastOccurrence = now
		w.FirstOccurrence = now
		w.LastUpdated = now
		return &w
	}

	tests := []struct {
		name   string
		status api.WarningStatus

		repoGetByUUIDWarning *warning.Warning

		repoGetByUUIDErr error
		repoUpdateErr    error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:                 "success",
			status:               api.WarningStatusAcknowledged,
			repoGetByUUIDWarning: testWarning(),

			assertErr: require.NoError,
		},
		{
			name:             "error - repoGetByUUID",
			status:           api.WarningStatusAcknowledged,
			repoGetByUUIDErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:                 "error - repoUpdate",
			status:               api.WarningStatusAcknowledged,
			repoGetByUUIDWarning: testWarning(),
			repoUpdateErr:        boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mock.WarningRepoMock{
				GetByUUIDFunc: func(ctx context.Context, id uuid.UUID) (*warning.Warning, error) {
					return tc.repoGetByUUIDWarning, tc.repoGetByUUIDErr
				},
				UpdateFunc: func(ctx context.Context, id uuid.UUID, w warning.Warning) error {
					return tc.repoUpdateErr
				},
			}

			warningSvc := warning.NewWarningService(repo)

			// Perform test.
			ctx := context.Background()
			now := time.Now().UTC()
			w, err := warningSvc.UpdateStatusByUUID(ctx, uuid.Nil, tc.status)
			tc.assertErr(t, err)
			if err == nil {
				require.Equal(t, tc.repoGetByUUIDWarning.LastUpdated, w.LastOccurrence)
				require.Equal(t, tc.status, w.Status)
				require.True(t, w.LastUpdated.After(now))
			}
		})
	}
}

func newTestWarning(t *testing.T, uuidPattern string, wType api.WarningType, sourceName string, message string) warning.Warning {
	t.Helper()

	return warning.Warning{
		UUID:       uuidgen.FromPattern(t, uuidPattern),
		Type:       wType,
		Scope:      "test",
		EntityType: "test",
		Entity:     sourceName,
		Status:     api.WarningStatusNew,
		Messages:   []string{message},
		Count:      1,
	}
}
