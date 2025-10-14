import { APIResponse } from "types/response";

export const processResponse = async (response: Response) => {
  if (!response.ok) {
    const error = (await response.json()) as APIResponse<null>;
    throw new Error(`API error (${error.error_code}): ${error.error}`);
  }
  return response.json();
};
