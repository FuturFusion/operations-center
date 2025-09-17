package entities

import "github.com/google/uuid"

// Code generation directives.
//
//generate-database:mapper target token_seed.mapper.go
//generate-database:mapper reset
//
//generate-database:mapper stmt -e token_seed objects
//generate-database:mapper stmt -e token_seed objects-by-Token
//generate-database:mapper stmt -e token_seed objects-by-Name-and-Token
//generate-database:mapper stmt -e token_seed id
//generate-database:mapper stmt -e token_seed create
//generate-database:mapper stmt -e token_seed update
//generate-database:mapper stmt -e token_seed delete-by-Name-and-Token
//
//generate-database:mapper method -e token_seed ID
//generate-database:mapper method -e token_seed Exists
//generate-database:mapper method -e token_seed GetOne
//generate-database:mapper method -e token_seed GetMany
//generate-database:mapper method -e token_seed Create
//generate-database:mapper method -e token_seed Update
//generate-database:mapper method -e token_seed DeleteOne-by-Name-and-Token

type TokenSeedFilter struct {
	Name  *string
	Token *uuid.UUID
}
