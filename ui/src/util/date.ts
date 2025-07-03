import { format, parseISO } from "date-fns";

export const formatDate = (input: string): string => {
  if (input === "" || input === "0001-01-01T00:00:00Z") {
    return "";
  }

  return format(parseISO(input), "yyyy-MM-dd HH:mm");
};
