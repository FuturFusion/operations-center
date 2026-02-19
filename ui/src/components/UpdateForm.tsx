import { FC, KeyboardEvent } from "react";
import { Button, Form } from "react-bootstrap";
import { useFormik } from "formik";
import { useChannels } from "context/useChannels";
import { Update, UpdateFormValues } from "types/update";
import { handleCtrlA } from "util/util";

interface Props {
  update?: Update;
  onSubmit: (values: UpdateFormValues) => void;
}

const UpdateForm: FC<Props> = ({ update, onSubmit }) => {
  const { data: channels } = useChannels();

  const handleChannelsCtrlA = (e: KeyboardEvent<HTMLSelectElement>) => {
    e.preventDefault();
    formik.setFieldValue("channels", channels?.map((s) => s.name) ?? []);
  };

  const formikInitialValues: UpdateFormValues = {
    channels: update?.channels ?? [],
  };

  const formik = useFormik({
    initialValues: formikInitialValues,
    enableReinitialize: true,
    onSubmit: (values: UpdateFormValues) => {
      onSubmit(values);
    },
  });

  return (
    <div className="form-container">
      <div>
        <Form noValidate>
          <Form.Group className="mb-4" controlId="channels">
            <Form.Label>Channels</Form.Label>
            <Form.Select
              multiple
              value={formik.values.channels}
              onChange={(e) => {
                const selected = Array.from(
                  e.target.selectedOptions,
                  (option) => option.value,
                );
                formik.setFieldValue("channels", selected);
              }}
              onKeyDown={handleCtrlA(handleChannelsCtrlA)}
            >
              {channels?.map((channel) => (
                <option key={channel.name} value={channel.name}>
                  {channel.name}
                </option>
              ))}
            </Form.Select>
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

export default UpdateForm;
