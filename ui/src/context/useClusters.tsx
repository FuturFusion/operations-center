import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { UseQueryResult } from "@tanstack/react-query";
import { fetchClusters } from "api/cluster";
import { fetchServers } from "api/server";
import { Cluster } from "types/cluster";

export const useClusters = (): UseQueryResult<Cluster[]> => {
  return useQuery({
    queryKey: ["clusters"],
    queryFn: () => fetchClusters(""),
  });
};

export const useClusterMap = () => {
  const { data: clusters, isLoading: clustersLoading, ...rest } = useClusters();
  const { data: servers, isLoading: serversLoading } = useQuery({
    queryKey: ["servers", ""],
    queryFn: () => fetchServers(""),
  });

  const clusterMap = useMemo(() => {
    if (!clusters) return {};

    const serverURLs: Record<string, string> = {};
    servers?.forEach((s) => {
      const url = s.public_connection_url || s.connection_url;
      if (s.cluster && url && !serverURLs[s.cluster]) {
        serverURLs[s.cluster] = url;
      }
    });

    return Object.fromEntries(
      clusters.map((c) => [c.name, c.connection_url || serverURLs[c.name]]),
    );
  }, [clusters, servers]);

  return {
    clusterMap,
    isLoading: clustersLoading || serversLoading,
    ...rest,
  };
};
