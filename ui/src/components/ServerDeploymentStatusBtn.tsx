import { FC, ReactNode, useState } from "react";
import { ProgressBar } from "react-bootstrap";
import { IoChevronDownOutline, IoChevronUpOutline } from "react-icons/io5";
import { MdInfoOutline } from "react-icons/md";
import ModalWindow from "components/ModalWindow";
import { Server, ServerDeploymentStatus } from "types/server";
import { formatDateTime } from "util/date";
import { bytesToHumanReadable } from "util/util";

interface Props {
  server: Server;
}

const detailRow = (header: ReactNode, value: ReactNode) => {
  return (
    <div className="row">
      <div className="col-4 detail-table-header">{header}</div>
      <div className="col-8 detail-table-cell">{value}</div>
    </div>
  );
};

const mediaRead = (deployment: ServerDeploymentStatus) => {
  const read = bytesToHumanReadable(deployment.media_bytes_read);
  if (deployment.media_size <= 0) {
    return read;
  }

  return (
    <>
      {read} of {bytesToHumanReadable(deployment.media_size)}
      <ProgressBar
        now={(deployment.media_bytes_read / deployment.media_size) * 100}
      />
    </>
  );
};

const attributesRow = (
  header: string,
  attributes: Record<string, unknown>,
  visible: boolean,
  toggle: () => void,
) => {
  if (Object.keys(attributes ?? {}).length == 0) {
    return <></>;
  }

  return detailRow(
    <>
      {header}{" "}
      <span onClick={toggle} className="hide-field-switch">
        {visible ? (
          <>
            <IoChevronDownOutline /> Hide
          </>
        ) : (
          <>
            <IoChevronUpOutline /> Show
          </>
        )}
      </span>
    </>,
    visible && <pre>{JSON.stringify(attributes, null, 2)}</pre>,
  );
};

const ServerDeploymentStatusBtn: FC<Props> = ({ server }) => {
  const [showModal, setShowModal] = useState(false);
  const [areAttributesVisible, setAreAttributesVisible] = useState(false);
  const [areDeferredAttributesVisible, setAreDeferredAttributesVisible] =
    useState(false);

  const actionStyle = {
    cursor: "pointer",
    color: "grey",
  };

  const deployment = server.deployment;
  if (!deployment) {
    return <></>;
  }

  return (
    <>
      <MdInfoOutline
        size={25}
        title="Show deployment status"
        style={actionStyle}
        onClick={() => {
          setShowModal(true);
        }}
      />
      <ModalWindow
        show={showModal}
        scrollable
        handleClose={() => setShowModal(false)}
        title={`Deployment status of "${server.name}"`}
      >
        <div className="container">
          {detailRow("State", deployment.state)}
          {deployment.failed_state != "" &&
            detailRow("Failed in", deployment.failed_state)}
          {deployment.last_error != "" &&
            detailRow("Last error", deployment.last_error)}
          {deployment.retries > 0 && detailRow("Retries", deployment.retries)}
          {detailRow("Started at", formatDateTime(deployment.started_at))}
          {detailRow(
            "State entered at",
            formatDateTime(deployment.state_entered_at),
          )}
          {formatDateTime(deployment.finished_at) != "" &&
            detailRow("Finished at", formatDateTime(deployment.finished_at))}
          {detailRow("Token", deployment.request.token_uuid)}
          {detailRow("Seed", deployment.request.seed)}
          {detailRow("Virtual media", deployment.request.virtual_media_id)}
          {detailRow("Force reboot", String(deployment.force_reboot))}
          {detailRow(
            "Skip secure boot certificates",
            String(deployment.request.skip_secure_boot_certificates),
          )}
          {deployment.media_url != "" &&
            detailRow("Media URL", deployment.media_url)}
          {deployment.media_bytes_read >= 0 &&
            detailRow("Media read", mediaRead(deployment))}
          {detailRow(
            "BIOS profiles",
            (deployment.bios_profiles ?? []).join(", "),
          )}
          {attributesRow(
            "BIOS attributes",
            deployment.bios_attributes,
            areAttributesVisible,
            () => setAreAttributesVisible(!areAttributesVisible),
          )}
          {attributesRow(
            "BIOS deferred attributes",
            deployment.bios_deferred_attributes,
            areDeferredAttributesVisible,
            () =>
              setAreDeferredAttributesVisible(!areDeferredAttributesVisible),
          )}
          {deployment.history?.length > 0 &&
            detailRow(
              "History",
              deployment.history.map((step) => (
                <div key={`${step.state}-${step.entered_at}`}>
                  {formatDateTime(step.entered_at)} {step.state}
                  {step.retries > 0 && ` (retries: ${step.retries})`}
                  {step.error != "" && `: ${step.error}`}
                </div>
              )),
            )}
        </div>
      </ModalWindow>
    </>
  );
};

export default ServerDeploymentStatusBtn;
