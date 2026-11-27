package entities

import "github.com/FuturFusion/operations-center/internal/provisioning"

// Code generation directives.
//
//generate-database:mapper target media.mapper.go
//generate-database:mapper reset
//
//generate-database:mapper stmt -e media objects table=media
//generate-database:mapper stmt -e media objects-by-Type table=media
//generate-database:mapper stmt -e media objects-by-MemberID table=media
//generate-database:mapper stmt -e media objects-by-Type-and-Name table=media
//generate-database:mapper stmt -e media objects-by-Type-and-MemberID table=media
//generate-database:mapper stmt -e media id table=media
//generate-database:mapper stmt -e media create table=media
//generate-database:mapper stmt -e media update table=media
//generate-database:mapper stmt -e media delete-by-Type-and-Name table=media
//
//generate-database:mapper method -e media ID table=media
//generate-database:mapper method -e media Exists table=media
//generate-database:mapper method -e media GetOne table=media
//generate-database:mapper method -e media GetMany table=media
//generate-database:mapper method -e media Create table=media
//generate-database:mapper method -e media Update table=media
//generate-database:mapper method -e media DeleteOne-by-Type-and-Name table=media

// MediaType lets the generated code name the media type of the provisioning package.
type MediaType = provisioning.MediaType
