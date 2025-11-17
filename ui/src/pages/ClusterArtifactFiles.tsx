import { useQuery } from "@tanstack/react-query";
import { useParams } from "react-router";
import { downloadArtifactFile, fetchClusterArtifact } from "api/cluster";
import DataTable from "components/DataTable";
import { useNotification } from "context/notificationContext";
import { bytesToHumanReadable, downloadFile } from "util/util";

const ClusterArtifactFiles = () => {
  const { notify } = useNotification();
  const { clusterName, artifactName } = useParams<{
    clusterName: string;
    artifactName: string;
  }>();

  const {
    data: artifact = undefined,
    error,
    isLoading,
  } = useQuery({
    queryKey: ["clusters", clusterName, "artifacts", artifactName, "files"],
    queryFn: () => fetchClusterArtifact(clusterName || "", artifactName || ""),
  });

  const onDownloadFile = async (filename: string) => {
    try {
      const url = await downloadArtifactFile(
        clusterName ?? "",
        artifactName ?? "",
        filename,
      );

      downloadFile(url, filename);
    } catch (error) {
      notify.error(`Error during file downloading: ${error}`);
    }
  };

  if (isLoading) {
    return <div>Loading clusters artifact files...</div>;
  }

  if (error) {
    return (
      <div>Error while loading cluster artifact files: {error.message}</div>
    );
  }

  const headers = ["Name", "Mime type", "Size"];
  const rows =
    artifact?.files.map((item) => {
      return [
        {
          content: (
            <a
              href="#"
              className="data-table-link"
              onClick={(e) => {
                e.preventDefault();
                onDownloadFile(item.name);
              }}
            >
              {item.name}
            </a>
          ),
          sortKey: item.name,
        },
        {
          content: item.mime_type,
          sortKey: item.mime_type,
        },
        {
          content: bytesToHumanReadable(item.size),
          sortKey: item.size,
        },
      ];
    }) ?? [];

  return (
    <>
      <div className="d-flex flex-column">
        <div className="scroll-container flex-grow-1">
          <DataTable headers={headers} rows={rows} />
        </div>
      </div>
    </>
  );
};

export default ClusterArtifactFiles;
