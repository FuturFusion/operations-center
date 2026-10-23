import { APIResponse, ErrorMetadata } from "types/response";
import { SupportBundlePost } from "types/support_bundle";
import { APIError } from "util/response";

export const createSupportBundle = (
  request: SupportBundlePost,
): Promise<{ url: string; filename: string }> => {
  return new Promise((resolve, reject) => {
    fetch(`/1.0/support-bundle`, {
      method: "POST",
      body: JSON.stringify(request),
      headers: {
        "Content-Type": "application/json",
      },
    })
      .then(async (response) => {
        if (!response.ok) {
          const error =
            (await response.json()) as APIResponse<ErrorMetadata | null>;
          throw new APIError(error);
        }

        const disposition = response.headers.get("Content-Disposition") ?? "";
        const match = /filename="([^"]+)"/.exec(disposition);
        const filename = match ? match[1] : "support-bundle.zip";

        return { blob: await response.blob(), filename };
      })
      .then(({ blob, filename }) =>
        resolve({ url: URL.createObjectURL(blob), filename }),
      )
      .catch(reject);
  });
};
