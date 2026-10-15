import { FC } from "react";
import { useQueries } from "@tanstack/react-query";
import { fetchUpdateChangelog } from "api/update";
import ChangelogView from "components/ChangelogView";
import { Update } from "types/update";

interface Props {
  update: Update;
}

const UpdateChangelogs: FC<Props> = ({ update }) => {
  // A changelog depends on the channel, since the prior version is taken from
  // the channel. Fall back to the upstream channels, if the update is not
  // assigned to any downstream channel.
  const upstream = (update.channels ?? []).length === 0;
  const channels = upstream
    ? (update.upstream_channels ?? [])
    : (update.channels ?? []);

  const results = useQueries({
    queries: channels.map((channel) => ({
      queryKey: ["updates", update.uuid, "changelog", channel, upstream],
      queryFn: () => fetchUpdateChangelog(update.uuid, channel, upstream),
    })),
  });

  if (channels.length === 0) {
    return <div>Update is not assigned to any channel</div>;
  }

  return (
    <div>
      {channels.map((channel, index) => {
        const { data: changelog, error, isLoading } = results[index];

        return (
          <div className="mb-3" key={channel}>
            <p className="mb-1">
              <strong>
                Channel {channel}
                {upstream ? " (upstream)" : ""}
              </strong>
              {changelog
                ? `: ${changelog.prior_version || "initial"} → ${changelog.current_version}`
                : ""}
            </p>
            {isLoading && <div>Loading...</div>}
            {error && <div>Error while loading changelog: {error.message}</div>}
            {changelog &&
              Object.keys(changelog.components ?? {}).length === 0 && (
                <div>No changes</div>
              )}
            {changelog && <ChangelogView changelog={changelog} />}
          </div>
        );
      })}
    </div>
  );
};

export default UpdateChangelogs;
