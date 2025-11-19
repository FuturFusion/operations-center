import { useQuery } from "@tanstack/react-query";
import { MdOutlineFileDownload } from "react-icons/md";
import { Link, useParams } from "react-router";
import { downloadArtifact, fetchClusterArtifacts } from "api/cluster";
import DataTable from "components/DataTable.tsx";
import { useNotification } from "context/notificationContext";
import { downloadFile } from "util/util";

const ClusterArtifacts = () => {
  const { notify } = useNotification();
  const { name } = useParams();
  const actionStyle = {
    cursor: "pointer",
    color: "grey",
  };

  const {
    data: artifacts = [],
    error,
    isLoading,
  } = useQuery({
    queryKey: ["clusters", name, "artifacts"],
    queryFn: () => fetchClusterArtifacts(name || ""),
  });

  const onDownloadArtifact = async (artifactName: string) => {
    try {
      const url = await downloadArtifact(name || "", artifactName);

      downloadFile(url, `${artifactName}.zip`);
    } catch (error) {
      notify.error(`Error during artifact downloading: ${error}`);
    }
  };

  if (isLoading) {
    return <div>Loading artifacts...</div>;
  }

  if (error) {
    return <div>Error while loading artifacts: {error.message}</div>;
  }

  const headers = ["Name", "Description", "Last updated", ""];

  const rows = artifacts.map((item) => {
    return [
      {
        content: (
          <Link
            to={`/ui/provisioning/clusters/${name}/artifacts/${item.name}/files`}
            className="data-table-link"
          >
            {item.name}
          </Link>
        ),
        sortKey: item.name,
      },
      {
        content: item.description,
        sortKey: item.description,
      },
      {
        content: item.last_updated,
        sortKey: item.last_updated,
      },
      {
        content: (
          <MdOutlineFileDownload
            size={25}
            title="Download artifact"
            style={actionStyle}
            onClick={() => {
              onDownloadArtifact(item.name);
            }}
          />
        ),
        sortKey: "",
      },
    ];
  });

  return <DataTable headers={headers} rows={rows} />;
};

export default ClusterArtifacts;
