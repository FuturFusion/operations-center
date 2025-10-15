export interface ClusterTemplateVariable {
  description: string;
  default: string;
}

export interface ClusterTemplate {
  name: string;
  description: string;
  service_config_template: string;
  application_config_template: string;
  variables: Record<string, ClusterTemplateVariable>;
  last_updated: string;
}

export interface ClusterTemplateFormValues {
  name: string;
  description: string;
  service_config_template: string;
  application_config_template: string;
  variables: Record<string, ClusterTemplateVariable>;
}
