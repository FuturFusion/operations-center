import { screen, within } from "@testing-library/react";
import { expect, test } from "vitest";
import ServerDeployBtn from "components/ServerDeployBtn";
import { apiError, Handler, mockFetch, ok } from "test/fetch";
import { renderWithProviders } from "test/render";
import { Server } from "types/server";

// A list entry of the server list does not contain the BMC data.
const listEntry = { name: "server01" } as Server;
const getServer = "GET /1.0/provisioning/servers/server01";

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

test("dialog loads the virtual media of the server", async () => {
  const { api } = await openDialogWith(
    ok({
      name: "server01",
      bmc_data: {
        virtual_media: {
          cd: { id: "cd", media_types: ["CD", "DVD"] },
        },
      },
    }),
  );

  const select = screen.getByLabelText("Virtual media");
  expect(
    await within(select).findByRole("option", { name: "cd (CD, DVD)" }),
  ).toBeInTheDocument();
  expect(select).toBeEnabled();
  expect(api.calls(getServer)).toHaveLength(1);
});

test("dialog shows an error if the server can not be loaded", async () => {
  const { api } = await openDialogWith(apiError(500, "Database is down"));

  expect(
    await screen.findByText(
      /Failed to load the virtual media:.*Database is down/,
    ),
  ).toBeInTheDocument();
  expect(
    within(screen.getByLabelText("Virtual media")).getAllByRole("option"),
  ).toHaveLength(1);
  expect(api.calls(getServer)).toHaveLength(1);
});
