export interface Token {
  uuid: string;
  description: string;
  expire_at: string;
  uses_remaining: number;
}

export interface TokenFormValues {
  description: string;
  expire_at: string;
  uses_remaining: number;
}
