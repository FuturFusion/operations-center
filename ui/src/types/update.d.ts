export interface Update {
  uuid: string;
  version: string;
  published_at: string;
  severity: string;
  origin: string;
  channel: string;
  changelog: string;
}

export interface UpdateFile {
  filename: string;
  size: number;
  sha256: string;
  component: string;
  type: string;
  architecture: string;
}
