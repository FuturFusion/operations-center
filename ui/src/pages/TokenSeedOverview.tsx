import { useQuery } from "@tanstack/react-query";
import { useParams } from "react-router";
import { fetchTokenSeed } from "api/token";
import YAML from "yaml";

const TokenSeedOverview = () => {
  const { uuid, name } = useParams<{ uuid: string; name: string }>();

  const {
    data: seed = null,
    error,
    isLoading,
  } = useQuery({
    queryKey: ["tokens", uuid, "seeds", name],
    queryFn: () => fetchTokenSeed(uuid || "", name || ""),
  });

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (error) {
    return <div>Error while loading token seed</div>;
  }

  return (
    <div className="container">
      <div className="row">
        <div className="col-2 detail-table-header">Name</div>
        <div className="col-10 detail-table-cell">{seed?.name}</div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Description</div>
        <div className="col-10 detail-table-cell">{seed?.description}</div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Public</div>
        <div className="col-10 detail-table-cell">
          {seed?.public ? "Yes" : "No"}
        </div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Applications</div>
        <div className="col-10 detail-table-cell">
          <pre>
            {seed?.seeds.applications
              ? YAML.stringify(seed?.seeds.applications, null, 2)
              : ""}
          </pre>
        </div>
      </div>
      {seed?.seeds.applications?.applications?.[0]?.name ===
        "migration-manager" && (
        <div className="row">
          <div className="col-2 detail-table-header">
            Migration manager seed data
          </div>
          <div className="col-10 detail-table-cell">
            <pre>
              {seed?.seeds.migration_manager
                ? YAML.stringify(seed?.seeds.migration_manager, null, 2)
                : ""}
            </pre>
          </div>
        </div>
      )}
      {seed?.seeds.applications?.applications?.[0]?.name ===
        "operations-center" && (
        <div className="row">
          <div className="col-2 detail-table-header">
            Operations center seed data
          </div>
          <div className="col-10 detail-table-cell">
            <pre>
              {seed?.seeds.operations_center
                ? YAML.stringify(seed?.seeds.operations_center, null, 2)
                : ""}
            </pre>
          </div>
        </div>
      )}
      <div className="row">
        <div className="col-2 detail-table-header">Install</div>
        <div className="col-10 detail-table-cell">
          <pre>
            {seed?.seeds.install
              ? YAML.stringify(seed?.seeds.install, null, 2)
              : ""}
          </pre>
        </div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Network</div>
        <div className="col-10 detail-table-cell">
          <pre>
            {seed?.seeds.network
              ? YAML.stringify(seed?.seeds.network, null, 2)
              : ""}
          </pre>
        </div>
      </div>
    </div>
  );
};

export default TokenSeedOverview;
