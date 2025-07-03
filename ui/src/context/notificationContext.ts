import { createContext, useContext } from "react";

interface NotifyFunctions {
  info: (message: string) => void;
  success: (message: string) => void;
  error: (message: string) => void;
}

interface Notification {
  message: string;
  type: string;
}

interface ContextProps {
  notify: NotifyFunctions;
  notification: Notification;
}

export const NotificationContext = createContext<ContextProps>({
  notify: {
    info: () => undefined,
    success: () => undefined,
    error: () => undefined,
  },
  notification: { message: "", type: "primary" },
});

export const useNotification = () => {
  return useContext(NotificationContext);
};
