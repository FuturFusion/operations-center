import { FC } from "react";
import { Form } from "react-bootstrap";
import { FormikProps } from "formik/dist/types";
import { ClusterBulkUpdateFormValues } from "types/cluster";

interface Props {
  formik: FormikProps<ClusterBulkUpdateFormValues>;
}

const ClusterBulkUpdateForm: FC<Props> = ({ formik }) => {
  return (
    <div>
      <Form noValidate>
        <Form.Group className="mb-4" controlId="action">
          <Form.Label>Action</Form.Label>
          <Form.Select
            value={formik.values.action}
            onChange={formik.handleChange}
            onBlur={formik.handleBlur}
          >
            <option key="" value=""></option>
            <option key="" value="add_network_interface_vlan_tags">
              Add VLAN(s) to a network interface
            </option>
            <option key="" value="remove_network_interface_vlan_tags">
              Remove VLAN(s) from a network interface
            </option>
            <option key="" value="add_iscsi_storage_target">
              Add an iSCSI target
            </option>
            <option key="" value="remove_iscsi_storage_target">
              Remove an iSCSI target
            </option>
            <option key="" value="add_nvme_storage_target">
              Add a NVME-over-TCP target
            </option>
            <option key="" value="remove_nvme_storage_target">
              Remove a NVME-over-TCP target
            </option>
            <option key="" value="add_multipath_storage_target">
              Add a multipath LUN
            </option>
            <option key="" value="remove_multipath_storage_target">
              Remove a multipath LUN
            </option>
            <option key="" value="add_application">
              Install an application
            </option>
            <option key="" value="update_system_kernel">
              Apply system kernel configuration
            </option>
            <option key="" value="update_system_logging">
              Apply system logging configuration
            </option>
          </Form.Select>
        </Form.Group>
        <Form.Group className="mb-4" controlId="arguments">
          <Form.Label>Arguments</Form.Label>
          <Form.Control
            type="text"
            as="textarea"
            rows={6}
            name="arguments"
            value={formik.values.arguments}
            onChange={formik.handleChange}
            onBlur={formik.handleBlur}
          />
        </Form.Group>
      </Form>
    </div>
  );
};

export default ClusterBulkUpdateForm;
