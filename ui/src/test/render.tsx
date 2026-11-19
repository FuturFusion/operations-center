import { ReactElement } from "react";
import {
  QueryClient,
  QueryClientProvider,
  QueryKey,
} from "@tanstack/react-query";
import { render } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import Notification from "components/Notification";
import { AuthProvider } from "context/authProvider";
import { NotificationProvider } from "context/notificationProvider";
import { Settings } from "types/server";

interface Options {
  // Initial URL of the router.
  route?: string;
  // Merged into the default settings, which describe an OIDC login.
  settings?: Partial<Settings>;
  // Data preloaded into the query cache.
  queryData?: [QueryKey, unknown][];
}

const defaultSettings: Settings = {
  api_status: "stable",
  api_version: 1,
  auth: "oidc",
  auth_user_name: "user@example.com",
  server_version: "1.0.0",
};

// renderWithProviders renders the UI with the providers of the app and shows its notifications.
export const renderWithProviders = (
  ui: ReactElement,
  options: Options = {},
) => {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: Infinity },
      mutations: { retry: false },
    },
  });

  queryClient.setQueryData(["settings"], {
    ...defaultSettings,
    ...options.settings,
  });

  for (const [key, data] of options.queryData ?? []) {
    queryClient.setQueryData(key, data);
  }

  const user = userEvent.setup();

  const result = render(
    <QueryClientProvider client={queryClient}>
      <AuthProvider>
        <MemoryRouter initialEntries={[options.route ?? "/ui"]}>
          <NotificationProvider>
            {ui}
            <Notification />
          </NotificationProvider>
        </MemoryRouter>
      </AuthProvider>
    </QueryClientProvider>,
  );

  return { ...result, queryClient, user };
};
