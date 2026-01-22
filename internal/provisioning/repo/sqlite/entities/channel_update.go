package entities

// Code generation directives.
//
//generate-database:mapper target channel_update.mapper.go
//generate-database:mapper reset
//
//generate-database:mapper stmt -e channel_update objects table=channels_updates
//generate-database:mapper stmt -e channel_update objects-by-ChannelID table=channels_updates
//generate-database:mapper stmt -e channel_update objects-by-UpdateID table=channels_updates
//generate-database:mapper stmt -e channel_update objects-by-ChannelID-and-UpdateID table=channels_updates
//generate-database:mapper stmt -e channel_update create table=channels_updates
//generate-database:mapper stmt -e channel_update delete-by-ChannelID table=channels_updates
//generate-database:mapper stmt -e channel_update delete-by-UpdateID table=channels_updates
//generate-database:mapper stmt -e channel_update delete-by-ChannelID-and-UpdateID table=channels_updates
//
//generate-database:mapper method -e channel_update GetMany struct=Channel
//generate-database:mapper method -e channel_update DeleteMany struct=Channel
//generate-database:mapper method -e channel_update Create struct=Channel
//generate-database:mapper method -e channel_update Update struct=Channel
//
//generate-database:mapper method -e channel_update GetMany struct=Update
//generate-database:mapper method -e channel_update DeleteMany struct=Update
//generate-database:mapper method -e channel_update Create struct=Update
//generate-database:mapper method -e channel_update Update struct=Update
