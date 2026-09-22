package warning

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/shared/api"
)

type Warning struct {
	ID              int64             `json:"-"`
	UUID            uuid.UUID         `json:"uuid"         db:"primary=yes"`
	Type            api.WarningType   `json:"type"`
	Scope           string            `json:"scope"`
	EntityType      string            `json:"entity_type"`
	Entity          string            `json:"entity"`
	Status          api.WarningStatus `json:"status"`
	FirstOccurrence time.Time         `json:"first_occurrence"`
	LastOccurrence  time.Time         `json:"last_occurrence"`
	LastUpdated     time.Time         `json:"last_updated" db:"update_timestamp"`
	Messages        []string          `json:"messages"     db:"marshal=json"`
	Count           int

	// Cause is the error, which caused the warning. It is reported in the log
	// by WarningService.Emit, it is neither stored with the warning nor
	// reported to the user.
	Cause error `json:"-" db:"ignore"`
}

func NewWarning(warningType api.WarningType, scope api.WarningScope, message string) Warning {
	return Warning{
		UUID:       uuid.New(),
		Type:       warningType,
		Scope:      scope.Scope,
		EntityType: scope.EntityType,
		Entity:     scope.Entity,
		Status:     api.WarningStatusNew,
		Messages:   []string{message},
		Count:      1,
	}
}

// NewWarningFromError returns a warning, which reports the given error. The
// message of the warning is the message of the error for the user, prefixed
// with the given context, if any. The technical details of the error are only
// reported in the log, see WarningService.Emit.
func NewWarningFromError(warningType api.WarningType, scope api.WarningScope, cause error, format string, a ...any) Warning {
	message := domain.UserMessage(cause)
	if format != "" {
		message = fmt.Sprintf(format, a...) + ": " + message
	}

	warning := NewWarning(warningType, scope, message)
	warning.Cause = cause

	return warning
}

// Match checks whether the given warning is within the given scope.
func (w Warning) Match(scope api.WarningScope) bool {
	entityTypeMatches := scope.EntityType == "" || scope.EntityType == w.EntityType
	entityMatches := scope.Entity == "" || scope.Entity == w.Entity

	// If the scope is not limited to an entity type or specific entity, just strictly match the scope.
	scopeMatches := scope.Scope == w.Scope

	return entityMatches && entityTypeMatches && scopeMatches
}

func (w Warning) Validate() error {
	if w.UUID == uuid.Nil {
		return domain.NewValidationErrf("Warning has invalid UUID: %q", w.UUID)
	}

	if w.Type == "" {
		return domain.NewValidationErrf("Warning %q cannot have empty type", w.UUID)
	}

	if w.Scope == "" {
		return domain.NewValidationErrf("Warning %q cannot have empty scope", w.UUID)
	}

	if w.EntityType == "" {
		return domain.NewValidationErrf("Warning %q cannot have empty entity type", w.UUID)
	}

	if w.Entity == "" {
		return domain.NewValidationErrf("Warning %q cannot have empty entity", w.UUID)
	}

	if w.Status == "" {
		return domain.NewValidationErrf("Warning %q cannot have empty status", w.UUID)
	}

	if len(w.Messages) == 0 {
		return domain.NewValidationErrf("Warning %q cannot have empty message", w.UUID)
	}

	if w.Count == 0 {
		return domain.NewValidationErrf("Warning %q count is 0", w.UUID)
	}

	return nil
}

type Warnings []Warning

type WarningFilter struct {
	ID         *int64
	UUID       *uuid.UUID
	Scope      *string
	EntityType *string
	Entity     *string
	Type       *api.WarningType
}
