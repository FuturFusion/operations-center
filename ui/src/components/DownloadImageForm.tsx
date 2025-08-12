import { FC } from "react";
import { Form } from "react-bootstrap";
import { FormikProps } from "formik/dist/types";
import { DownloadImageFormValues } from "types/token";

interface Props {
  formik: FormikProps<DownloadImageFormValues>;
}

const DownloadImageForm: FC<Props> = ({ formik }) => {
  return (
    <div>
      <Form noValidate>
        <Form.Group className="mb-3" controlId="type">
          <Form.Label>Image type</Form.Label>
          <Form.Check
            type="radio"
            label="iso"
            name="type"
            value="iso"
            checked={formik.values.type == "iso"}
            onChange={formik.handleChange}
          />
          <Form.Check
            type="radio"
            label="raw"
            name="type"
            value="raw"
            checked={formik.values.type == "raw"}
            onChange={formik.handleChange}
          />
        </Form.Group>
        <Form.Group className="mb-3">
          <Form.Label>Installation target</Form.Label>
          <Form.Check
            type="checkbox"
            label="Wipe the target drive"
            name="install.force_install"
            checked={formik.values.install.force_install}
            onChange={formik.handleChange}
            onBlur={formik.handleBlur}
          />
          <Form.Check
            type="checkbox"
            label="Automatically reboot after installation"
            name="install.force_reboot"
            checked={formik.values.install.force_reboot}
            onChange={formik.handleChange}
            onBlur={formik.handleBlur}
          />
          <Form.Label>
            Drive identifier (as seen in /dev/disk/by-id), can be a partial
            string but must match exactly one drive. If empty, IncusOS will
            auto-install so long as only one drive is present.
          </Form.Label>
          <Form.Control
            type="text"
            name="install.target.id"
            placeholder="nvme-eui.123456789"
            value={formik.values.install.target.id}
            onChange={formik.handleChange}
            onBlur={formik.handleBlur}
          />
        </Form.Group>
        <Form.Group className="mb-3" controlId="network">
          <Form.Label>Network configuration</Form.Label>
          <Form.Control
            type="text"
            as="textarea"
            rows={6}
            name="network"
            value={formik.values.network}
            onChange={formik.handleChange}
            onBlur={formik.handleBlur}
          />
        </Form.Group>
      </Form>
    </div>
  );
};

export default DownloadImageForm;
