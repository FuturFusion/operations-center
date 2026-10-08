import { FC, useState } from "react";
import { MdSystemUpdateAlt } from "react-icons/md";
import { updateClusterRolling } from "api/cluster";
import LoadingButton from "components/LoadingButton";
import ModalWindow from "components/ModalWindow";
import { useNotification } from "context/notificationContext";
import { useServers } from "context/useServers";
import { Cluster, ClusterUpdatePost } from "types/cluster";
import { useQueryClient } from "@tanstack/react-query";
import { Form } from "react-bootstrap";
import { errorMessage } from "util/response";

interface Props {
  cluster: Cluster;
  recommended?: boolean;
}

type UpdateMode = "os" | "applications";

const ClusterUpdateBtn: FC<Props> = ({ cluster, recommended }) => {
  const [showModal, setShowModal] = useState(false);
  const [opInProgress, setOpInProgress] = useState(false);
  const [reboot, setReboot] = useState(true);
  const [updateMode, setUpdateMode] = useState<UpdateMode>("os");
  const [osOnly, setOsOnly] = useState(false);
  const [selectedApplications, setSelectedApplications] = useState<string[]>(
    [],
  );
  const {
    data: servers = [],
    error: serversError,
    isFetching: serversFetching,
    refetch: refetchServers,
  } = useServers("");
  const { notify } = useNotification();
  const queryClient = useQueryClient();
  const actionStyle = {
    cursor: "pointer",
    color: recommended ? "red" : "grey",
  };

  // Applications, which need an update on any server of the cluster.
  const applicationsNeedingUpdate = [
    ...new Set(
      servers
        .filter((server) => server.cluster === cluster.name)
        .flatMap((server) => server.version_data.applications ?? [])
        .filter((application) => application.needs_update)
        .map((application) => application.name),
    ),
  ].sort();

  const toggleApplication = (name: string) => {
    setSelectedApplications((selected) =>
      selected.includes(name)
        ? selected.filter((entry) => entry !== name)
        : [...selected, name],
    );
  };

  const nothingSelected =
    updateMode === "applications" && selectedApplications.length === 0;

  const openModal = () => {
    setUpdateMode("os");
    setOsOnly(false);
    setReboot(true);
    setSelectedApplications([]);
    refetchServers();
    setShowModal(true);
  };

  const onUpdateCluster = () => {
    const request: ClusterUpdatePost = {
      reboot: updateMode === "os" && reboot,
      applications: updateMode === "os" ? [] : selectedApplications,
      os_only: updateMode === "os" && osOnly,
    };

    setOpInProgress(true);
    updateClusterRolling(cluster.name, JSON.stringify(request, null, 2))
      .then((response) => {
        setOpInProgress(false);
        setShowModal(false);
        if (response.error_code == 0) {
          notify.success(`Cluster update triggered`);
          queryClient.invalidateQueries({ queryKey: ["clusters"] });
          queryClient.invalidateQueries({ queryKey: ["servers"] });
          return;
        }
        notify.error(errorMessage(response));
      })
      .catch((e) => {
        setOpInProgress(false);
        setShowModal(false);
        notify.error(`Error during cluster update: ${e}`);
      });
  };

  return (
    <>
      <MdSystemUpdateAlt
        size={25}
        title="Update cluster"
        style={actionStyle}
        onClick={openModal}
      />
      <ModalWindow
        show={showModal}
        scrollable
        handleClose={() => setShowModal(false)}
        title="Update cluster"
        footer={
          <>
            <LoadingButton
              isLoading={opInProgress}
              variant="danger"
              disabled={nothingSelected}
              onClick={onUpdateCluster}
            >
              Update
            </LoadingButton>
          </>
        }
      >
        <div>
          <div className="mb-3">
            Are you sure you want to update the cluster "{cluster.name}"?
          </div>
          <h3>What to update</h3>
          <Form.Check
            type="radio"
            id={`update-${cluster.name}-os`}
            name={`update-${cluster.name}-mode`}
            label="Operating system and applications"
            checked={updateMode === "os"}
            onChange={() => setUpdateMode("os")}
          />
          {updateMode === "os" && (
            <div className="ms-4">
              <Form.Check
                type="checkbox"
                id={`update-${cluster.name}-os-only`}
                label="Leave the installed applications on their current version"
                checked={osOnly}
                onChange={() => setOsOnly((selected) => !selected)}
              />
              <Form.Check
                type="checkbox"
                id={`update-${cluster.name}-reboot`}
                label="Perform a rolling reboot following the installation of the update"
                checked={reboot}
                onChange={(e) => setReboot(e.target.checked)}
              />
            </div>
          )}
          <Form.Check
            type="radio"
            id={`update-${cluster.name}-applications`}
            name={`update-${cluster.name}-mode`}
            label="Individual applications"
            checked={updateMode === "applications"}
            disabled={applicationsNeedingUpdate.length === 0}
            onChange={() => {
              setUpdateMode("applications");
              setSelectedApplications(applicationsNeedingUpdate);
            }}
          />
          {serversError && (
            <Form.Text className="ms-4 text-danger">
              Failed to load the applications: {serversError.message}
            </Form.Text>
          )}
          {!serversError &&
            serversFetching &&
            applicationsNeedingUpdate.length === 0 && (
              <Form.Text className="ms-4" muted>
                Loading the applications ...
              </Form.Text>
            )}
          {updateMode === "applications" && (
            <div className="ms-4">
              {applicationsNeedingUpdate.map((application) => (
                <Form.Check
                  key={application}
                  type="checkbox"
                  id={`update-${cluster.name}-${application}`}
                  label={application}
                  checked={selectedApplications.includes(application)}
                  onChange={() => toggleApplication(application)}
                />
              ))}
            </div>
          )}
          <p className="mt-3">
            An update of the operating system also updates every installed
            application, unless it is restricted to the operating system. The
            operating system update is staged and applied with the reboot of the
            servers, while an application is updated right away.
          </p>
        </div>
      </ModalWindow>
    </>
  );
};

export default ClusterUpdateBtn;
