package entities

import "github.com/google/uuid"

// Code generation directives.
//
//generate-database:mapper target token.mapper.go
//generate-database:mapper reset
//
//generate-database:mapper stmt -e token objects
//generate-database:mapper stmt -e token objects-by-UUID
//generate-database:mapper stmt -e token names
//generate-database:mapper stmt -e token id
//generate-database:mapper stmt -e token create
//generate-database:mapper stmt -e token update
//generate-database:mapper stmt -e token delete-by-UUID
//
//generate-database:mapper method -e token ID
//generate-database:mapper method -e token Exists
//generate-database:mapper method -e token GetOne
//generate-database:mapper method -e token GetMany
//generate-database:mapper method -e token GetNames
//generate-database:mapper method -e token Create
//generate-database:mapper method -e token Update
//generate-database:mapper method -e token DeleteOne-by-UUID

type TokenFilter struct {
	UUID *uuid.UUID
}
