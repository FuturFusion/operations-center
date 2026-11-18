import { screen } from "@testing-library/react";
import { expect, test } from "vitest";
import Sidebar from "components/Sidebar";
import { renderWithProviders } from "test/render";

test.each(["oidc", "tls", "untrusted"])(
  "shows the username only for an OIDC login (%s)",
  (auth) => {
    renderWithProviders(<Sidebar />, {
      settings: { auth },
      queryData: [[["os-check"], false]],
    });

    if (auth === "oidc") {
      expect(screen.getByText("user@example.com")).toBeInTheDocument();
    } else {
      expect(screen.queryByText("user@example.com")).not.toBeInTheDocument();
    }
  },
);
