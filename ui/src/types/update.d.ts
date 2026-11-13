export interface Update {
  uuid: string;
  version: string;
  published_at: string;
  severity: string;
  origin: string;
  channels: string[] | null;
  upstream_channels: string[] | null;
  changelog: string;
  update_status: string;
}

export interface UpdateFormValues {
  channels: string[];
}

export interface UpdateFile {
  filename: string;
  size: number;
  sha256: string;
  component: string;
  type: string;
  architecture: string;
}
