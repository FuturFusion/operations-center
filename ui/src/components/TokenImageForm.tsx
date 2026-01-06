import { FC } from "react";
import { Form } from "react-bootstrap";
import { FormikProps } from "formik/dist/types";
import ArchSelect from "components/ArchSelect";
import BootSecuritySelect from "components/BootSecuritySelect";
import ImageTypeSelect from "components/ImageTypeSelect";
import SecondaryIncusSelect from "components/SecondaryIncusSelect";
import { TokenImageFormValues } from "types/token";
import { ServerTypeString } from "util/server";

interface Props {
  formik: FormikProps<TokenImageFormValues>;
}

const TokenImageForm: FC<Props> = ({ formik }) => {
  return (
    <div>
      <Form noValidate>
        <ImageTypeSelect
          value={formik.values.type}
          onChange={(val) => formik.setFieldValue("type", val)}
        />
        <ArchSelect
          value={formik.values.architecture}
          onChange={(val) => formik.setFieldValue("architecture", val)}
        />
        <Form.Group className="mb-4">
          <Form.Label>Installation target</Form.Label>
          <Form.Check
            type="checkbox"
            label="Wipe the target drive"
            name="seeds.install.force_install"
            checked={formik.values.seeds.install.force_install}
            onChange={formik.handleChange}
            onBlur={formik.handleBlur}
          />
          <Form.Check
            type="checkbox"
            label="Automatically reboot after installation"
            name="seeds.install.force_reboot"
            checked={formik.values.seeds.install.force_reboot}
            onChange={formik.handleChange}
            onBlur={formik.handleBlur}
            className="mb-3"
          />
          <Form.Label>
            Drive identifier (as seen in /dev/disk/by-id), can be a partial
            string but must match exactly one drive. If empty, IncusOS will
            auto-install so long as only one drive is present.
          </Form.Label>
          <Form.Control
            type="text"
            name="seeds.install.target.id"
            placeholder="nvme-eui.123456789"
            value={formik.values.seeds.install.target.id}
            onChange={formik.handleChange}
            onBlur={formik.handleBlur}
          />
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
            {Object.entries(ServerTypeString).map(([value, label]) => (
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
              className="editor"
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
              className="editor"
            />
          </Form.Group>
        )}
        <Form.Group className="mb-4" controlId="network">
          <Form.Label>Network configuration</Form.Label>
          <Form.Control
            type="text"
            as="textarea"
            rows={6}
            name="seeds.network"
            value={formik.values.seeds.network}
            onChange={formik.handleChange}
            onBlur={formik.handleBlur}
            className="editor"
          />
        </Form.Group>
        <BootSecuritySelect
          value={formik.values.seeds.install.boot_security}
          onChange={(val) =>
            formik.setFieldValue("seeds.install.boot_security", val)
          }
        />
      </Form>
    </div>
  );
};

export default TokenImageForm;
