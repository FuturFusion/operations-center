import { Settings } from "types/server";

export const fetchSettings = (): Promise<Settings> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0`)
      .then((response) => response.json())
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};
