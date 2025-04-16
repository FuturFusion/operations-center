import { Project } from "types/project";
import { processResponse } from "util/response";

export const fetchProjects = (): Promise<Project[]> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/inventory/projects?recursion=1`)
      .then(processResponse)
      .then((data) => resolve(data.metadata))
      .catch(reject);
  });
};
