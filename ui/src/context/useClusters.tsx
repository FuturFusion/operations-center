import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { UseQueryResult } from "@tanstack/react-query";
import { fetchClusters } from "api/cluster";
import { Cluster } from "types/cluster";

export const useClusters = (): UseQueryResult<Cluster[]> => {
  return useQuery({
    queryKey: ["clusters"],
    queryFn: fetchClusters,
  });
};

export const useClusterMap = () => {
  const { data: clusters, ...rest } = useClusters();

  const clusterMap = useMemo(() => {
    if (!clusters) return {};
    return Object.fromEntries(clusters.map((c) => [c.name, c.connection_url]));
  }, [clusters]);

  return { clusterMap, ...rest };
};
