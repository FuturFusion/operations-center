import { FC } from "react";
import { Button, Form } from "react-bootstrap";
import { useFormik } from "formik";
import { SystemSecurity } from "types/settings";

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
