import { useQuery } from "@tanstack/react-query";
import { UseQueryResult } from "@tanstack/react-query";
import { fetchClusterTemplates } from "api/cluster_template";
import { ClusterTemplate } from "types/cluster_template";

export const useClusterTemplates = (): UseQueryResult<ClusterTemplate[]> => {
  return useQuery({
    queryKey: ["cluster-templates"],
    queryFn: fetchClusterTemplates,
  });
};
