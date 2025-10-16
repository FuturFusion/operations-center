import { FC } from "react";
import { Form } from "react-bootstrap";
import { FormikErrors, useFormik } from "formik";
import { useNotification } from "context/notificationContext";
import { useServers } from "context/useServers";
import { useClusterTemplates } from "context/useClusterTemplates";
import LoadingButton from "components/LoadingButton";
import { ClusterFormValues, ClusterPost } from "types/cluster";
import { ClusterTemplateVariable } from "types/cluster_template";
import { ServerType } from "util/server";
import YAML from "yaml";

enum CreateType {
  Manual = "manual",
  Template = "template",
}

interface Props {
  mode: string;
  onSubmit: (values: ClusterPost) => void;
}

const ClusterCreateForm: FC<Props> = ({ mode, onSubmit }) => {
  const { data: servers } = useServers("Cluster==nil");
  const { data: templates } = useClusterTemplates();
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

    if (mode == CreateType.Template && values.template == "") {
      errors.template = "Template is required";
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
    variables: "",
    template: "",
  };

  const applyVariables = (
    template: string,
    variables: Record<string, ClusterTemplateVariable>,
    variableValues: Record<string, string>,
  ): string => {
    Object.entries(variables).forEach(([key, clusterVar]) => {
      if (!(key in variableValues)) {
        if (clusterVar.default == "") {
          throw new Error(
            `No value provided for variable ${key}, which is required, since it has no default value defined`,
          );
        }
        variableValues[key] = clusterVar.default;
      }
    });

    let result = template;
    Object.entries(variableValues).forEach(([name, value]) => {
      const variableName = "@" + name + "@";
      const regex = new RegExp(variableName, "g");

      result = result.replace(regex, value);
    });

    return result;
  };

  const formik = useFormik({
    initialValues: formikInitialValues,
    validate: validateForm,
    enableReinitialize: true,
    onSubmit: (values: ClusterFormValues, { setSubmitting }) => {
      let servicesConfig = "";
      let applicationSeedConfig = "";

      if (mode == CreateType.Manual) {
        try {
          servicesConfig = YAML.parse(values.services_config);
          applicationSeedConfig = YAML.parse(values.application_seed_config);
        } catch (error) {
          notify.error(`Error during YAML value parsing: ${error}`);
          setSubmitting(false);
          return;
        }
      } else {
        const template = templates?.find((t) => t.name === values.template);

        let variablesValues = {};
        try {
          variablesValues = YAML.parse(values.variables);
        } catch (error) {
          notify.error(`Error during variables parsing: ${error}`);
          setSubmitting(false);
          return;
        }

        try {
          servicesConfig = YAML.parse(
            applyVariables(
              template?.service_config_template || "",
              template?.variables || {},
              variablesValues || {},
            ),
          );
          applicationSeedConfig = YAML.parse(
            applyVariables(
              template?.application_config_template || "",
              template?.variables || {},
              variablesValues || {},
            ),
          );
        } catch (error) {
          notify.error(`Error during YAML value parsing: ${error}`);
          setSubmitting(false);
          return;
        }
      }

      return onSubmit({
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
          {mode == CreateType.Manual && (
            <>
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
            </>
          )}
          {mode == CreateType.Template && (
            <>
              <Form.Group className="mb-4" controlId="templates">
                <Form.Label>Templates</Form.Label>
                <Form.Select
                  name="template"
                  value={formik.values.template}
                  onChange={formik.handleChange}
                  isInvalid={
                    !!formik.errors.template && formik.touched.template
                  }
                  disabled={formik.isSubmitting}
                >
                  <option key="" value=""></option>
                  {templates?.map((template) => (
                    <option key={template.name} value={template.name}>
                      {template.name}
                    </option>
                  ))}
                </Form.Select>
                <Form.Control.Feedback type="invalid">
                  {formik.errors.template}
                </Form.Control.Feedback>
              </Form.Group>
              <Form.Group className="mb-4" controlId="variables">
                <Form.Label>Variables</Form.Label>
                <Form.Control
                  type="text"
                  as="textarea"
                  rows={6}
                  name="variables"
                  value={formik.values.variables}
                  onChange={formik.handleChange}
                  onBlur={formik.handleBlur}
                  disabled={formik.isSubmitting}
                />
              </Form.Group>
            </>
          )}
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

export default ClusterCreateForm;
