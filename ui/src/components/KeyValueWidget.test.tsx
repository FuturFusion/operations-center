import { act, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import KeyValueWidget from "components/KeyValueWidget";

test("add new item to KeyValueWidget", async () => {
  const handleChange = vi.fn();

  render(<KeyValueWidget value={{}} onChange={handleChange} />);

  const keyInput = screen.getByPlaceholderText("New key");
  const valueInput = screen.getByPlaceholderText("New value");
  const addButton = screen.getByTitle("Add");

  await userEvent.type(keyInput, "foo");
  await userEvent.type(valueInput, "bar");

  await act(async () => {
    await fireEvent.click(addButton);
  });

  // Check if onChange was called with correct data
  expect(handleChange).toHaveBeenCalledTimes(1);
  expect(handleChange).toHaveBeenCalledWith({
    foo: "bar",
  });
});

test("remove item from KeyValueWidget", async () => {
  const handleChange = vi.fn();

  const val = {
    a: "b",
    c: "d",
  };

  render(<KeyValueWidget value={val} onChange={handleChange} />);

  const deleteButtons = screen.getAllByTitle("Delete");

  await act(async () => {
    await fireEvent.click(deleteButtons[0]);
  });

  // Check if onChange was called with correct data
  expect(handleChange).toHaveBeenCalledTimes(1);
  expect(handleChange).toHaveBeenCalledWith({
    c: "d",
  });
});
