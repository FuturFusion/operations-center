package entities

// Code generation directives.
//
//generate-database:mapper target warning.mapper.go
//generate-database:mapper reset
//
//generate-database:mapper stmt -e warning objects
//generate-database:mapper stmt -e warning objects-by-UUID
//generate-database:mapper stmt -e warning objects-by-Type
//generate-database:mapper stmt -e warning objects-by-Type-and-Scope
//generate-database:mapper stmt -e warning objects-by-Type-and-Scope-and-EntityType
//generate-database:mapper stmt -e warning objects-by-Type-and-Scope-and-EntityType-and-Entity
//generate-database:mapper stmt -e warning id
//generate-database:mapper stmt -e warning create-or-replace
//generate-database:mapper stmt -e warning update
//generate-database:mapper stmt -e warning delete-by-UUID
//
//generate-database:mapper method -e warning ID
//generate-database:mapper method -e warning GetOne
//generate-database:mapper method -e warning GetMany
//generate-database:mapper method -e warning CreateOrReplace
//generate-database:mapper method -e warning Update
//generate-database:mapper method -e warning DeleteOne-by-UUID
