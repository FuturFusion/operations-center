import { useQuery } from "@tanstack/react-query";
import { useParams } from "react-router";
import { BsHash } from "react-icons/bs";
import { fetchUpdateFiles } from "api/update";
import DataTable from "components/DataTable.tsx";
import { bytesToHumanReadable } from "util/util";

const UpdateFiles = () => {
  const { uuid } = useParams();

  const {
    data: files = [],
    error,
    isLoading,
  } = useQuery({
    queryKey: ["updates", uuid, "files"],
    queryFn: () => fetchUpdateFiles(uuid || ""),
  });

  if (isLoading) {
    return <div>Loading files...</div>;
  }

  if (error) {
    return <div>Error while loading files: {error.message}</div>;
  }

  const headers = ["Filename", "Size", "Component", "Type", "Architecture", ""];

  const rows = files.map((item) => {
    return [
      {
        content: item.filename,
        sortKey: item.filename,
      },
      {
        content: bytesToHumanReadable(item.size),
        sortKey: item.size,
      },
      {
        content: item.component,
        sortKey: item.component,
      },
      {
        content: item.type,
        sortKey: item.type,
      },
      {
        content: item.architecture,
        sortKey: item.architecture,
      },
      {
        content: <BsHash title={item.sha256} style={{ cursor: "pointer" }} />,
        sortKey: item.sha256,
      },
    ];
  });

  return <DataTable headers={headers} rows={rows} />;
};

export default UpdateFiles;
