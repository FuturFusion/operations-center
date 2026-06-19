import { FC } from "react";
import { Button, Form } from "react-bootstrap";
import { useFormik } from "formik";
import { ImageSource } from "types/image_incus_source";

interface Props {
  source?: ImageSource;
  onSubmit: (values: ImageSource) => void;
}

const ImageSourceForm: FC<Props> = ({ source, onSubmit }) => {
  const formikInitialValues: ImageSource = {
    name: source?.name ?? "",
    url: source?.url ?? "",
    filter_expression: source?.filter_expression ?? "",
    last_updated: "",
  };

  const formik = useFormik({
    initialValues: formikInitialValues,
    enableReinitialize: true,
    onSubmit: (values: ImageSource) => {
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
              disabled={!!source}
            />
          </Form.Group>
          <Form.Group className="mb-3" controlId="url">
            <Form.Label>URL</Form.Label>
            <Form.Control
              type="text"
              name="url"
              value={formik.values.url}
              onChange={formik.handleChange}
              onBlur={formik.handleBlur}
            />
          </Form.Group>
          <Form.Group className="mb-3" controlId="filter_expression">
            <Form.Label>Filter expression</Form.Label>
            <Form.Control
              type="text"
              name="filter_expression"
              value={formik.values.filter_expression}
              onChange={formik.handleChange}
              onBlur={formik.handleBlur}
            />
            <Form.Text muted>
              Only images matching the filter expression are fetched, e.g.
              architecture == "amd64". Use "true" to fetch all images.
            </Form.Text>
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

export default ImageSourceForm;
