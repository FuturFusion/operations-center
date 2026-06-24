import type { FC } from "react";
import { Link, useParams } from "react-router";
import { MdOutlineRestartAlt } from "react-icons/md";
import { useQuery } from "@tanstack/react-query";
import { fetchOSServices, runOSAction } from "api/os";
import ExtendedDataTable from "components/ExtendedDataTable";
import OSAction from "components/OSAction";
import { nameFromURL } from "util/os";
import OSServiceDetails from "./OSServiceDetails";

const OSServices: FC = () => {
  const { subTab } = useParams<{ subTab?: string }>();

  const {
    data: services,
    isLoading,
    error,
  } = useQuery({
    queryKey: ["os-services"],
    queryFn: async () => fetchOSServices(),
  });

  if (subTab) {
    return <OSServiceDetails name={subTab} />;
  }

  const headers = ["Name", "Actions"];

  const rows =
    services?.map((item) => {
      const serviceName = nameFromURL(item);
      return {
        cols: [
          {
            content: [
              <Link
                to={`/ui/os/services/${serviceName}`}
                className="data-table-link"
                title="Service details"
              >
                {serviceName}
              </Link>,
            ],
            sortKey: serviceName,
          },
          {
            content: (
              <OSAction
                label="Reset service"
                mode="confirm"
                icon={<MdOutlineRestartAlt size={22} />}
                confirmMessage={`Reset the ${serviceName} service?`}
                run={() => runOSAction(`services/${serviceName}`, "reset")}
                successMessage={`Service ${serviceName} reset`}
              />
            ),
          },
        ],
      };
    }) || [];

  return (
    <ExtendedDataTable
      headers={headers}
      rows={rows}
      isLoading={isLoading}
      error={error}
    />
  );
};

export default OSServices;
