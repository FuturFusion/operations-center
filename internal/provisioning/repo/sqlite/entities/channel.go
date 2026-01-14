package entities

// Code generation directives.
//
//generate-database:mapper target channel.mapper.go
//generate-database:mapper reset
//
//generate-database:mapper stmt -e channel objects table=channels
//generate-database:mapper stmt -e channel objects-by-Name table=channels
//generate-database:mapper stmt -e channel names table=channels
//generate-database:mapper stmt -e channel id table=channels
//generate-database:mapper stmt -e channel create table=channels
//generate-database:mapper stmt -e channel update table=channels
//generate-database:mapper stmt -e channel delete-by-Name table=channels
//
//generate-database:mapper method -e channel ID
//generate-database:mapper method -e channel Exists
//generate-database:mapper method -e channel GetOne
//generate-database:mapper method -e channel GetMany
//generate-database:mapper method -e channel GetNames
//generate-database:mapper method -e channel Create
//generate-database:mapper method -e channel Update
//generate-database:mapper method -e channel DeleteOne-by-Name
