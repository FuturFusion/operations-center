import { FC } from "react";
import { Button, Form } from "react-bootstrap";
import { useFormik } from "formik";
import { SystemSecurity } from "types/settings";
import { ACMEChallengeValues } from "util/settings";

interface Props {
  security?: SystemSecurity;
  onSubmit: (values: SystemSecurity) => void;
}

const SystemCertForm: FC<Props> = ({ security, onSubmit }) => {
  const formikInitialValues: SystemSecurity = {
    trusted_tls_client_cert_fingerprints:
      security?.trusted_tls_client_cert_fingerprints ?? [],
    oidc: {
      issuer: security?.oidc.issuer ?? "",
      client_id: security?.oidc.client_id ?? "",
      scopes: security?.oidc.scopes ?? "",
      audience: security?.oidc.audience ?? "",
      claim: security?.oidc.claim ?? "",
    },
    openfga: {
      api_token: security?.openfga.api_token ?? "",
      api_url: security?.openfga.api_url ?? "",
      store_id: security?.openfga.store_id ?? "",
    },
    acme: {
      agree_tos: security?.acme.agree_tos ?? false,
      ca_url: security?.acme.ca_url ?? "",
      challenge: security?.acme.challenge ?? "HTTP-01",
      email: security?.acme.email ?? "",
      domain: security?.acme.domain ?? "",
      http_challenge_address: security?.acme.http_challenge_address ?? "",
      provider: security?.acme.provider ?? "",
      provider_environment: security?.acme.provider_environment ?? [],
      provider_resolvers: security?.acme.provider_resolvers ?? [],
    },
  };

  const formik = useFormik({
    initialValues: formikInitialValues,
    onSubmit: (values: SystemSecurity) => {
      onSubmit({
        ...values,
        trusted_tls_client_cert_fingerprints:
          values.trusted_tls_client_cert_fingerprints.filter(
            (s) => s.trim() !== "",
          ),
        acme: {
          ...values.acme,
          provider_environment: values.acme.provider_environment.filter(
            (s) => s.trim() !== "",
          ),
        },
      });
    },
  });

  return (
    <div className="form-container">
      <div>
        <Form noValidate>
          <fieldset className="border p-3 mb-3 rounded">
            <legend className="fs-5">General</legend>
            <Form.Group className="mb-3" controlId="trusted_tls_cert">
              <Form.Label>Trusted TLS certification fingerprints</Form.Label>
              <Form.Control
                type="text"
                as="textarea"
                rows={10}
                name="trusted_tls_client_cert_fingerprints"
                value={formik.values.trusted_tls_client_cert_fingerprints.join(
                  "\n",
                )}
                onChange={(e) => {
                  const lines = e.target.value.split("\n");
                  formik.setFieldValue(
                    "trusted_tls_client_cert_fingerprints",
                    lines,
                  );
                }}
                onBlur={formik.handleBlur}
              />
            </Form.Group>
          </fieldset>
          <fieldset className="border p-3 mb-3 rounded">
            <legend className="fs-5">OIDC</legend>
            <Form.Group className="mb-3" controlId="issuer">
              <Form.Label>Issuer</Form.Label>
              <Form.Control
                type="text"
                name="oidc.issuer"
                value={formik.values.oidc.issuer}
                onChange={formik.handleChange}
                onBlur={formik.handleBlur}
              />
            </Form.Group>
            <Form.Group className="mb-3" controlId="client_id">
              <Form.Label>Client ID</Form.Label>
              <Form.Control
                type="text"
                name="oidc.client_id"
                value={formik.values.oidc.client_id}
                onChange={formik.handleChange}
                onBlur={formik.handleBlur}
              />
            </Form.Group>
            <Form.Group className="mb-3" controlId="scopes">
              <Form.Label>Scopes</Form.Label>
              <Form.Control
                type="text"
                name="oidc.scopes"
                value={formik.values.oidc.scopes}
                onChange={formik.handleChange}
                onBlur={formik.handleBlur}
              />
            </Form.Group>
            <Form.Group className="mb-3" controlId="audience">
              <Form.Label>Audience</Form.Label>
              <Form.Control
                type="text"
                name="oidc.audience"
                value={formik.values.oidc.audience}
                onChange={formik.handleChange}
                onBlur={formik.handleBlur}
              />
            </Form.Group>
            <Form.Group className="mb-3" controlId="Claim">
              <Form.Label>Claim</Form.Label>
              <Form.Control
                type="text"
                name="oidc.claim"
                value={formik.values.oidc.claim}
                onChange={formik.handleChange}
                onBlur={formik.handleBlur}
              />
            </Form.Group>
          </fieldset>
          <fieldset className="border p-3 mb-3 rounded">
            <legend className="fs-5">OpenFGA</legend>
            <Form.Group className="mb-3" controlId="api_token">
              <Form.Label>API token</Form.Label>
              <Form.Control
                type="text"
                name="openfga.api_token"
                value={formik.values.openfga.api_token}
                onChange={formik.handleChange}
                onBlur={formik.handleBlur}
              />
            </Form.Group>
            <Form.Group className="mb-3" controlId="api_url">
              <Form.Label>API url</Form.Label>
              <Form.Control
                type="text"
                name="openfga.api_url"
                value={formik.values.openfga.api_url}
                onChange={formik.handleChange}
                onBlur={formik.handleBlur}
              />
            </Form.Group>
            <Form.Group className="mb-3" controlId="store_id">
              <Form.Label>Store ID</Form.Label>
              <Form.Control
                type="text"
                name="openfga.store_id"
                value={formik.values.openfga.store_id}
                onChange={formik.handleChange}
                onBlur={formik.handleBlur}
              />
            </Form.Group>
          </fieldset>
          <fieldset className="border p-3 mb-3 rounded">
            <legend className="fs-5">ACME</legend>
            <Form.Group className="mb-3" controlId="agree_tos">
              <Form.Label>Agree to ACME terms of service</Form.Label>
              <Form.Select
                name="acme.agree_tos"
                value={formik.values.acme.agree_tos ? "true" : "false"}
                onChange={(e) =>
                  formik.setFieldValue(
                    "acme.agree_tos",
                    e.target.value === "true",
                  )
                }
                onBlur={formik.handleBlur}
              >
                <option value="false">no</option>
                <option value="true">yes</option>
              </Form.Select>
            </Form.Group>
            <Form.Group className="mb-3" controlId="ca_url">
              <Form.Label>CA URL</Form.Label>
              <Form.Control
                type="text"
                name="acme.ca_url"
                value={formik.values.acme.ca_url}
                onChange={formik.handleChange}
                onBlur={formik.handleBlur}
              />
            </Form.Group>
            <Form.Group className="mb-3" controlId="challenge">
              <Form.Label>Challenge</Form.Label>
              <Form.Select
                name="acme.challenge"
                value={formik.values.acme.challenge}
                onChange={formik.handleChange}
                onBlur={formik.handleBlur}
              >
                {ACMEChallengeValues.map((option) => (
                  <option key={option} value={option}>
                    {option}
                  </option>
                ))}
              </Form.Select>
            </Form.Group>
            <Form.Group className="mb-3" controlId="domain">
              <Form.Label>Domain</Form.Label>
              <Form.Control
                type="text"
                name="acme.domain"
                value={formik.values.acme.domain}
                onChange={formik.handleChange}
                onBlur={formik.handleBlur}
              />
            </Form.Group>
            <Form.Group className="mb-3" controlId="email">
              <Form.Label>Email</Form.Label>
              <Form.Control
                type="text"
                name="acme.email"
                value={formik.values.acme.email}
                onChange={formik.handleChange}
                onBlur={formik.handleBlur}
              />
            </Form.Group>
            <Form.Group className="mb-3" controlId="http_challenge_address">
              <Form.Label>Address</Form.Label>
              <Form.Control
                type="text"
                name="acme.http_challenge_address"
                value={formik.values.acme.http_challenge_address}
                onChange={formik.handleChange}
                onBlur={formik.handleBlur}
              />
            </Form.Group>
            <Form.Group className="mb-3" controlId="provider">
              <Form.Label>Provider</Form.Label>
              <Form.Control
                type="text"
                name="acme.provider"
                value={formik.values.acme.provider}
                onChange={formik.handleChange}
                onBlur={formik.handleBlur}
              />
            </Form.Group>
            <Form.Group className="mb-3" controlId="provider_environment">
              <Form.Label>Environment variables</Form.Label>
              <Form.Control
                type="text"
                as="textarea"
                rows={10}
                name="acme.provider_environment"
                value={formik.values.acme.provider_environment.join("\n")}
                onChange={(e) => {
                  const lines = e.target.value.split("\n");
                  formik.setFieldValue("acme.provider_environment", lines);
                }}
                onBlur={formik.handleBlur}
              />
            </Form.Group>
            <Form.Group className="mb-3" controlId="provider_resolvers">
              <Form.Label>DNS resolvers</Form.Label>
              <Form.Control
                type="text"
                as="textarea"
                rows={10}
                name="acme.provider_resolvers"
                value={formik.values.acme.provider_resolvers.join("\n")}
                onChange={(e) => {
                  const lines = e.target.value.split("\n");
                  formik.setFieldValue("acme.provider_resolvers", lines);
                }}
                onBlur={formik.handleBlur}
              />
            </Form.Group>
          </fieldset>
        </Form>
      </div>
      <div className="fixed-footer p-3">
        <Button
          className="float-end"
          variant="success"
          onClick={() => formik.handleSubmit()}
        >
          Submit
        </Button>
      </div>
    </div>
  );
};

export default SystemCertForm;
