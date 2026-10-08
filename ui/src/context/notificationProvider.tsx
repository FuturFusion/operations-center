import {
  FC,
  ReactNode,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import {
  Notification,
  NotificationContext,
  NotificationType,
} from "context/notificationContext";

const autoDismissDelay = 5000;
const maxHistory = 50;

// Errors and warnings stay until the user dismisses them.
const isSticky = (type: NotificationType) =>
  type == "error" || type == "warning";

export const NotificationProvider: FC<{ children: ReactNode }> = ({
  children,
}) => {
  const [notifications, setNotifications] = useState<Notification[]>([]);
  const current = useRef<Notification[]>([]);
  const nextId = useRef(0);
  const timers = useRef(new Map<number, ReturnType<typeof setTimeout>>());

  useEffect(() => {
    const pending = timers.current;

    return () => {
      pending.forEach((timer) => clearTimeout(timer));
      pending.clear();
    };
  }, []);

  const update = useCallback((next: Notification[]) => {
    current.current = next;
    setNotifications(next);
  }, []);

  const clearTimers = useCallback(() => {
    timers.current.forEach((timer) => clearTimeout(timer));
    timers.current.clear();
  }, []);

  const dismiss = useCallback(
    (id: number) => {
      clearTimeout(timers.current.get(id));
      timers.current.delete(id);
      update(
        current.current.map((n) => (n.id == id ? { ...n, visible: false } : n)),
      );
    },
    [update],
  );

  const dismissAll = useCallback(() => {
    clearTimers();
    update(current.current.map((n) => ({ ...n, visible: false })));
  }, [clearTimers, update]);

  const clear = useCallback(() => {
    clearTimers();
    update([]);
  }, [clearTimers, update]);

  const add = useCallback(
    (type: NotificationType, message: string) => {
      // A repeated notification replaces the newest one. This avoids a stack
      // of identical messages.
      const newest = current.current[0];
      const isRepeat =
        newest?.visible && newest.type == type && newest.message == message;
      const id = isRepeat ? newest.id : nextId.current++;
      const rest = isRepeat ? current.current.slice(1) : current.current;
      const notification = {
        id,
        type,
        message,
        timestamp: new Date().toISOString(),
        visible: true,
      };

      update([notification, ...rest].slice(0, maxHistory));

      if (!isSticky(type)) {
        clearTimeout(timers.current.get(id));
        timers.current.set(
          id,
          setTimeout(() => dismiss(id), autoDismissDelay),
        );
      }
    },
    [dismiss, update],
  );

  const notify = useMemo(
    () => ({
      info: (message: string) => add("info", message),
      success: (message: string) => add("success", message),
      warning: (message: string) => add("warning", message),
      error: (message: string) => add("error", message),
    }),
    [add],
  );

  return (
    <NotificationContext.Provider
      value={{ notify, notifications, dismiss, dismissAll, clear }}
    >
      {children}
    </NotificationContext.Provider>
  );
};
