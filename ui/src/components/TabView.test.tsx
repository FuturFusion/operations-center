import { fireEvent, render, screen } from "@testing-library/react";
import { expect, test, vi } from "vitest";
import TabView from "components/TabView";

test("renders base TabView", () => {
  const tabs = [
    {
      key: "tab1",
      title: "Tab 1",
      content: <span>Tab 1 content</span>,
    },
    {
      key: "tab2",
      title: "Tab 2",
      content: <span>Tab 2 content</span>,
    },
  ];

  const handleSelect = vi.fn();
  render(
    <TabView
      defaultTab="tab1"
      activeTab="tab1"
      tabs={tabs}
      onSelect={handleSelect}
    />,
  );

  expect(handleSelect).toHaveBeenCalledTimes(0);
  expect(screen.getByText("Tab 1 content")).toBeInTheDocument();
});

test("renders TabView with selected second tab", () => {
  const tabs = [
    {
      key: "tab1",
      title: "Tab 1",
      content: <span>Tab 1 content</span>,
    },
    {
      key: "tab2",
      title: "Tab 2",
      content: <span>Tab 2 content</span>,
    },
  ];

  const handleSelect = vi.fn();
  render(
    <TabView
      defaultTab="tab1"
      activeTab="tab2"
      tabs={tabs}
      onSelect={handleSelect}
    />,
  );

  expect(handleSelect).toHaveBeenCalledTimes(0);
  expect(screen.getByText("Tab 2 content")).toBeInTheDocument();
});

test("TabView select second tab", () => {
  const tabs = [
    {
      key: "tab1",
      title: "Tab 1",
      content: <span>Tab 1 content</span>,
    },
    {
      key: "tab2",
      title: "Tab 2",
      content: <span>Tab 2 content</span>,
    },
  ];

  const handleSelect = vi.fn();
  render(
    <TabView
      defaultTab="tab1"
      activeTab="tab1"
      tabs={tabs}
      onSelect={handleSelect}
    />,
  );

  fireEvent.click(screen.getByText("Tab 2"));

  expect(handleSelect).toHaveBeenCalledTimes(1);
});

test("renders only default tab content without selected tab", () => {
  const tabs = [
    {
      key: "tab1",
      title: "Tab 1",
      content: <span>Tab 1 content</span>,
    },
    {
      key: "tab2",
      title: "Tab 2",
      content: <span>Tab 2 content</span>,
    },
  ];

  const handleSelect = vi.fn();
  render(
    <TabView
      defaultTab="tab1"
      activeTab={undefined}
      tabs={tabs}
      onSelect={handleSelect}
    />,
  );

  expect(handleSelect).toHaveBeenCalledTimes(0);
  expect(screen.getByText("Tab 1 content")).toBeInTheDocument();
  expect(screen.queryByText("Tab 2 content")).not.toBeInTheDocument();
});

test("renders error message of failing tab content", () => {
  const FailingContent = () => {
    throw new Error("boom");
  };

  const tabs = [
    {
      key: "tab1",
      title: "Tab 1",
      content: <FailingContent />,
    },
    {
      key: "tab2",
      title: "Tab 2",
      content: <span>Tab 2 content</span>,
    },
  ];

  // React logs caught render errors. Keep the test output clean.
  const consoleError = vi.spyOn(console, "error").mockImplementation(() => {});

  const handleSelect = vi.fn();
  render(
    <TabView
      defaultTab="tab1"
      activeTab="tab1"
      tabs={tabs}
      onSelect={handleSelect}
    />,
  );

  expect(
    screen.getByText("Error while rendering Tab 1: boom"),
  ).toBeInTheDocument();
  expect(screen.getByText("Tab 2")).toBeInTheDocument();

  consoleError.mockRestore();
});
