import { FC } from "react";
import { Button, Form } from "react-bootstrap";
import { useFormik } from "formik";
import { IncusImage, IncusImageFormValues } from "types/image_incus";

interface Props {
  image?: IncusImage;
  onSubmit: (values: IncusImageFormValues) => void;
}

const IncusImageForm: FC<Props> = ({ image, onSubmit }) => {
  const formikInitialValues = {
    aliases: (image?.aliases ?? []).join("\n"),
    description: image?.description ?? "",
  };

  const formik = useFormik({
    initialValues: formikInitialValues,
    enableReinitialize: true,
    onSubmit: (values) => {
      onSubmit({
        aliases: values.aliases
          .split("\n")
          .map((alias) => alias.trim())
          .filter((alias) => alias != ""),
        description: values.description,
      });
    },
  });

  return (
    <div className="form-container">
      <div>
        <Form noValidate>
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
          <Form.Group className="mb-3" controlId="aliases">
            <Form.Label>Aliases</Form.Label>
            <Form.Control
              as="textarea"
              rows={5}
              name="aliases"
              value={formik.values.aliases}
              onChange={formik.handleChange}
              onBlur={formik.handleBlur}
            />
            <Form.Text muted>One alias per line.</Form.Text>
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

export default IncusImageForm;
