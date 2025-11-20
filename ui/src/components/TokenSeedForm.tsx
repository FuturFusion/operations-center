import { FC } from "react";
import { Button, Form } from "react-bootstrap";
import { FormikErrors, useFormik } from "formik";
import SecondaryIncusSelect from "components/SecondaryIncusSelect";
import { useNotification } from "context/notificationContext";
import {
  TokenSeed,
  TokenSeedApplication,
  TokenSeedFormValues,
} from "types/token";
import { applicationsOptions, secondaryIncusAppOptions } from "util/util";
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
      application: "",
      secondary_applications: [],
      migration_manager: "",
      operations_center: "",
      install: "",
      network: "",
    },
  };

  const getMainApplication = (apps: TokenSeedApplication[]): string => {
    return apps.find((app) => app.name in applicationsOptions)?.name ?? "";
  };

  const getSecondaryApplications = (apps: TokenSeedApplication[]): string[] => {
    return (
      apps
        .filter((app) => app.name in secondaryIncusAppOptions)
        .map((a) => a.name) ?? []
    );
  };

  if (seed) {
    formikInitialValues = {
      name: seed.name,
      description: seed.description,
      public: seed.public,
      seeds: {
        application: getMainApplication(
          seed.seeds.applications?.applications ?? [],
        ),
        secondary_applications: getSecondaryApplications(
          seed.seeds.applications?.applications ?? [],
        ),
        migration_manager: seed.seeds.migration_manager
          ? YAML.stringify(seed.seeds.migration_manager)
          : "",
        operations_center: seed.seeds.operations_center
          ? YAML.stringify(seed.seeds.operations_center)
          : "",
        install: seed.seeds.install ? YAML.stringify(seed.seeds.install) : "",
        network: seed.seeds.network ? YAML.stringify(seed.seeds.network) : "",
      },
    };
  }

  const validateForm = (
    values: TokenSeedFormValues,
  ): FormikErrors<TokenSeedFormValues> => {
    const errors: FormikErrors<TokenSeedFormValues> = {};

    if (!values.seeds.application) {
      errors.seeds ??= {};
      errors.seeds.application = "Applications is required";
    }

    return errors;
  };

  const formik = useFormik({
    initialValues: formikInitialValues,
    enableReinitialize: true,
    validate: validateForm,
    onSubmit: (values: TokenSeedFormValues) => {
      let parsedInstall = {};
      let parsedNetwork = {};
      let parsedMigrationManager = {};
      let parsedOperationsCenter = {};
      try {
        parsedInstall = YAML.parse(values.seeds.install);
        parsedNetwork = YAML.parse(values.seeds.network);
        parsedMigrationManager = YAML.parse(values.seeds.migration_manager);
        parsedOperationsCenter = YAML.parse(values.seeds.operations_center);
      } catch (error) {
        notify.error(`Error during yaml parsing: ${error}`);
        return;
      }

      let applications = [{ name: values.seeds.application }];
      if (values.seeds.application == "incus") {
        applications = [
          ...applications,
          ...values.seeds.secondary_applications.map((app) => {
            return { name: app };
          }),
        ];
      }

      const tokenSeed = {
        ...values,
        seeds: {
          ...values.seeds,
          applications: { applications: applications },
          install: parsedInstall,
          network: parsedNetwork,
          migration_manager: parsedMigrationManager,
          operations_center: parsedOperationsCenter,
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
          <Form.Group className="mb-4" controlId="application">
            <Form.Label>Application</Form.Label>
            <Form.Select
              value={formik.values.seeds?.application}
              onChange={(e) => {
                formik.setFieldValue("seeds.application", e.target.value);
              }}
              isInvalid={!!formik.errors.seeds?.application}
            >
              {Object.entries(applicationsOptions).map(([value, label]) => (
                <option key={value} value={value}>
                  {label}
                </option>
              ))}
            </Form.Select>
            <Form.Control.Feedback type="invalid">
              {formik.errors.seeds?.application}
            </Form.Control.Feedback>
          </Form.Group>
          {formik.values.seeds.application === "incus" && (
            <SecondaryIncusSelect
              value={formik.values.seeds.secondary_applications}
              onChange={(val, checked) => {
                if (checked) {
                  formik.setFieldValue("seeds.secondary_applications", [
                    ...formik.values.seeds.secondary_applications,
                    val,
                  ]);
                } else {
                  formik.setFieldValue(
                    "seeds.secondary_applications",
                    formik.values.seeds.secondary_applications.filter(
                      (v) => v !== val,
                    ),
                  );
                }
              }}
            />
          )}
          {formik.values.seeds.application === "migration-manager" && (
            <Form.Group className="mb-4" controlId="migration-manager">
              <Form.Label>Migration manager seed data</Form.Label>
              <Form.Control
                type="text"
                as="textarea"
                rows={6}
                name="seeds.migration_manager"
                value={formik.values.seeds.migration_manager}
                onChange={formik.handleChange}
                onBlur={formik.handleBlur}
              />
            </Form.Group>
          )}
          {formik.values.seeds.application === "operations-center" && (
            <Form.Group className="mb-4" controlId="operations-center">
              <Form.Label>Operations center seed data</Form.Label>
              <Form.Control
                type="text"
                as="textarea"
                rows={6}
                name="seeds.operations_center"
                value={formik.values.seeds.operations_center}
                onChange={formik.handleChange}
                onBlur={formik.handleBlur}
              />
            </Form.Group>
          )}
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
