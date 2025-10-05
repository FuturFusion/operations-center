import { FC } from "react";
import { Form } from "react-bootstrap";
import { FormikProps } from "formik/dist/types";
import { ClusterCertFormValues } from "types/cluster";

interface Props {
  formik: FormikProps<ClusterCertFormValues>;
}

const ClusterCertForm: FC<Props> = ({ formik }) => {
  return (
    <div>
      <Form noValidate>
        <Form.Group className="mb-4" controlId="cert">
          <Form.Label>Cluster certificate</Form.Label>
          <Form.Control
            type="text"
            as="textarea"
            rows={6}
            name="cluster_certificate"
            value={formik.values.cluster_certificate}
            onChange={formik.handleChange}
            onBlur={formik.handleBlur}
          />
        </Form.Group>
        <Form.Group className="mb-4" controlId="cert_key">
          <Form.Label>Cluster certificate key</Form.Label>
          <Form.Control
            type="text"
            as="textarea"
            rows={6}
            name="cluster_certificate_key"
            value={formik.values.cluster_certificate_key}
            onChange={formik.handleChange}
            onBlur={formik.handleBlur}
          />
        </Form.Group>
      </Form>
    </div>
  );
};

export default ClusterCertForm;
