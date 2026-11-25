import { ReactElement } from "react";
import {
  QueryClient,
  QueryClientProvider,
  QueryKey,
} from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { expect } from "vitest";
import Notification from "components/Notification";
import { AuthProvider } from "context/authProvider";
import { NotificationType } from "context/notificationContext";
import { NotificationProvider } from "context/notificationProvider";
import { Settings } from "types/server";

interface Options {
  // Initial URL of the router.
  route?: string;
  // Merged into the default settings, which describe an OIDC login. With null, they are fetched.
  settings?: Partial<Settings> | null;
  // Data preloaded into the query cache.
  queryData?: [QueryKey, unknown][];
  // Whether to show the notifications, false for UI rendering them itself.
  notifications?: boolean;
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

  if (options.settings !== null) {
    queryClient.setQueryData(["settings"], {
      ...defaultSettings,
      ...options.settings,
    });
  }

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
            {options.notifications !== false && <Notification />}
          </NotificationProvider>
        </MemoryRouter>
      </AuthProvider>
    </QueryClientProvider>,
  );

  return { ...result, queryClient, user };
};

// Spelled out instead of reusing notificationVariant, so a broken mapping fails the tests.
const toastClass: Record<NotificationType, string> = {
  info: "bg-primary",
  success: "bg-success",
  warning: "bg-warning",
  error: "bg-danger",
};

// expectToast asserts the single shown notification, its text and its type.
export const expectToast = async (
  text: string | RegExp,
  type: NotificationType,
) => {
  const toasts = await waitFor(() => {
    const found = screen
      .getAllByRole("alert")
      .filter((el) => el.classList.contains("toast"));
    expect(found).toHaveLength(1);
    return found;
  });

  const toast = toasts[0];
  expect(toast).toHaveTextContent(text);
  expect(toast).toHaveClass(toastClass[type]);
};
