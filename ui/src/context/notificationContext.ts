import { createContext, useContext } from "react";

export type NotificationType = "info" | "success" | "warning" | "error";

interface NotifyFunctions {
  info: (message: string) => void;
  success: (message: string) => void;
  warning: (message: string) => void;
  error: (message: string) => void;
}

export interface Notification {
  id: number;
  type: NotificationType;
  message: string;
  // RFC 3339 format.
  timestamp: string;
  // A notification that is not visible is only listed in the history.
  visible: boolean;
}

interface ContextProps {
  notify: NotifyFunctions;
  // Newest first.
  notifications: Notification[];
  dismiss: (id: number) => void;
  dismissAll: () => void;
  clear: () => void;
}

export const NotificationContext = createContext<ContextProps>({
  notify: {
    info: () => undefined,
    success: () => undefined,
    warning: () => undefined,
    error: () => undefined,
  },
  notifications: [],
  dismiss: () => undefined,
  dismissAll: () => undefined,
  clear: () => undefined,
});

export const useNotification = () => {
  return useContext(NotificationContext);
};
