import { FC } from "react";
import { Button, Form } from "react-bootstrap";
import { useFormik } from "formik";
import ChannelSelect from "components/ChannelSelect";
import { Cluster, ClusterFormValues } from "types/cluster";

interface Props {
  cluster?: Cluster;
  onRename: (newName: string) => void;
  onSubmit: (values: ClusterFormValues) => void;
}

const ClusterForm: FC<Props> = ({ cluster, onRename, onSubmit }) => {
  const formikInitialValues = {
    name: cluster?.name || "",
    connection_url: cluster?.connection_url || "",
    channel: cluster?.channel || "",
  };

  const formik = useFormik({
    initialValues: formikInitialValues,
    enableReinitialize: true,
    onSubmit: (values: ClusterFormValues) => {
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
          <Form.Group className="mb-3" controlId="connection_url">
            <Form.Label>Connection URL</Form.Label>
            <Form.Control
              type="text"
              name="connection_url"
              value={formik.values.connection_url}
              onChange={formik.handleChange}
              onBlur={formik.handleBlur}
            />
          </Form.Group>
          <ChannelSelect
            formClasses="mb-3"
            value={formik.values.channel}
            onChange={(val) => formik.setFieldValue("channel", val)}
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

export default ClusterForm;
