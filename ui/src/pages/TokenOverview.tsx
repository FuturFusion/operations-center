import { useQuery } from "@tanstack/react-query";
import { useParams } from "react-router";
import { fetchToken } from "api/token";
import { formatDate } from "util/date";

const TokenOverview = () => {
  const { uuid } = useParams();

  const {
    data: token = null,
    error,
    isLoading,
  } = useQuery({
    queryKey: ["tokens", uuid],
    queryFn: () => fetchToken(uuid || ""),
  });

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (error) {
    return <div>Error while loading tokens</div>;
  }

  return (
    <div className="container">
      <div className="row">
        <div className="col-2 detail-table-header">Token</div>
        <div className="col-10 detail-table-cell">{token?.uuid}</div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Description</div>
        <div className="col-10 detail-table-cell">{token?.description}</div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Expiry</div>
        <div className="col-10 detail-table-cell">
          {formatDate(token?.expire_at || "")}
        </div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Remaining uses</div>
        <div className="col-10 detail-table-cell">{token?.uses_remaining}</div>
      </div>
    </div>
  );
};

export default TokenOverview;
