import { FC } from "react";
import { Button, Form } from "react-bootstrap";
import { useFormik } from "formik";
import { Server, ServerFormValues } from "types/server";
import YAML from "yaml";

interface Props {
  server?: Server;
  systemNetwork?: object;
  onRename: (newName: string) => void;
  onSubmit: (values: ServerFormValues) => void;
}

const ServerForm: FC<Props> = ({
  server,
  systemNetwork,
  onRename,
  onSubmit,
}) => {
  const formikInitialValues = {
    name: server?.name || "",
    public_connection_url: server?.public_connection_url || "",
    network_configuration: YAML.stringify(systemNetwork, null, 2),
  };

  const formik = useFormik({
    initialValues: formikInitialValues,
    enableReinitialize: true,
    onSubmit: (values: ServerFormValues) => {
      onSubmit(values);
    },
  });

  return (
    <div className="form-container">
      <div>
        <Form noValidate>
          <Form.Group className="mb-3" controlId="name">
            <Form.Label>Name</Form.Label>
            <div className="d-flex align-items-center gap-2">
              <Form.Control
                type="text"
                name="name"
                value={formik.values.name}
                onChange={formik.handleChange}
                onBlur={formik.handleBlur}
                isInvalid={!!formik.errors.name && formik.touched.name}
              />
              <Form.Control.Feedback type="invalid">
                {formik.errors.name}
              </Form.Control.Feedback>
              <Button
                className="float-end"
                variant="success"
                onClick={() => onRename(formik.values.name)}
              >
                Rename
              </Button>
            </div>
          </Form.Group>
          <Form.Group className="mb-3" controlId="public_connection_url">
            <Form.Label>Connection URL</Form.Label>
            <Form.Control
              type="text"
              name="public_connection_url"
              value={formik.values.public_connection_url}
              onChange={formik.handleChange}
              onBlur={formik.handleBlur}
            />
          </Form.Group>
          <Form.Group className="mb-3" controlId="network_configuration">
            <Form.Label>Network configuration</Form.Label>
            <Form.Control
              type="text"
              name="network_configuration"
              as="textarea"
              rows={10}
              value={formik.values.network_configuration}
              onChange={formik.handleChange}
              onBlur={formik.handleBlur}
              className="editor"
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

export default ServerForm;
