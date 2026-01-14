package entities

// Code generation directives.
//
//generate-database:mapper target exposedchannel_update.mapper.go
//generate-database:mapper reset
//
//generate-database:mapper stmt -e exposedchannel_update objects table=exposedchannels_updates
//generate-database:mapper stmt -e exposedchannel_update objects-by-ExposedchannelID table=exposedchannels_updates
//generate-database:mapper stmt -e exposedchannel_update objects-by-UpdateID table=exposedchannels_updates
//generate-database:mapper stmt -e exposedchannel_update objects-by-ExposedchannelID-and-UpdateID table=exposedchannels_updates
//generate-database:mapper stmt -e exposedchannel_update create table=exposedchannels_updates
//generate-database:mapper stmt -e exposedchannel_update delete-by-ExposedchannelID table=exposedchannels_updates
//generate-database:mapper stmt -e exposedchannel_update delete-by-UpdateID table=exposedchannels_updates
//generate-database:mapper stmt -e exposedchannel_update delete-by-ExposedchannelID-and-UpdateID table=exposedchannels_updates
//
//generate-database:mapper method -e exposedchannel_update GetMany struct=Exposedchannel
//generate-database:mapper method -e exposedchannel_update DeleteMany struct=Exposedchannel
//generate-database:mapper method -e exposedchannel_update Create struct=Exposedchannel
//generate-database:mapper method -e exposedchannel_update Update struct=Exposedchannel
//
//generate-database:mapper method -e exposedchannel_update GetMany struct=Update
//generate-database:mapper method -e exposedchannel_update DeleteMany struct=Update
//generate-database:mapper method -e exposedchannel_update Create struct=Update
//generate-database:mapper method -e exposedchannel_update Update struct=Update
