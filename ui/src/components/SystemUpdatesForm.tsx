import { FC } from "react";
import { Button, Form } from "react-bootstrap";
import { useFormik } from "formik";
import ChannelSelect from "components/ChannelSelect";
import { SystemUpdates } from "types/settings";

interface Props {
  updates?: SystemUpdates;
  onSubmit: (values: SystemUpdates) => void;
}

const SystemUpdatesForm: FC<Props> = ({ updates, onSubmit }) => {
  const formikInitialValues: SystemUpdates = {
    source: updates?.source ?? "",
    signature_verification_root_ca:
      updates?.signature_verification_root_ca ?? "",
    filter_expression: updates?.filter_expression ?? "",
    file_filter_expression: updates?.file_filter_expression ?? "",
    server_default_channel: updates?.server_default_channel ?? "",
    updates_default_channel: updates?.updates_default_channel ?? "",
  };

  const formik = useFormik({
    initialValues: formikInitialValues,
    enableReinitialize: true,
    onSubmit: (values: SystemUpdates) => {
      onSubmit(values);
    },
  });

  return (
    <div className="form-container">
      <div>
        <Form noValidate>
          <Form.Group className="mb-3" controlId="source">
            <Form.Label>Image server URL</Form.Label>
            <Form.Control
              type="text"
              name="source"
              value={formik.values.source}
              onChange={formik.handleChange}
              onBlur={formik.handleBlur}
            />
          </Form.Group>
          <Form.Group
            className="mb-3"
            controlId="signature_verification_root_ca"
          >
            <Form.Label>Image metadata signing CA</Form.Label>
            <Form.Control
              type="text"
              as="textarea"
              rows={10}
              name="signature_verification_root_ca"
              value={formik.values.signature_verification_root_ca}
              onChange={formik.handleChange}
              onBlur={formik.handleBlur}
            />
          </Form.Group>
          <Form.Group className="mb-3" controlId="filter_expression">
            <Form.Label>Filter expression</Form.Label>
            <Form.Control
              type="text"
              name="filter_expression"
              value={formik.values.filter_expression}
              onChange={formik.handleChange}
              onBlur={formik.handleBlur}
            />
          </Form.Group>
          <Form.Group className="mb-3" controlId="file_filter_expression">
            <Form.Label>File filter expression</Form.Label>
            <Form.Control
              type="text"
              name="file_filter_expression"
              value={formik.values.file_filter_expression}
              onChange={formik.handleChange}
              onBlur={formik.handleBlur}
            />
          </Form.Group>
          <ChannelSelect
            formClasses="mb-3"
            label="Default update channel for new servers"
            value={formik.values.server_default_channel}
            onChange={(val) =>
              formik.setFieldValue("server_default_channel", val)
            }
          />
          <ChannelSelect
            formClasses="mb-3"
            label="Default update channel for new images"
            value={formik.values.updates_default_channel}
            onChange={(val) =>
              formik.setFieldValue("updates_default_channel", val)
            }
          />
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

export default SystemUpdatesForm;
