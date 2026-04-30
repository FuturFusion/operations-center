export interface WarningScope {
  scope: string;
  entity_type: string;
  entity: string;
}

export interface Warning {
  uuid: string;
  status: string;
  scope: WarningScope;
  type: string;
  first_occurrence: string;
  last_occurrence: string;
  last_updated: string;
  messages: string[];
  count: number;
}
