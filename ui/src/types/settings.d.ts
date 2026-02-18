import { ACMEChallengeValues } from "util/settings";

export interface SystemNetwork {
  address: string;
  rest_server_address: string;
}

export interface SystemSecurity {
  trusted_tls_client_cert_fingerprints: string[];
  trusted_https_proxies: string[];
  oidc: SystemSecurityOIDC;
  openfga: SystemSecurityOpenFGA;
  acme: SystemSecurityACME;
}

export interface SystemSecurityOIDC {
  issuer: string;
  client_id: string;
  scopes: string;
  audience: string;
  claim: string;
}

export interface SystemSecurityOpenFGA {
  api_token: string;
  api_url: string;
  store_id: string;
}

export type ACMEChallengeType = (typeof ACMEChallengeValues)[number];

export interface SystemSecurityACME {
  agree_tos: boolean;
  ca_url: string;
  challenge: ACMEChallengeType;
  domain: string;
  email: string;
  http_challenge_address: string;
  provider: string;
  provider_environment: string[];
  provider_resolvers: string[];
}

export interface SystemSettings {
  log_level: string;
}

export interface SystemCertificate {
  certificate: string;
  key: string;
}

export interface SystemUpdates {
  source: string;
  signature_verification_root_ca: string;
  filter_expression: string;
  file_filter_expression: string;
  server_default_channel: string;
  updates_default_channel: string;
}
