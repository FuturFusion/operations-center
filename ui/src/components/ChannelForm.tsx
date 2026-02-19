import { FC } from "react";
import { Button, Form } from "react-bootstrap";
import { useFormik } from "formik";
import { Channel } from "types/channel";

interface Props {
  channel?: Channel;
  onSubmit: (values: Channel) => void;
}

const ChannelForm: FC<Props> = ({ channel, onSubmit }) => {
  const formikInitialValues: Channel = {
    name: channel?.name ?? "",
    description: channel?.description ?? "",
    last_updated: "",
  };

  const formik = useFormik({
    initialValues: formikInitialValues,
    enableReinitialize: true,
    onSubmit: (values: Channel) => {
      onSubmit(values);
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
              disabled={!!channel}
            />
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

export default ChannelForm;
