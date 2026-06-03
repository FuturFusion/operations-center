package entities

// Code generation directives.
//
//generate-database:mapper target image_source.mapper.go
//generate-database:mapper reset
//
//generate-database:mapper stmt -e image_source objects table=image_sources
//generate-database:mapper stmt -e image_source objects-by-ID table=image_sources
//generate-database:mapper stmt -e image_source objects-by-Name table=image_sources
//generate-database:mapper stmt -e image_source names table=image_sources
//generate-database:mapper stmt -e image_source id table=image_sources
//generate-database:mapper stmt -e image_source create table=image_sources
//generate-database:mapper stmt -e image_source update table=image_sources
//generate-database:mapper stmt -e image_source delete-by-Name table=image_sources
//
//generate-database:mapper method -e image_source ID
//generate-database:mapper method -e image_source Exists
//generate-database:mapper method -e image_source GetOne
//generate-database:mapper method -e image_source GetMany
//generate-database:mapper method -e image_source GetNames
//generate-database:mapper method -e image_source Create
//generate-database:mapper method -e image_source Update
//generate-database:mapper method -e image_source DeleteOne-by-Name
