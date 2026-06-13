export interface IncusImage {
  name: string;
  aliases: string[];
  description: string;
  os: string;
  release: string;
  arch: string;
  variant: string;
  versions: Record<string, IncusImageVersion>;
  last_updated: string;
}

export interface IncusImageVersion {
  items: Record<string, IncusImageVersionItem>;
}

export interface IncusImageVersionItem {
  "combined_disk-kvm-img_sha256"?: string;
  combined_rootxz_sha256?: string;
  combined_squashfs_sha256?: string;
  combined_type?: string;
  ftype: string;
  path: string;
  sha256?: string;
  size: number;
}

export interface IncusImageFormValues {
  aliases: string[];
  description: string;
}
