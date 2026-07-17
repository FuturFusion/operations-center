import type { FC, ReactNode } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { fetchOSSection, updateOSSection } from "api/os";
import YAMLEditor from "components/YAMLEditor";
import { useNotification } from "context/notificationContext";
import type { IncusOSConfig } from "types/os";
import YAML from "yaml";

interface Props {
  endpoint: string;
  queryKey: string;
  label: string;
  readOnly?: boolean;
  actions?: ReactNode;
}

const OSConfigSection: FC<Props> = ({
  endpoint,
  queryKey,
  label,
  readOnly = false,
  actions,
}) => {
  const queryClient = useQueryClient();
  const { notify } = useNotification();

  const {
    data: sectionData,
    isLoading,
    error,
  } = useQuery({
    queryKey: [queryKey],
    queryFn: async () => fetchOSSection(endpoint),
  });

  const update = (value: string): Promise<boolean> => {
    let parsed = {} as IncusOSConfig;

    try {
      parsed = YAML.parse(value);
    } catch (error) {
      notify.error(`Error during YAML value parsing: ${error}`);
      return Promise.resolve(false);
    }

    return updateOSSection(endpoint, parsed.config)
      .then(() => {
        notify.success(`${label} updated`);
        queryClient.invalidateQueries({ queryKey: [queryKey] });
        return true;
      })
      .catch((e) => {
        notify.error(`${label} update failed: ${e}`);
        return false;
      });
  };

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (error) {
    return <div>Error while loading {label.toLowerCase()} data</div>;
  }

  return (
    <div className="d-flex flex-column" style={{ height: "70vh" }}>
      {actions && <div className="d-flex gap-2 mb-3 flex-wrap">{actions}</div>}
      <div className="flex-grow-1">
        {readOnly ? (
          <pre className="bg-body-tertiary border rounded-3 p-3 mb-0 yaml-editor">
            {YAML.stringify(sectionData, null, 2)}
          </pre>
        ) : (
          <YAMLEditor
            yamlData={YAML.stringify(sectionData, null, 2)}
            onSubmit={update}
          />
        )}
      </div>
    </div>
  );
};

export default OSConfigSection;
