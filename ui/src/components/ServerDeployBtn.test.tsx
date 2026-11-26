import { screen, waitFor, within } from "@testing-library/react";
import { expect, test } from "vitest";
import ServerDeployBtn from "components/ServerDeployBtn";
import { apiError, Handler, mockFetch, ok } from "test/fetch";
import { renderWithProviders } from "test/render";
import { Server } from "types/server";

// Omit BMC data to verify that the dialog loads the full server.
const listEntry = { name: "server01" } as Server;
const getServer = "GET /1.0/provisioning/servers/server01";
const serverWithMedia = {
  name: "server01",
  bmc_data: {
    virtual_media: {
      cd: { id: "cd", media_types: ["CD", "DVD"] },
    },
  },
};

const openDialogWith = async (serverHandler: Handler) => {
  const api = mockFetch({
    [getServer]: serverHandler,
    "GET /1.0/provisioning/tokens": ok([]),
    "GET /1.0/provisioning/channels": ok([]),
  });
  const view = renderWithProviders(<ServerDeployBtn server={listEntry} />);

  expect(api.calls(getServer)).toHaveLength(0);

  await view.user.click(screen.getByTitle("Deploy IncusOS"));

  return { api };
};

const optionTexts = (select: HTMLElement) =>
  within(select)
    .getAllByRole("option")
    .map((option) => option.textContent);

test("dialog loads the virtual media of the server", async () => {
  const { api } = await openDialogWith(ok(serverWithMedia));

  const select = screen.getByLabelText("Virtual media");
  await within(select).findByRole("option", { name: "cd (CD, DVD)" });

  expect(optionTexts(select), "the options list the virtual media").toEqual([
    "Automatic",
    "cd (CD, DVD)",
  ]);
  expect(select, "the select is enabled after loading").toBeEnabled();
  expect(
    screen.queryByText(/Failed to load the virtual media/),
    "no load error is shown",
  ).not.toBeInTheDocument();
  expect(api.calls(getServer)).toHaveLength(1);
});

test("dialog disables the virtual media while the server loads", async () => {
  let release = () => {};
  const pending = new Promise<void>((resolve) => {
    release = resolve;
  });
  const { api } = await openDialogWith(async (request) => {
    await pending;
    return ok(serverWithMedia)(request);
  });

  const select = screen.getByLabelText("Virtual media");
  expect(select, "the select is disabled while loading").toBeDisabled();
  expect(
    screen.getByText("Loading the virtual media ..."),
    "the loading text is shown",
  ).toBeInTheDocument();

  release();
  await within(select).findByRole("option", { name: "cd (CD, DVD)" });

  expect(select, "the select is enabled after loading").toBeEnabled();
  expect(
    screen.queryByText("Loading the virtual media ..."),
    "the loading text is removed after loading",
  ).not.toBeInTheDocument();
  expect(api.calls(getServer)).toHaveLength(1);
});

test("dialog shows an error if the server can not be loaded", async () => {
  const { api } = await openDialogWith(apiError(500, "Database is down"));

  expect(
    await screen.findByText(
      "Failed to load the virtual media: Database is down",
    ),
    "the error message is shown without the error class name",
  ).toBeInTheDocument();

  const select = screen.getByLabelText("Virtual media");
  await waitFor(() =>
    expect(select, "the select is enabled after the error").toBeEnabled(),
  );
  expect(optionTexts(select), "only the automatic option remains").toEqual([
    "Automatic",
  ]);
  expect(api.calls(getServer)).toHaveLength(1);
});
