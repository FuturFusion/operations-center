package entities

// Code generation directives.
//
//generate-database:mapper target incus_image.mapper.go
//generate-database:mapper reset
//
//generate-database:mapper stmt -e incus_image objects table=incus_images
//generate-database:mapper stmt -e incus_image objects-by-ID table=incus_images
//generate-database:mapper stmt -e incus_image objects-by-Name table=incus_images
//generate-database:mapper stmt -e incus_image names table=incus_images
//generate-database:mapper stmt -e incus_image id table=incus_images
//generate-database:mapper stmt -e incus_image create table=incus_images
//generate-database:mapper stmt -e incus_image update table=incus_images
//generate-database:mapper stmt -e incus_image delete-by-Name table=incus_images
//
//generate-database:mapper method -e incus_image ID
//generate-database:mapper method -e incus_image Exists
//generate-database:mapper method -e incus_image GetOne
//generate-database:mapper method -e incus_image GetMany
//generate-database:mapper method -e incus_image GetNames
//generate-database:mapper method -e incus_image Create
//generate-database:mapper method -e incus_image Update
//generate-database:mapper method -e incus_image DeleteOne-by-Name
