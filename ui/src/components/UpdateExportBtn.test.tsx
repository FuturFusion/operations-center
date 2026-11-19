import { screen, waitFor } from "@testing-library/react";
import { beforeEach, expect, test, vi } from "vitest";
import UpdateExportBtn from "components/UpdateExportBtn";
import {
  apiError,
  Handler,
  mockFetch,
  networkError,
  raw,
  Routes,
} from "test/fetch";
import { renderWithProviders } from "test/render";
import { Update } from "types/update";
import { downloadFile } from "util/util";

vi.mock("util/util", async (importOriginal) => ({
  ...(await importOriginal<typeof import("util/util")>()),
  downloadFile: vi.fn(),
}));

beforeEach(() => {
  vi.mocked(downloadFile).mockClear();
});

const uuid = "3b9d0f85-67b4-480e-b369-fef25e9d8ccc";
const archiveURL = `/1.0/provisioning/updates/${uuid}/archive`;
const head = `HEAD ${archiveURL}`;
const get = `GET ${archiveURL}`;

const update = {
  uuid,
  version: "202610272234",
  update_status: "ready",
} as Update;

const exportWith = async (routes: Routes) => {
  const api = mockFetch(routes);
  const view = renderWithProviders(<UpdateExportBtn update={update} />);

  await view.user.click(screen.getByTitle("Export update"));

  // The button is back once the check finished.
  await waitFor(() => {
    expect(screen.getByTitle("Export update")).toBeInTheDocument();
  });

  return api;
};

const notFound = (): Handler => raw(404, "");

test("export downloads the archive after the HEAD request", async () => {
  const api = await exportWith({ [head]: raw(200, "") });

  expect(api.calls(head)).toHaveLength(1);
  expect(api.calls(get)).toHaveLength(0);
  expect(downloadFile).toHaveBeenCalledWith(
    archiveURL,
    "update-202610272234.tar",
  );
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
});

test("export shows the API error of the GET request", async () => {
  const api = await exportWith({
    [head]: notFound(),
    [get]: apiError(404, `Update "${uuid}" has no signed manifest`, {
      reason: "",
      hint: "Refresh the updates.",
    }),
  });

  expect(api.calls(head)).toHaveLength(1);
  expect(api.calls(get)).toHaveLength(1);
  const toast = screen.getByRole("alert");
  expect(toast).toHaveTextContent(
    `Update "${uuid}" has no signed manifest Refresh the updates.`,
  );
  expect(toast).toHaveClass("bg-danger");
  expect(downloadFile).not.toHaveBeenCalled();
});

test("export shows a non JSON error of the GET request", async () => {
  const api = await exportWith({
    [head]: raw(502, ""),
    [get]: raw(502, "Bad Gateway"),
  });

  expect(api.calls(head)).toHaveLength(1);
  expect(api.calls(get)).toHaveLength(1);
  const toast = screen.getByRole("alert");
  expect(toast).toHaveTextContent("Error during update export: Error: 502");
  expect(toast).toHaveClass("bg-danger");
  expect(downloadFile).not.toHaveBeenCalled();
});

test("export downloads the archive if the GET request succeeds", async () => {
  const api = await exportWith({
    [head]: notFound(),
    [get]: raw(200, "archive content"),
  });

  expect(api.calls(head)).toHaveLength(1);
  expect(api.calls(get)).toHaveLength(1);
  expect(downloadFile).toHaveBeenCalledWith(
    archiveURL,
    "update-202610272234.tar",
  );
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
});

test("export fails with a network error", async () => {
  const api = await exportWith({ [head]: networkError() });

  expect(api.calls(head)).toHaveLength(1);
  expect(api.calls(get)).toHaveLength(0);
  const toast = screen.getByRole("alert");
  expect(toast).toHaveTextContent(
    "Error during update export: TypeError: Failed to fetch",
  );
  expect(toast).toHaveClass("bg-danger");
  expect(downloadFile).not.toHaveBeenCalled();
});

test("export is hidden for an update that is not ready", () => {
  renderWithProviders(
    <UpdateExportBtn update={{ ...update, update_status: "pending" }} />,
  );

  expect(screen.queryByTitle("Export update")).not.toBeInTheDocument();
});
