import "@testing-library/jest-dom/vitest";
import { afterEach, vi } from "vitest";
import { takeUnhandledRequests } from "./fetch";

// Modals and offcanvases of react-bootstrap need matchMedia, which jsdom lacks.
if (typeof window != "undefined") {
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    value: (query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: () => {},
      removeListener: () => {},
      addEventListener: () => {},
      removeEventListener: () => {},
      dispatchEvent: () => false,
    }),
  });
}

afterEach(() => {
  vi.restoreAllMocks();
  vi.useRealTimers();

  // A request without a mocked response may be swallowed by the component, fail the test instead.
  const unhandled = takeUnhandledRequests();
  if (unhandled.length > 0) {
    throw new Error(
      `Requests without mocked response: ${unhandled.join(", ")}`,
    );
  }
});
