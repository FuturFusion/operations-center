import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { expect, test } from "vitest";
import DownloadUpdateBtn from "components/DownloadUpdateBtn";
import { apiError, mockFetch, ok, raw, Routes } from "test/fetch";
import { renderWithProviders } from "test/render";

const download = "POST /1.0/provisioning/updates/:download";

const openModal = (routes: Routes) => {
  const api = mockFetch(routes);
  renderWithProviders(<DownloadUpdateBtn />, {
    queryData: [[["channels"], [{ name: "stable" }, { name: "legacy" }]]],
  });

  fireEvent.click(screen.getByRole("button", { name: "Download" }));
  const dialog = screen.getByRole("dialog");

  return { api, dialog };
};

const submit = (dialog: HTMLElement) => {
  fireEvent.click(within(dialog).getByRole("button", { name: "Download" }));
};

test("download is disabled without a version", () => {
  const { dialog } = openModal({});

  expect(
    within(dialog).getByRole("button", { name: "Download" }),
  ).toBeDisabled();
});

test("downloads the given version into the selected channels", async () => {
  const { api, dialog } = openModal({ [download]: ok() });

  fireEvent.change(within(dialog).getByLabelText("Version"), {
    target: { value: " 202610272234 " },
  });
  const legacy = within(dialog).getByRole("option", { name: "legacy" });
  (legacy as HTMLOptionElement).selected = true;
  fireEvent.change(within(dialog).getByLabelText("Channels"));
  submit(dialog);

  expect(api.calls(download)).toHaveLength(1);
  expect(JSON.parse(api.calls(download)[0].body ?? "")).toEqual({
    version: "202610272234",
    channels: ["legacy"],
  });
  const toast = await screen.findByRole("alert");
  expect(toast).toHaveTextContent("Update 202610272234 downloaded");
  expect(toast).toHaveClass("bg-success");
  await waitFor(() =>
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
  );
});

test("download shows the API error and keeps the modal open", async () => {
  const { api, dialog } = openModal({
    [download]: apiError(404, `Update source does not provide version "1"`, {
      reason: "",
      hint: "Export the update from another Operations Center.",
    }),
  });

  fireEvent.change(within(dialog).getByLabelText("Version"), {
    target: { value: "1" },
  });
  submit(dialog);

  expect(api.calls(download)).toHaveLength(1);
  const toast = await screen.findByRole("alert");
  expect(toast).toHaveTextContent(
    `Update source does not provide version "1" Export the update from another Operations Center.`,
  );
  expect(toast).toHaveClass("bg-danger");
  expect(screen.getByRole("dialog")).toBeInTheDocument();
});

test("download shows a non JSON error of a proxy", async () => {
  const { api, dialog } = openModal({
    [download]: raw(504, "<html>Gateway Timeout</html>"),
  });

  fireEvent.change(within(dialog).getByLabelText("Version"), {
    target: { value: "1" },
  });
  submit(dialog);

  expect(api.calls(download)).toHaveLength(1);
  const toast = await screen.findByRole("alert");
  expect(toast).toHaveTextContent(
    "Error during update download: Error: 504 : <html>Gateway Timeout</html>",
  );
  expect(toast).toHaveClass("bg-danger");
  expect(screen.getByRole("dialog")).toBeInTheDocument();
});
