import { FC } from "react";
import { Form } from "react-bootstrap";
import { FormikErrors, useFormik } from "formik";
import { useNotification } from "context/notificationContext";
import { useServers } from "context/useServers";
import LoadingButton from "components/LoadingButton";
import { ClusterFormValues } from "types/cluster";
import { ServerType } from "util/server";
import YAML from "yaml";

interface Props {
  onSubmit: (values: ClusterFormValues) => void;
}

const ClusterCreateManualForm: FC<Props> = ({ onSubmit }) => {
  const { data: servers } = useServers("Cluster==nil");
  const { notify } = useNotification();

  const validateForm = (
    values: ClusterFormValues,
  ): FormikErrors<ClusterFormValues> => {
    const errors: FormikErrors<ClusterFormValues> = {};

    if (!values.name) {
      errors.name = "Name is required";
    }

    if (values.server_names.length <= 0) {
      errors.server_names = "List of server names can not be empty";
    }

    return errors;
  };

  const formikInitialValues: ClusterFormValues = {
    name: "",
    connection_url: "",
    server_names: [],
    server_type: Object.values(ServerType)[0],
    services_config: "",
    application_seed_config: "",
  };

  const formik = useFormik({
    initialValues: formikInitialValues,
    validate: validateForm,
    enableReinitialize: true,
    onSubmit: (values: ClusterFormValues) => {
      let servicesConfig = "";
      let applicationSeedConfig = "";

      try {
        servicesConfig = YAML.parse(values.services_config);
        applicationSeedConfig = YAML.parse(values.application_seed_config);
      } catch (error) {
        notify.error(`Error during YAML value parsing: ${error}`);
        return;
      }

      onSubmit({
        name: values.name,
        connection_url: values.connection_url,
        server_names: values.server_names,
        server_type: values.server_type,
        services_config: servicesConfig,
        application_seed_config: applicationSeedConfig,
      });
    },
  });

  return (
    <div className="form-container">
      <div>
        <Form noValidate>
          <Form.Group className="mb-4" controlId="name">
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
          <Form.Group className="mb-4" controlId="connectionURL">
            <Form.Label>Connection URL</Form.Label>
            <Form.Control
              type="text"
              name="connection_url"
              value={formik.values.connection_url}
              onChange={formik.handleChange}
              onBlur={formik.handleBlur}
              isInvalid={
                !!formik.errors.connection_url && formik.touched.connection_url
              }
              disabled={formik.isSubmitting}
            />
            <Form.Control.Feedback type="invalid">
              {formik.errors.connection_url}
            </Form.Control.Feedback>
          </Form.Group>
          <Form.Group className="mb-4" controlId="serverNames">
            <Form.Label>Servers</Form.Label>
            <Form.Select
              multiple
              value={formik.values.server_names}
              onChange={(e) => {
                const selected = Array.from(
                  e.target.selectedOptions,
                  (option) => option.value,
                );
                formik.setFieldValue("server_names", selected);
              }}
              isInvalid={
                !!formik.errors.server_names && formik.touched.server_names
              }
              disabled={formik.isSubmitting}
            >
              {servers?.map((server) => (
                <option key={server.name} value={server.name}>
                  {server.name}
                </option>
              ))}
            </Form.Select>
            <Form.Control.Feedback type="invalid">
              {formik.errors.server_names}
            </Form.Control.Feedback>
          </Form.Group>
          <Form.Group className="mb-4" controlId="serverType">
            <Form.Label>Server type</Form.Label>
            <Form.Select
              value={formik.values.server_type}
              onChange={(e) => {
                formik.setFieldValue("server_type", e.target.value);
              }}
              disabled={formik.isSubmitting}
            >
              {Object.values(ServerType).map((type) => (
                <option key={status} value={type}>
                  {type}
                </option>
              ))}
            </Form.Select>
          </Form.Group>
          <Form.Group className="mb-4" controlId="servicesConfig">
            <Form.Label>Services config</Form.Label>
            <Form.Control
              type="text"
              as="textarea"
              rows={6}
              name="services_config"
              value={formik.values.services_config}
              onChange={formik.handleChange}
              onBlur={formik.handleBlur}
              disabled={formik.isSubmitting}
            />
          </Form.Group>
          <Form.Group className="mb-4" controlId="applicationSeedConfig">
            <Form.Label>Application seed config</Form.Label>
            <Form.Control
              type="text"
              as="textarea"
              rows={6}
              name="application_seed_config"
              value={formik.values.application_seed_config}
              onChange={formik.handleChange}
              onBlur={formik.handleBlur}
              disabled={formik.isSubmitting}
            />
          </Form.Group>
        </Form>
      </div>
      <div className="fixed-footer p-3">
        <LoadingButton
          isLoading={formik.isSubmitting}
          className="float-end"
          variant="success"
          onClick={() => formik.handleSubmit()}
        >
          Create
        </LoadingButton>
      </div>
    </div>
  );
};

export default ClusterCreateManualForm;
