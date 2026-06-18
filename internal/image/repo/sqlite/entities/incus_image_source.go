package entities

// Code generation directives.
//
//generate-database:mapper target incus_image_source.mapper.go
//generate-database:mapper reset
//
//generate-database:mapper stmt -e incus_image_source objects table=incus_image_sources
//generate-database:mapper stmt -e incus_image_source objects-by-ID table=incus_image_sources
//generate-database:mapper stmt -e incus_image_source objects-by-Name table=incus_image_sources
//generate-database:mapper stmt -e incus_image_source names table=incus_image_sources
//generate-database:mapper stmt -e incus_image_source id table=incus_image_sources
//generate-database:mapper stmt -e incus_image_source create table=incus_image_sources
//generate-database:mapper stmt -e incus_image_source update table=incus_image_sources
//generate-database:mapper stmt -e incus_image_source delete-by-Name table=incus_image_sources
//
//generate-database:mapper method -e incus_image_source ID
//generate-database:mapper method -e incus_image_source Exists
//generate-database:mapper method -e incus_image_source GetOne
//generate-database:mapper method -e incus_image_source GetMany
//generate-database:mapper method -e incus_image_source GetNames
//generate-database:mapper method -e incus_image_source Create
//generate-database:mapper method -e incus_image_source Update
//generate-database:mapper method -e incus_image_source DeleteOne-by-Name
