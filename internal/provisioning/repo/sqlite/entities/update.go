package entities

import "github.com/google/uuid"

// Code generation directives.
//
//generate-database:mapper target update.mapper.go
//generate-database:mapper reset
//
//generate-database:mapper stmt -e update objects
//generate-database:mapper stmt -e update objects-by-UUID
//generate-database:mapper stmt -e update names
//generate-database:mapper stmt -e update id
//generate-database:mapper stmt -e update create
//generate-database:mapper stmt -e update delete-by-UUID
//
//generate-database:mapper method -e update ID
//generate-database:mapper method -e update Exists
//generate-database:mapper method -e update GetOne
//generate-database:mapper method -e update GetMany
//generate-database:mapper method -e update GetNames
//generate-database:mapper method -e update Create
//generate-database:mapper method -e update DeleteOne-by-UUID

type UpdateFilter struct {
	UUID *uuid.UUID
}
