import { FC, ReactNode } from "react";
import { act, renderHook } from "@testing-library/react";
import { beforeEach, expect, test, vi } from "vitest";
import { useNotification } from "context/notificationContext";
import { NotificationProvider } from "context/notificationProvider";

const wrapper: FC<{ children: ReactNode }> = ({ children }) => (
  <NotificationProvider>{children}</NotificationProvider>
);

const renderNotifications = () => renderHook(useNotification, { wrapper });

beforeEach(() => {
  vi.useFakeTimers();
});

test("info and success are dismissed after a delay", () => {
  const { result } = renderNotifications();

  act(() => {
    result.current.notify.info("info");
    result.current.notify.success("success");
  });

  expect(result.current.notifications.map((n) => n.visible)).toEqual([
    true,
    true,
  ]);

  act(() => {
    vi.advanceTimersByTime(5000);
  });

  // Dismissed notifications stay in the history.
  expect(result.current.notifications.map((n) => n.visible)).toEqual([
    false,
    false,
  ]);
});

test("errors and warnings stay until dismissed", () => {
  const { result } = renderNotifications();

  act(() => {
    result.current.notify.error("error");
    result.current.notify.warning("warning");
  });

  act(() => {
    vi.advanceTimersByTime(60000);
  });

  expect(result.current.notifications.map((n) => n.visible)).toEqual([
    true,
    true,
  ]);

  act(() => {
    result.current.dismiss(result.current.notifications[1].id);
  });

  expect(result.current.notifications.map((n) => n.visible)).toEqual([
    true,
    false,
  ]);

  act(() => {
    result.current.dismissAll();
  });

  expect(result.current.notifications.map((n) => n.visible)).toEqual([
    false,
    false,
  ]);
});

test("a repeated notification replaces the newest one", () => {
  const { result } = renderNotifications();

  act(() => {
    result.current.notify.info("same");
  });

  act(() => {
    vi.advanceTimersByTime(4000);
    result.current.notify.info("same");
  });

  expect(result.current.notifications).toHaveLength(1);

  // The repeat restarts the dismiss delay.
  act(() => {
    vi.advanceTimersByTime(4000);
  });

  expect(result.current.notifications[0].visible).toBe(true);

  act(() => {
    result.current.notify.error("same");
    result.current.notify.info("same");
  });

  expect(result.current.notifications.map((n) => n.type)).toEqual([
    "info",
    "error",
    "info",
  ]);
});

test("a dismissed notification is not replaced by a repeat", () => {
  const { result } = renderNotifications();

  act(() => {
    result.current.notify.error("same");
  });

  act(() => {
    result.current.dismissAll();
    result.current.notify.error("same");
  });

  expect(result.current.notifications.map((n) => n.visible)).toEqual([
    true,
    false,
  ]);
});

test("history keeps the newest 50 notifications", () => {
  const { result } = renderNotifications();

  act(() => {
    for (let i = 0; i < 60; i++) {
      result.current.notify.error(`error ${i}`);
    }
  });

  expect(result.current.notifications).toHaveLength(50);
  expect(result.current.notifications[0].message).toBe("error 59");
  expect(result.current.notifications[49].message).toBe("error 10");

  act(() => {
    result.current.clear();
  });

  expect(result.current.notifications).toHaveLength(0);
});
