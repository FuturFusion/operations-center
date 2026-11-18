// @vitest-environment node
import { expect, test } from "vitest";
import { primaryApplicationVersion } from "util/server";
import { ApplicationVersionData, Server } from "types/server";

const server = (applications: ApplicationVersionData[] | null) =>
  ({ version_data: { applications } }) as Server;

test.each([
  { name: "no applications", applications: null, want: "" },
  {
    name: "secondary application only",
    applications: [{ name: "incus-ceph", version: "1" }],
    want: "",
  },
  {
    name: "primary after secondary",
    applications: [
      { name: "incus-ceph", version: "1" },
      { name: "incus", version: "6.0" },
    ],
    want: "6.0",
  },
  {
    name: "friendly version preferred",
    applications: [
      { name: "incus", version: "202611180000", friendly_version: "6.18" },
    ],
    want: "6.18",
  },
])("primaryApplicationVersion with $name", ({ applications, want }) => {
  expect(primaryApplicationVersion(server(applications))).toBe(want);
});
