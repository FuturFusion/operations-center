import { useQuery } from "@tanstack/react-query";
import { UseQueryResult } from "@tanstack/react-query";
import { fetchServers } from "api/server";
import { Server } from "types/server";

export const useServers = (filter: string): UseQueryResult<Server[]> => {
  return useQuery({
    queryKey: ["servers", filter],
    queryFn: () => fetchServers(filter),
  });
};
