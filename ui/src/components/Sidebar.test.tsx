import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";
import { expect, test, vi } from "vitest";
import Sidebar from "components/Sidebar";
import { AuthProvider } from "context/authProvider";

// The Offcanvas of the notification history needs matchMedia, which jsdom lacks.
vi.stubGlobal("matchMedia", () => ({
  matches: false,
  addListener: () => {},
  removeListener: () => {},
}));

test.each(["oidc", "tls", "untrusted"])(
  "shows the username only for an OIDC login (%s)",
  (auth) => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { staleTime: Infinity } },
    });
    queryClient.setQueryData(["settings"], {
      auth,
      auth_user_name: "user@example.com",
      server_version: "1.0.0",
    });
    queryClient.setQueryData(["os-check"], false);

    render(
      <QueryClientProvider client={queryClient}>
        <AuthProvider>
          <MemoryRouter>
            <Sidebar />
          </MemoryRouter>
        </AuthProvider>
      </QueryClientProvider>,
    );

    if (auth === "oidc") {
      expect(screen.getByText("user@example.com")).toBeInTheDocument();
    } else {
      expect(screen.queryByText("user@example.com")).not.toBeInTheDocument();
    }
  },
);
