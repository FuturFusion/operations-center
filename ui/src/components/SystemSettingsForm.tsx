import { FC } from "react";
import { Button, Form } from "react-bootstrap";
import { useFormik } from "formik";
import { SystemSettings } from "types/settings";
import { LogLevel } from "util/settings";

interface Props {
  settings?: SystemSettings;
  onSubmit: (values: SystemSettings) => void;
}

const SystemSettingsForm: FC<Props> = ({ settings, onSubmit }) => {
  const formikInitialValues: SystemSettings = {
    log_level: settings?.log_level ?? "",
  };

  const formik = useFormik({
    initialValues: formikInitialValues,
    enableReinitialize: true,
    onSubmit: (values: SystemSettings) => {
      onSubmit(values);
    },
  });

  return (
    <div className="form-container">
      <div>
        <Form noValidate>
          <Form.Group className="mb-3" controlId="log_level">
            <Form.Label>Log level</Form.Label>
            <Form.Select
              name="log_level"
              value={formik.values.log_level}
              onChange={formik.handleChange}
              onBlur={formik.handleBlur}
              isInvalid={!!formik.errors.log_level && formik.touched.log_level}
            >
              {Object.values(LogLevel).map((value) => (
                <option key={value} value={value}>
                  {value}
                </option>
              ))}
            </Form.Select>
            <Form.Control.Feedback type="invalid">
              {formik.errors.log_level}
            </Form.Control.Feedback>
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

export default SystemSettingsForm;
