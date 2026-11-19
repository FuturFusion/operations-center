import { screen, waitFor } from "@testing-library/react";
import { expect, test, vi } from "vitest";
import ServerRebootBtn from "components/ServerRebootBtn";
import { apiError, Handler, mockFetch, networkError, ok } from "test/fetch";
import { renderWithProviders } from "test/render";
import { Server } from "types/server";

const server = { name: "server01" } as Server;
const reboot = "POST /1.0/provisioning/servers/server01/system/:reboot";

const rebootWith = async (handler: Handler) => {
  const api = mockFetch({ [reboot]: handler });
  const view = renderWithProviders(<ServerRebootBtn server={server} />);
  const invalidate = vi.spyOn(view.queryClient, "invalidateQueries");

  await view.user.click(screen.getByTitle("Reboot server"));
  expect(
    screen.getByText('Are you sure you want to reboot the server "server01"?'),
  ).toBeInTheDocument();

  await view.user.click(screen.getByRole("button", { name: "Reboot" }));

  // The modal closes once the request finished.
  await waitFor(() => {
    expect(
      screen.queryByRole("button", { name: "Reboot" }),
    ).not.toBeInTheDocument();
  });

  expect(api.calls(reboot)).toHaveLength(1);

  return { invalidate };
};

test("reboot succeeds", async () => {
  const { invalidate } = await rebootWith(ok());

  const toast = screen.getByRole("alert");
  expect(toast).toHaveTextContent("Server reboot triggered");
  expect(toast).toHaveClass("bg-success");
  expect(invalidate).toHaveBeenCalledWith({ queryKey: ["servers"] });
});

test("reboot fails with an API error", async () => {
  const { invalidate } = await rebootWith(
    apiError(400, "Server is offline", { reason: "", hint: "Power it on." }),
  );

  const toast = screen.getByRole("alert");
  expect(toast).toHaveTextContent("Server is offline Power it on.");
  expect(toast).toHaveClass("bg-danger");
  expect(invalidate).not.toHaveBeenCalled();
});

test("reboot fails with a network error", async () => {
  const { invalidate } = await rebootWith(networkError());

  const toast = screen.getByRole("alert");
  expect(toast).toHaveTextContent(
    "Error during server reboot: TypeError: Failed to fetch",
  );
  expect(toast).toHaveClass("bg-danger");
  expect(invalidate).not.toHaveBeenCalled();
});
