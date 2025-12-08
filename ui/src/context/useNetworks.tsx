import { useMemo } from "react";
import { useQuery, UseQueryResult } from "@tanstack/react-query";
import { fetchNetworks } from "api/network";
import { Network } from "types/network";

export const useNetworks = (): UseQueryResult<Network[]> => {
  return useQuery({
    queryKey: ["networks"],
    queryFn: () => fetchNetworks(""),
  });
};

export const useNetworkMap = () => {
  const { data: networks, ...rest } = useNetworks();

  const networkMap = useMemo(() => {
    if (!networks) return {};
    return Object.fromEntries(networks.map((c) => [c.name, c]));
  }, [networks]);

  return { networkMap, ...rest };
};
