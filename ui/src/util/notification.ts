import type { NotificationType } from "context/notificationContext";

export const notificationVariant: Record<NotificationType, string> = {
  info: "primary",
  success: "success",
  warning: "warning",
  error: "danger",
};
