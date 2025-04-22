import { Image } from "types/image";
import { processResponse } from "util/response";

export const fetchImages = (): Promise<Image[]> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/inventory/images?recursion=1`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};
