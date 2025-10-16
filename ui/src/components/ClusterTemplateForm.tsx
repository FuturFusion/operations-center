import { FC } from "react";
import { Button, Form } from "react-bootstrap";
import { useFormik } from "formik";
import ClusterTemplateVariablesWidget from "components/ClusterTemplateVariablesWidget";
import {
  ClusterTemplate,
  ClusterTemplateFormValues,
} from "types/cluster_template";

interface Props {
  clusterTemplate?: ClusterTemplate;
  onRename?: (newName: string) => void;
  onSubmit: (values: ClusterTemplateFormValues) => void;
}

const ClusterTemplateForm: FC<Props> = ({
  clusterTemplate,
  onRename,
  onSubmit,
}) => {
  let formikInitialValues: ClusterTemplateFormValues = {
    name: "",
    description: "",
    service_config_template: "",
    application_config_template: "",
    variables: {},
  };

  if (clusterTemplate) {
    formikInitialValues = {
      name: clusterTemplate.name,
      description: clusterTemplate.description,
      service_config_template: clusterTemplate.service_config_template,
      application_config_template: clusterTemplate.application_config_template,
      variables: clusterTemplate.variables,
    };
  }

  const formik = useFormik({
    initialValues: formikInitialValues,
    enableReinitialize: true,
    onSubmit: (values: ClusterTemplateFormValues) => {
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
              />
              {clusterTemplate && (
                <Button
                  className="float-end"
                  variant="success"
                  onClick={() => onRename?.(formik.values.name)}
                >
                  Rename
                </Button>
              )}
            </div>
          </Form.Group>
          <Form.Group className="mb-3" controlId="description">
            <Form.Label>Description</Form.Label>
            <Form.Control
              type="text"
              name="description"
              value={formik.values.description}
              onChange={formik.handleChange}
              onBlur={formik.handleBlur}
            />
          </Form.Group>
          <Form.Group className="mb-3" controlId="serviceTemplate">
            <Form.Label>Service configuration template</Form.Label>
            <Form.Control
              type="text"
              name="service_config_template"
              as="textarea"
              rows={10}
              value={formik.values.service_config_template}
              onChange={formik.handleChange}
              onBlur={formik.handleBlur}
            />
          </Form.Group>
          <Form.Group className="mb-3" controlId="applicationTemplate">
            <Form.Label>Application configuration template</Form.Label>
            <Form.Control
              type="text"
              name="application_config_template"
              as="textarea"
              rows={10}
              value={formik.values.application_config_template}
              onChange={formik.handleChange}
              onBlur={formik.handleBlur}
            />
          </Form.Group>
          <Form.Group className="mb-3" controlId="variables">
            <Form.Label>Variables</Form.Label>
            <ClusterTemplateVariablesWidget
              value={formik.values.variables}
              onChange={(value) => formik.setFieldValue("variables", value)}
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

export default ClusterTemplateForm;
