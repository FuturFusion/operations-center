export interface APIResponse<T> {
  error: string;
  error_code: number;
  metadata: T;
  operation: string;
  status: string;
  status_code: number;
  type: "sync" | "error";
}

export interface ErrorMetadata {
  reason: string;
  hint?: string;
  details?: Record<string, string>;
  request_id?: string;
}

export interface APIImageURL {
  image: string;
}
