package entities

// Code generation directives.
//
//generate-database:mapper target cluster_template.mapper.go
//generate-database:mapper reset
//
//generate-database:mapper stmt -e cluster_template objects table=cluster_templates
//generate-database:mapper stmt -e cluster_template objects-by-Name table=cluster_templates
//generate-database:mapper stmt -e cluster_template names table=cluster_templates
//generate-database:mapper stmt -e cluster_template id table=cluster_templates
//generate-database:mapper stmt -e cluster_template create table=cluster_templates
//generate-database:mapper stmt -e cluster_template update table=cluster_templates
//generate-database:mapper stmt -e cluster_template rename table=cluster_templates
//generate-database:mapper stmt -e cluster_template delete-by-Name table=cluster_templates
//
//generate-database:mapper method -e cluster_template ID table=cluster_templates
//generate-database:mapper method -e cluster_template Exists table=cluster_templates
//generate-database:mapper method -e cluster_template GetOne table=cluster_templates
//generate-database:mapper method -e cluster_template GetMany table=cluster_templates
//generate-database:mapper method -e cluster_template GetNames table=cluster_templates
//generate-database:mapper method -e cluster_template Create table=cluster_templates
//generate-database:mapper method -e cluster_template Update table=cluster_templates
//generate-database:mapper method -e cluster_template Rename table=cluster_templates
//generate-database:mapper method -e cluster_template DeleteOne-by-Name table=cluster_templates

type ClusterTemplateFilter struct {
	Name *string
}
