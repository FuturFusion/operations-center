import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import ClusterTemplateVariablesWidget from "components/ClusterTemplateVariablesWidget";

const value = {
  region: { description: "The region", default: "eu" },
};

test("adds a variable", async () => {
  const user = userEvent.setup();
  const handleChange = vi.fn();
  render(
    <ClusterTemplateVariablesWidget value={value} onChange={handleChange} />,
  );

  const add = screen.getByTitle("Add");
  expect(add).toBeDisabled();

  // An existing name can not be added again.
  await user.type(screen.getByPlaceholderText("New variable"), "region");
  expect(add).toBeDisabled();

  await user.clear(screen.getByPlaceholderText("New variable"));
  await user.type(screen.getByPlaceholderText("New variable"), "size");
  await user.type(screen.getByPlaceholderText("Description"), "Disk size");
  await user.type(screen.getByPlaceholderText("Default value"), "10GiB");
  await user.click(add);

  expect(handleChange).toHaveBeenCalledExactlyOnceWith({
    ...value,
    size: { description: "Disk size", default: "10GiB" },
  });
  expect(screen.getByPlaceholderText("New variable")).toHaveValue("");
});

test("edits a variable", () => {
  const handleChange = vi.fn();
  render(
    <ClusterTemplateVariablesWidget value={value} onChange={handleChange} />,
  );

  fireEvent.change(screen.getByDisplayValue("eu"), { target: { value: "us" } });

  expect(handleChange).toHaveBeenCalledExactlyOnceWith({
    region: { description: "The region", default: "us" },
  });
});

test("deletes a variable", async () => {
  const user = userEvent.setup();
  const handleChange = vi.fn();
  render(
    <ClusterTemplateVariablesWidget value={value} onChange={handleChange} />,
  );

  await user.click(screen.getByTitle("Delete"));

  expect(handleChange).toHaveBeenCalledExactlyOnceWith({});
});
