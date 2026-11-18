// @vitest-environment node
import { expect, test } from "vitest";
import { bytesToHumanReadable, humanReadableToBytes } from "util/util";

test.each([
  { bytes: 0, decimalPlaces: undefined, want: "0 B" },
  { bytes: 512, decimalPlaces: undefined, want: "512.00 B" },
  { bytes: 1536, decimalPlaces: undefined, want: "1.50 KiB" },
  { bytes: 5 * 1024 ** 3, decimalPlaces: 0, want: "5 GiB" },
])("bytesToHumanReadable($bytes)", ({ bytes, decimalPlaces, want }) => {
  expect(bytesToHumanReadable(bytes, decimalPlaces)).toBe(want);
});

test.each([
  { input: "10 B", want: 10 },
  { input: "1 KiB", want: 1024 },
  { input: "1.5GiB", want: 1.5 * 1024 ** 3 },
  { input: "1 kib", want: 1024 },
  { input: "1 KB", want: 1000 },
  { input: "1.5 GB", want: 1.5e9 },
])("humanReadableToBytes($input)", ({ input, want }) => {
  expect(humanReadableToBytes(input)).toBe(want);
});

test.each(["", "KiB", "1", "1 XB", "-1 KiB"])(
  "humanReadableToBytes rejects %j",
  (input) => {
    expect(() => humanReadableToBytes(input)).toThrow("Invalid format");
  },
);
