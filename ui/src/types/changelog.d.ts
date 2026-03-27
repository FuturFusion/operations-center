export interface ChangelogEntry {
  added?: string[];
  updated?: string[];
  removed?: string[];
}

export interface Changelog {
  current_version: string;
  prior_version: string;
  channel: string;
  components: Record<string, ChangelogEntry>;
}
