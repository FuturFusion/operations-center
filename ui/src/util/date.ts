import { format, parseISO } from "date-fns";

export const formatDate = (input: string): string => {
  if (input == "") {
    return "";
  }

  return format(parseISO(input), "yyyy-MM-dd HH:mm");
};
