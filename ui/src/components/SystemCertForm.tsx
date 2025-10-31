import { FC } from "react";
import { Button, Form } from "react-bootstrap";
import { useFormik } from "formik";
import { SystemCertificate } from "types/settings";

interface Props {
  onSubmit: (values: SystemCertificate) => void;
}

const SystemCertForm: FC<Props> = ({ onSubmit }) => {
  const formikInitialValues: SystemCertificate = {
    certificate: "",
    key: "",
  };

  const formik = useFormik({
    initialValues: formikInitialValues,
    onSubmit: (values: SystemCertificate) => {
      onSubmit(values);
    },
  });

  return (
    <div className="form-container">
      <div>
        <Form noValidate>
          <Form.Group className="mb-3" controlId="certificate">
            <Form.Label>Certificate</Form.Label>
            <Form.Control
              type="text"
              as="textarea"
              rows={10}
              name="certificate"
              value={formik.values.certificate}
              onChange={formik.handleChange}
              onBlur={formik.handleBlur}
            />
          </Form.Group>
          <Form.Group className="mb-3" controlId="key">
            <Form.Label>Key</Form.Label>
            <Form.Control
              type="text"
              as="textarea"
              rows={10}
              name="key"
              value={formik.values.key}
              onChange={formik.handleChange}
              onBlur={formik.handleBlur}
            />
          </Form.Group>
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
