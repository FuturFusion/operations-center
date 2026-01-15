export interface Update {
  uuid: string;
  version: string;
  published_at: string;
  severity: string;
  origin: string;
  upstream_channels: string[];
  changelog: string;
  update_status: string;
}

export interface UpdateFile {
  filename: string;
  size: number;
  sha256: string;
  component: string;
  type: string;
  architecture: string;
}
