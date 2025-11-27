package entities

// Code generation directives.
//
//generate-database:mapper target server.mapper.go
//generate-database:mapper reset
//
//generate-database:mapper stmt -e server objects
//generate-database:mapper stmt -e server objects-by-Name
//generate-database:mapper stmt -e server objects-by-Cluster
//generate-database:mapper stmt -e server objects-by-Cluster-and-Status
//generate-database:mapper stmt -e server objects-by-Status
//generate-database:mapper stmt -e server objects-by-Certificate
//generate-database:mapper stmt -e server objects-by-Type
//generate-database:mapper stmt -e server names
//generate-database:mapper stmt -e server names-by-Cluster
//generate-database:mapper stmt -e server id
//generate-database:mapper stmt -e server create
//generate-database:mapper stmt -e server update
//generate-database:mapper stmt -e server rename
//generate-database:mapper stmt -e server delete-by-Name
//
//generate-database:mapper method -e server ID
//generate-database:mapper method -e server Exists
//generate-database:mapper method -e server GetOne
//generate-database:mapper method -e server GetMany
//generate-database:mapper method -e server GetNames
//generate-database:mapper method -e server Create
//generate-database:mapper method -e server Update
//generate-database:mapper method -e server Rename
//generate-database:mapper method -e server DeleteOne-by-Name

type ServerFilter struct {
	Name    *string
	Cluster *string
}
