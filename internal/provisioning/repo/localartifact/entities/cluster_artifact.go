package entities

// Code generation directives.
//
//generate-database:mapper target cluster_artifact.mapper.go
//generate-database:mapper reset
//
//generate-database:mapper stmt -e cluster_artifact objects table=cluster_artifacts
//generate-database:mapper stmt -e cluster_artifact objects-by-Cluster table=cluster_artifacts
//generate-database:mapper stmt -e cluster_artifact objects-by-Cluster-and-Name table=cluster_artifacts
//generate-database:mapper stmt -e cluster_artifact id table=cluster_artifacts
//generate-database:mapper stmt -e cluster_artifact create table=cluster_artifacts
//
//generate-database:mapper method -e cluster_artifact ID table=cluster_artifacts
//generate-database:mapper method -e cluster_artifact Exists table=cluster_artifacts
//generate-database:mapper method -e cluster_artifact GetOne table=cluster_artifacts
//generate-database:mapper method -e cluster_artifact GetMany table=cluster_artifacts
//generate-database:mapper method -e cluster_artifact Create table=cluster_artifacts

type ClusterArtifactFilter struct {
	Name    *string
	Cluster *string
}
