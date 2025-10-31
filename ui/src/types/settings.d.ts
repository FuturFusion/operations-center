export interface SystemNetwork {
  address: string;
  rest_server_address: string;
}

export interface SystemSecurity {
  trusted_tls_client_cert_fingerprints: string[];
  oidc: SystemSecurityOIDC;
  openfga: SystemSecurityOpenFGA;
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

export interface SystemCertificate {
  certificate: string;
  key: string;
}

export interface SystemUpdates {
  source: string;
  signature_verification_root_ca: string;
  filter_expression: string;
  file_filter_expression: string;
}
