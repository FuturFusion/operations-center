import { Profile } from "types/profile";
import { processResponse } from "util/response";

export const fetchProfiles = (): Promise<Profile[]> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/inventory/profiles?recursion=1`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};
