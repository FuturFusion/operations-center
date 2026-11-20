import { screen } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import App from "./App";
import { mockFetch, networkError, ok } from "test/fetch";
import { renderWithProviders } from "test/render";

const settings = (auth: string) => ({
  api_status: "stable",
  api_version: 1,
  auth,
  server_version: "1.0.0",
});

const initialURL = window.location.href;

afterEach(() => {
  window.history.replaceState({}, "", initialURL);
});

const renderApp = () => {
  // App redirects to /ui/ based on the location of the window, not of the router.
  window.history.replaceState({}, "", "/ui/");

  return renderWithProviders(<App />, {
    route: "/ui/",
    settings: null,
    queryData: [[["os-check"], false]],
    notifications: false,
  });
};

test("reports an unreachable server", async () => {
  mockFetch({ "GET /1.0": networkError() });
  renderApp();

  expect(
    await screen.findByText(
      "Error while connecting to Operations Center: Failed to fetch",
    ),
  ).toBeInTheDocument();
  expect(
    screen.queryByText("Welcome to Operations Center"),
  ).not.toBeInTheDocument();
});

test("asks an untrusted user to log in", async () => {
  mockFetch({ "GET /1.0": ok(settings("untrusted")) });
  renderApp();

  expect(await screen.findByText(/Please log in/)).toBeInTheDocument();
  expect(screen.queryByText("Servers")).not.toBeInTheDocument();
});

test("shows the navigation to an authenticated user", async () => {
  mockFetch({ "GET /1.0": ok(settings("tls")) });
  renderApp();

  expect(
    await screen.findByText("Welcome to Operations Center"),
  ).toBeInTheDocument();
  expect(screen.queryByText(/Please log in/)).not.toBeInTheDocument();
  expect(screen.getByText("Servers")).toBeInTheDocument();
});
