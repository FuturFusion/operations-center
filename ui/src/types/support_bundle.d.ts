export type SupportBundleType = "cluster" | "project" | "instance" | "server";

export interface SupportBundlePost {
  type: SupportBundleType;
  cluster?: string;
  project?: string;
  name?: string;
  server?: string;
}
