import { FC, KeyboardEvent } from "react";
import { Button, Form } from "react-bootstrap";
import { useFormik } from "formik";
import ChannelMultiSelect from "components/ChannelMultiSelect";
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
            <ChannelMultiSelect
              value={formik.values.channels}
              onChange={(selected) =>
                formik.setFieldValue("channels", selected)
              }
              onKeyDown={handleCtrlA(handleChannelsCtrlA)}
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

export default UpdateForm;
