// @vitest-environment node
import { expect, test } from "vitest";
import { formatDate, formatDateTime } from "util/date";

// Without an offset the input is parsed as local time, keeping the test independent of the time zone.
const input = "2026-11-18T09:05:07";

test("formatDate", () => {
  expect(formatDate(input)).toBe("2026-11-18 09:05");
});

test("formatDateTime", () => {
  expect(formatDateTime(input)).toBe("2026-11-18 09:05:07");
});

test.each([undefined, "", "0001-01-01T00:00:00Z"])(
  "formats unset date %j as empty",
  (value) => {
    expect(formatDate(value)).toBe("");
    expect(formatDateTime(value)).toBe("");
  },
);
