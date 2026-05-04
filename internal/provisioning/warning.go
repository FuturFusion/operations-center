package provisioning

import (
	"context"
	"log/slog"
	"strings"

	"github.com/FuturFusion/operations-center/internal/warning"
	"github.com/FuturFusion/operations-center/shared/api"
)

type WarningServicePort interface {
	Emit(ctx context.Context, w warning.Warning)
	RemoveStale(ctx context.Context, scope api.WarningScope, newWarnings warning.Warnings)
}

type LogWarningService struct{}

var _ WarningServicePort = LogWarningService{}

func (LogWarningService) Emit(ctx context.Context, warn warning.Warning) {
	slog.WarnContext(ctx,
		strings.Join(warn.Messages, "; "), //nolint:sloglint
		slog.String("uuid", warn.UUID.String()),
		slog.String("type", string(warn.Type)),
		slog.String("scope", warn.Scope),
		slog.String("entity_type", warn.EntityType),
		slog.String("entity", warn.Entity),
	)
}

func (LogWarningService) RemoveStale(_ context.Context, _ api.WarningScope, _ warning.Warnings) {
	_ = true // no-op
}

type NoopWarningService struct{}

var _ WarningServicePort = NoopWarningService{}

func (NoopWarningService) Emit(ctx context.Context, warn warning.Warning) {
	_ = true // no-op
}

func (NoopWarningService) RemoveStale(_ context.Context, _ api.WarningScope, _ warning.Warnings) {
	_ = true // no-op
}
