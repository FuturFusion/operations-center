package entities

// Code generation directives.
//
//generate-database:mapper target cluster.mapper.go
//generate-database:mapper reset
//
//generate-database:mapper stmt -e cluster objects
//generate-database:mapper stmt -e cluster objects-by-Name
//generate-database:mapper stmt -e cluster names
//generate-database:mapper stmt -e cluster id
//generate-database:mapper stmt -e cluster create
//generate-database:mapper stmt -e cluster update
//generate-database:mapper stmt -e cluster rename
//generate-database:mapper stmt -e cluster delete-by-Name
//
//generate-database:mapper method -e cluster ID
//generate-database:mapper method -e cluster Exists
//generate-database:mapper method -e cluster GetOne
//generate-database:mapper method -e cluster GetMany
//generate-database:mapper method -e cluster GetNames
//generate-database:mapper method -e cluster Create
//generate-database:mapper method -e cluster Update
//generate-database:mapper method -e cluster Rename
//generate-database:mapper method -e cluster DeleteOne-by-Name
