import { FC } from "react";
import { Form } from "react-bootstrap";
import { FormikErrors, useFormik } from "formik";
import ChannelSelect from "components/ChannelSelect";
import KeyValueWidget from "components/KeyValueWidget";
import LoadingButton from "components/LoadingButton";
import { ServerPreRegisterFormValues } from "types/server";

interface Props {
  onSubmit: (values: ServerPreRegisterFormValues) => Promise<void>;
}

const validateForm = (
  values: ServerPreRegisterFormValues,
): FormikErrors<ServerPreRegisterFormValues> => {
  const errors: FormikErrors<ServerPreRegisterFormValues> = {};

  if (!values.name) {
    errors.name = "Name is required";
  }

  return errors;
};

const ServerPreRegisterForm: FC<Props> = ({ onSubmit }) => {
  const formik = useFormik<ServerPreRegisterFormValues>({
    initialValues: {
      name: "",
      description: "",
      properties: {},
      public_connection_url: "",
      channel: "stable",
      bmc_endpoint: "",
      bmc_certificate: "",
      bmc_auto_pin_certificate: false,
      bmc_username: "",
      bmc_password: "",
    },
    validate: validateForm,
    onSubmit: onSubmit,
  });

  return (
    <div className="form-container">
      <div>
        <Form noValidate>
          <fieldset className="border p-3 mb-3 rounded">
            <Form.Group className="mb-3" controlId="name">
              <Form.Label>Name</Form.Label>
              <Form.Control
                type="text"
                name="name"
                value={formik.values.name}
                onChange={formik.handleChange}
                onBlur={formik.handleBlur}
                isInvalid={!!formik.errors.name && formik.touched.name}
                disabled={formik.isSubmitting}
              />
              <Form.Control.Feedback type="invalid">
                {formik.errors.name}
              </Form.Control.Feedback>
            </Form.Group>
            <Form.Group className="mb-3" controlId="description">
              <Form.Label>Description</Form.Label>
              <Form.Control
                type="text"
                name="description"
                value={formik.values.description}
                onChange={formik.handleChange}
                onBlur={formik.handleBlur}
                disabled={formik.isSubmitting}
              />
            </Form.Group>
            <Form.Group className="mb-3" controlId="properties">
              <Form.Label>Properties</Form.Label>
              <KeyValueWidget
                value={formik.values.properties}
                onChange={(value) => formik.setFieldValue("properties", value)}
              />
            </Form.Group>
            <Form.Group className="mb-3" controlId="public_connection_url">
              <Form.Label>Connection URL</Form.Label>
              <Form.Control
                type="text"
                name="public_connection_url"
                value={formik.values.public_connection_url}
                onChange={formik.handleChange}
                onBlur={formik.handleBlur}
                disabled={formik.isSubmitting}
              />
            </Form.Group>
            <ChannelSelect
              value={formik.values.channel}
              onChange={(val) => formik.setFieldValue("channel", val)}
              disabled={formik.isSubmitting}
            />
          </fieldset>
          <fieldset className="border p-3 mb-3 rounded">
            <legend className="fs-5">BMC</legend>
            <Form.Group className="mb-3" controlId="bmc_endpoint">
              <Form.Label>URL</Form.Label>
              <Form.Control
                type="text"
                name="bmc_endpoint"
                value={formik.values.bmc_endpoint}
                onChange={formik.handleChange}
                onBlur={formik.handleBlur}
                disabled={formik.isSubmitting}
              />
            </Form.Group>
            <Form.Group className="mb-3" controlId="bmc_certificate">
              <Form.Label>Certificate</Form.Label>
              <Form.Control
                type="text"
                name="bmc_certificate"
                as="textarea"
                rows={6}
                value={formik.values.bmc_certificate}
                onChange={formik.handleChange}
                onBlur={formik.handleBlur}
                disabled={formik.isSubmitting}
              />
            </Form.Group>
            <Form.Group
              className="mb-3 d-flex align-items-center gap-2"
              controlId="bmc_auto_pin_certificate"
            >
              <Form.Check
                type="checkbox"
                name="bmc_auto_pin_certificate"
                checked={formik.values.bmc_auto_pin_certificate}
                onChange={formik.handleChange}
                onBlur={formik.handleBlur}
                disabled={
                  formik.isSubmitting || formik.values.bmc_certificate != ""
                }
              />
              <Form.Label className="me-2 mb-0">
                Automatically record current certificate
              </Form.Label>
            </Form.Group>
            <Form.Group className="mb-3" controlId="bmc_username">
              <Form.Label>Username</Form.Label>
              <Form.Control
                type="text"
                name="bmc_username"
                value={formik.values.bmc_username}
                onChange={formik.handleChange}
                onBlur={formik.handleBlur}
                disabled={formik.isSubmitting}
              />
            </Form.Group>
            <Form.Group className="mb-3" controlId="bmc_password">
              <Form.Label>Password</Form.Label>
              <Form.Control
                type="password"
                name="bmc_password"
                value={formik.values.bmc_password}
                onChange={formik.handleChange}
                onBlur={formik.handleBlur}
                disabled={formik.isSubmitting}
              />
            </Form.Group>
          </fieldset>
        </Form>
      </div>
      <div className="fixed-footer p-3">
        <LoadingButton
          isLoading={formik.isSubmitting}
          className="float-end"
          variant="success"
          onClick={() => formik.handleSubmit()}
        >
          Pre register
        </LoadingButton>
      </div>
    </div>
  );
};

export default ServerPreRegisterForm;
