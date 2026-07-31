import { FC, useState } from "react";
import { Button, Form } from "react-bootstrap";
import { useFormik } from "formik";
import ChannelSelect from "components/ChannelSelect";
import LoadingButton from "components/LoadingButton";
import KeyValueWidget from "components/KeyValueWidget";
import { APIResponse } from "types/response";
import { Server, ServerFormValues } from "types/server";
import YAML from "yaml";

interface Props {
  server?: Server;
  systemNetwork?: object;
  systemStorage?: object;
  onRename: (newName: string) => void;
  onSubmit: (
    values: ServerFormValues,
    section: string,
  ) => Promise<APIResponse<null> | void>;
}

const ServerForm: FC<Props> = ({
  server,
  systemNetwork,
  systemStorage,
  onRename,
  onSubmit,
}) => {
  const [submitting, setSubmitting] = useState<Record<string, boolean>>({});

  const formikInitialValues = {
    name: server?.name || "",
    public_connection_url: server?.public_connection_url || "",
    channel: server?.channel || "",
    description: server?.description || "",
    properties: server?.properties || {},
    network_configuration: YAML.stringify(systemNetwork, null, 2),
    storage_configuration: YAML.stringify(systemStorage, null, 2),
    bmc_endpoint: server?.bmc_config?.endpoint || "",
    bmc_certificate: server?.bmc_config?.certificate || "",
    bmc_auto_pin_certificate: server?.bmc_config?.auto_pin_certificate || false,
    bmc_username: server?.bmc_config?.username || "",
    bmc_password: server?.bmc_config?.password || "",
  };

  const formik = useFormik({
    initialValues: formikInitialValues,
    enableReinitialize: true,
    onSubmit: () => {},
  });

  const submitForm = async (values: ServerFormValues, section: string) => {
    setSubmitting((prev) => ({
      ...prev,
      [section]: true,
    }));
    await onSubmit(values, section);
    setSubmitting((prev) => ({
      ...prev,
      [section]: false,
    }));
  };

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
              <Button
                className="mt-3 float-end"
                variant="success"
                onClick={() => onRename(formik.values.name)}
              >
                Rename
              </Button>
            </Form.Group>
          </fieldset>
          <fieldset className="border p-3 mb-3 rounded">
            <Form.Group className="mb-3" controlId="description">
              <Form.Label>Description</Form.Label>
              <Form.Control
                type="text"
                name="description"
                value={formik.values.description}
                onChange={formik.handleChange}
                onBlur={formik.handleBlur}
                disabled={submitting["configuration"]}
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
                disabled={submitting["configuration"]}
              />
            </Form.Group>
            <ChannelSelect
              value={formik.values.channel}
              onChange={(val) => formik.setFieldValue("channel", val)}
              disabled={submitting["configuration"]}
            />
            <LoadingButton
              isLoading={submitting["configuration"]}
              className="mt-3 float-end"
              variant="success"
              onClick={() => submitForm(formik.values, "configuration")}
            >
              Submit
            </LoadingButton>
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
                disabled={submitting["bmc"]}
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
                disabled={submitting["bmc"]}
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
                  submitting["bmc"] || formik.values.bmc_certificate != ""
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
                disabled={submitting["bmc"]}
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
                disabled={submitting["bmc"]}
              />
            </Form.Group>
            <LoadingButton
              isLoading={submitting["bmc"]}
              className="mt-3 float-end"
              variant="success"
              onClick={() => submitForm(formik.values, "bmc")}
            >
              Submit
            </LoadingButton>
          </fieldset>
          <fieldset className="border p-3 mb-3 rounded">
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
                disabled={submitting["network"]}
                className="editor"
              />
              <LoadingButton
                isLoading={submitting["network"]}
                className="mt-3 float-end"
                variant="success"
                onClick={() => submitForm(formik.values, "network")}
              >
                Submit
              </LoadingButton>
            </Form.Group>
          </fieldset>
          <fieldset className="border p-3 mb-3 rounded">
            <Form.Group className="mb-3" controlId="storage_configuration">
              <Form.Label>Storage configuration</Form.Label>
              <Form.Control
                type="text"
                name="storage_configuration"
                as="textarea"
                rows={10}
                value={formik.values.storage_configuration}
                onChange={formik.handleChange}
                onBlur={formik.handleBlur}
                disabled={submitting["storage"]}
                className="editor"
              />
              <LoadingButton
                isLoading={submitting["storage"]}
                className="mt-3 float-end"
                variant="success"
                onClick={() => submitForm(formik.values, "storage")}
              >
                Submit
              </LoadingButton>
            </Form.Group>
          </fieldset>
        </Form>
      </div>
      <div className="fixed-footer p-3"></div>
    </div>
  );
};

export default ServerForm;
