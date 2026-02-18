import { useQuery } from "@tanstack/react-query";
import { UseQueryResult } from "@tanstack/react-query";
import { fetchChannels } from "api/channel";
import { Channel } from "types/channel";

export const useChannels = (): UseQueryResult<Channel[]> => {
  return useQuery({
    queryKey: ["channels"],
    queryFn: () => fetchChannels(),
  });
};
