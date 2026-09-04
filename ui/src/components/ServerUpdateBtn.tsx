import { FC, useState } from "react";
import Form from "react-bootstrap/Form";
import { useQuery } from "@tanstack/react-query";
import { MdSystemUpdateAlt } from "react-icons/md";
import { fetchServerChangelog, updateSystemServer } from "api/server";
import ChangelogView from "components/ChangelogView";
import LoadingButton from "components/LoadingButton";
import ModalWindow from "components/ModalWindow";
import { useNotification } from "context/notificationContext";
import { Server } from "types/server";
import { useQueryClient } from "@tanstack/react-query";

interface Props {
  server: Server;
  recommended?: boolean;
}

type UpdateMode = "os" | "applications";

const ServerUpdateBtn: FC<Props> = ({ server, recommended }) => {
  const [showModal, setShowModal] = useState(false);
  const [opInProgress, setOpInProgress] = useState(false);
  const { notify } = useNotification();
  const queryClient = useQueryClient();
  const actionStyle = {
    cursor: "pointer",
    color: recommended ? "red" : "grey",
  };

  const osNeedsUpdate = server.version_data.os?.needs_update ?? false;
  const applicationsNeedingUpdate = (
    server.version_data.applications ?? []
  ).filter((application) => application.needs_update);

  const [updateMode, setUpdateMode] = useState<UpdateMode>(
    osNeedsUpdate ? "os" : "applications",
  );
  const [selectedApplications, setSelectedApplications] = useState<string[]>(
    applicationsNeedingUpdate.map((application) => application.name),
  );

  const toggleApplication = (name: string) => {
    setSelectedApplications((selected) =>
      selected.includes(name)
        ? selected.filter((entry) => entry !== name)
        : [...selected, name],
    );
  };

  const nothingSelected =
    updateMode === "os" ? !osNeedsUpdate : selectedApplications.length === 0;

  const {
    data: changelog = null,
    error,
    isLoading,
  } = useQuery({
    queryKey: ["servers", server.name, "changelog"],
    queryFn: () => fetchServerChangelog(server.name),
  });

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (error) {
    return <div>Error while loading changelog</div>;
  }

  const onUpdateServer = () => {
    setOpInProgress(true);
    updateSystemServer(
      server.name,
      updateMode === "os",
      updateMode === "os" ? [] : selectedApplications,
    )
      .then((response) => {
        setOpInProgress(false);
        setShowModal(false);
        if (response.error_code == 0) {
          notify.success(`Server update triggered`);
          queryClient.invalidateQueries({ queryKey: ["servers"] });
          return;
        }
        notify.error(response.error);
      })
      .catch((e) => {
        setOpInProgress(false);
        setShowModal(false);
        notify.error(`Error during server update: ${e}`);
      });
  };

  return (
    <>
      <MdSystemUpdateAlt
        size={25}
        title="Update server"
        style={actionStyle}
        onClick={() => {
          // Reset the selection to what needs an update right now, the server
          // data may have been refreshed since the component was mounted.
          setUpdateMode(osNeedsUpdate ? "os" : "applications");
          setSelectedApplications(
            applicationsNeedingUpdate.map((application) => application.name),
          );
          setShowModal(true);
        }}
      />
      <ModalWindow
        show={showModal}
        scrollable
        handleClose={() => setShowModal(false)}
        title="Update server"
        footer={
          <>
            <LoadingButton
              isLoading={opInProgress}
              variant="danger"
              disabled={nothingSelected}
              onClick={onUpdateServer}
            >
              Update
            </LoadingButton>
          </>
        }
      >
        <p>
          Are you sure you want to update server "{server.name}"?
          <br />
          {changelog?.prior_version}
          {" -> "}
          {changelog?.current_version}
        </p>
        <h3>What to update</h3>
        <Form.Check
          type="radio"
          id={`update-${server.name}-os`}
          name={`update-${server.name}-mode`}
          label={`Operating system (${server.version_data.os?.name})`}
          checked={updateMode === "os"}
          disabled={!osNeedsUpdate}
          onChange={() => setUpdateMode("os")}
        />
        <Form.Check
          type="radio"
          id={`update-${server.name}-applications`}
          name={`update-${server.name}-mode`}
          label="Individual applications"
          checked={updateMode === "applications"}
          disabled={applicationsNeedingUpdate.length === 0}
          onChange={() => setUpdateMode("applications")}
        />
        {updateMode === "applications" && (
          <div className="ms-4">
            {applicationsNeedingUpdate.map((application) => (
              <Form.Check
                key={application.name}
                type="checkbox"
                id={`update-${server.name}-${application.name}`}
                label={application.name}
                checked={selectedApplications.includes(application.name)}
                onChange={() => toggleApplication(application.name)}
              />
            ))}
          </div>
        )}
        {!osNeedsUpdate && applicationsNeedingUpdate.length === 0 && (
          <p>No component of this server needs an update.</p>
        )}
        <p>
          An update of the operating system also updates every installed
          application. It is applied with the next reboot of the server, while
          an application is updated right away.
        </p>
        <p>
          <h3>Changes</h3>
          <ChangelogView
            changelog={changelog ?? undefined}
            installedApplications={server.version_data.applications}
            osName={server.version_data.os?.name}
          />
        </p>
      </ModalWindow>
    </>
  );
};

export default ServerUpdateBtn;
