import { FC } from "react";
import { Button, Form } from "react-bootstrap";
import { useFormik } from "formik";
import { useNotification } from "context/notificationContext";
import { TokenSeed, TokenSeedFormValues } from "types/token";
import YAML from "yaml";

interface Props {
  seed?: TokenSeed;
  onSubmit: (tokenSeed: TokenSeed) => void;
}

const TokenSeedForm: FC<Props> = ({ seed, onSubmit }) => {
  const { notify } = useNotification();
  let formikInitialValues: TokenSeedFormValues = {
    name: "",
    description: "",
    public: false,
    seeds: {
      applications: "",
      install: "",
      network: "",
    },
  };

  if (seed) {
    formikInitialValues = {
      name: seed.name,
      description: seed.description,
      public: seed.public,
      seeds: {
        applications: seed.seeds.applications
          ? YAML.stringify(seed.seeds.applications)
          : "",
        install: seed.seeds.install ? YAML.stringify(seed.seeds.install) : "",
        network: seed.seeds.network ? YAML.stringify(seed.seeds.network) : "",
      },
    };
  }

  const formik = useFormik({
    initialValues: formikInitialValues,
    enableReinitialize: true,
    onSubmit: (values: TokenSeedFormValues) => {
      let parsedApplications = {};
      let parsedInstall = {};
      let parsedNetwork = {};
      try {
        parsedApplications = YAML.parse(values.seeds.applications);
        parsedInstall = YAML.parse(values.seeds.install);
        parsedNetwork = YAML.parse(values.seeds.network);
      } catch (error) {
        notify.error(`Error during yaml parsing: ${error}`);
        return;
      }

      const tokenSeed = {
        ...values,
        seeds: {
          ...values.seeds,
          applications: parsedApplications,
          install: parsedInstall,
          network: parsedNetwork,
        },
      };
      onSubmit(tokenSeed);
    },
  });

  return (
    <div className="form-container">
      <div>
        <Form noValidate>
          <Form.Group className="mb-3" controlId="name">
            <Form.Label>Name</Form.Label>
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
          </Form.Group>
          <Form.Group className="mb-3" controlId="description">
            <Form.Label>Description</Form.Label>
            <Form.Control
              type="text"
              name="description"
              value={formik.values.description}
              onChange={formik.handleChange}
              onBlur={formik.handleBlur}
              isInvalid={
                !!formik.errors.description && formik.touched.description
              }
            />
            <Form.Control.Feedback type="invalid">
              {formik.errors.description}
            </Form.Control.Feedback>
          </Form.Group>
          <Form.Group className="mb-3" controlId="public">
            <Form.Label>Public</Form.Label>
            <Form.Select
              name="public"
              value={formik.values.public ? "true" : "false"}
              onChange={(e) =>
                formik.setFieldValue("public", e.target.value === "true")
              }
              onBlur={formik.handleBlur}
              isInvalid={!!formik.errors.public && formik.touched.public}
            >
              <option value="false">No</option>
              <option value="true">Yes</option>
            </Form.Select>
          </Form.Group>
          <Form.Group className="mb-4" controlId="applications">
            <Form.Label>Applications</Form.Label>
            <Form.Control
              type="text"
              as="textarea"
              rows={6}
              name="seeds.applications"
              value={formik.values.seeds.applications}
              onChange={formik.handleChange}
              onBlur={formik.handleBlur}
            />
          </Form.Group>
          <Form.Group className="mb-4" controlId="Install">
            <Form.Label>Install</Form.Label>
            <Form.Control
              type="text"
              as="textarea"
              rows={6}
              name="seeds.install"
              value={formik.values.seeds.install}
              onChange={formik.handleChange}
              onBlur={formik.handleBlur}
            />
          </Form.Group>
          <Form.Group className="mb-4" controlId="network">
            <Form.Label>Network</Form.Label>
            <Form.Control
              type="text"
              as="textarea"
              rows={6}
              name="seeds.network"
              value={formik.values.seeds.network}
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

export default TokenSeedForm;
