import { FC, ReactNode, useRef, useState } from "react";
import { NotificationContext } from "context/notificationContext";

export const NotificationProvider: FC<{ children: ReactNode }> = ({
  children,
}) => {
  const [notification, setNotification] = useState({
    message: "",
    type: "primary",
  });
  const timeoutRef = useRef(-1);

  const setupTimeout = () => {
    clearTimeout(timeoutRef.current);
    timeoutRef.current = setTimeout(
      () => setNotification({ message: "", type: "primary" }),
      5000,
    );
  };

  const notify = {
    info: (message: string) => {
      setNotification({ message: message, type: "primary" });
      setupTimeout();
    },
    success: (message: string) => {
      setNotification({ message: message, type: "success" });
      setupTimeout();
    },
    error: (message: string) => {
      setNotification({ message: message, type: "danger" });
      setupTimeout();
    },
  };

  return (
    <NotificationContext.Provider value={{ notify, notification }}>
      {children}
    </NotificationContext.Provider>
  );
};
