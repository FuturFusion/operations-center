import { FC, useRef } from "react";
import { Button, Form } from "react-bootstrap";
import DatePicker from "react-datepicker";
import { useFormik } from "formik";
import { Token, TokenFormValues } from "types/token";

interface Props {
  token?: Token;
  onSubmit: (values: TokenFormValues) => void;
}

const TokenForm: FC<Props> = ({ token, onSubmit }) => {
  const in30Days = useRef<Date | null>(null);

  if (in30Days.current === null) {
    const now = new Date();
    in30Days.current = new Date(now.getTime() + 30 * 24 * 60 * 60 * 1000);
  }

  let formikInitialValues: TokenFormValues = {
    description: "",
    expire_at: in30Days.current?.toISOString(),
    uses_remaining: 1,
  };

  if (token) {
    formikInitialValues = {
      description: token.description,
      expire_at: token.expire_at,
      uses_remaining: token.uses_remaining,
    };
  }

  const formik = useFormik({
    initialValues: formikInitialValues,
    enableReinitialize: true,
    onSubmit: (values: TokenFormValues) => {
      onSubmit(values);
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
              isInvalid={
                !!formik.errors.description && formik.touched.description
              }
            />
            <Form.Control.Feedback type="invalid">
              {formik.errors.description}
            </Form.Control.Feedback>
          </Form.Group>
          <Form.Group className="mb-3" controlId="expire_at">
            <Form.Label>Expiry</Form.Label>
            <div>
              <DatePicker
                id="expire_at"
                name="expire_at"
                className="form-control"
                placeholderText="Expiry"
                selected={new Date(formik.values.expire_at)}
                onChange={(date) => formik.setFieldValue("expire_at", date)}
                showTimeSelect
                timeFormat="HH:mm"
                timeIntervals={60}
                timeCaption="time"
                dateFormat="yyyy-MM-dd HH:mm:ss"
              />
            </div>
            <Form.Control.Feedback type="invalid">
              {formik.errors.expire_at}
            </Form.Control.Feedback>
          </Form.Group>
          <Form.Group className="mb-3" controlId="uses_remaining">
            <Form.Label>Remaining uses</Form.Label>
            <Form.Control
              type="number"
              name="uses_remaining"
              value={formik.values.uses_remaining}
              onChange={formik.handleChange}
              onBlur={formik.handleBlur}
              isInvalid={
                !!formik.errors.uses_remaining && formik.touched.uses_remaining
              }
            />
            <Form.Control.Feedback type="invalid">
              {formik.errors.uses_remaining}
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

export default TokenForm;
