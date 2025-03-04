package entities

import (
	"time"

	"github.com/google/uuid"
)

// Code generation directives.
//
// DISABLED generate-database:mapper target tokens.mapper.go
// DISABLED generate-database:mapper reset
//
// DISABLED generate-database:mapper stmt -e token objects
// DISABLED generate-database:mapper stmt -e token objects-by-UUID
// DISABLED generate-database:mapper stmt -e token id
// DISABLED generate-database:mapper stmt -e token create
// DISABLED generate-database:mapper stmt -e token update
// DISABLED generate-database:mapper stmt -e token delete-by-UUID
//
// DISABLED generate-database:mapper method -e token ID
// DISABLED generate-database:mapper method -e token Exists
// DISABLED generate-database:mapper method -e token GetOne
// DISABLED generate-database:mapper method -e token GetMany
// DISABLED generate-database:mapper method -e token Create
// DISABLED generate-database:mapper method -e token Update
// DISABLED generate-database:mapper method -e token DeleteOne-by-UUID

type Token struct {
	ID            int
	UUID          UUID `db:"primary=yes"`
	UsesRemaining int
	ExpireAt      time.Time
	Description   string
}

type TokenFilter struct {
	UUID *UUID
}

type UUID uuid.UUID

// MarshalDB implements the Marshaler interface.
func (u UUID) MarshalDB() (string, error) {
	return uuid.UUID(u).String(), nil
}

// UnmarshalDB implements the Unmarshaler interface.
func (u *UUID) UnmarshalDB(text string) error {
	id, err := uuid.Parse(text)
	if err != nil {
		return err
	}

	*u = UUID(id)

	return nil
}
