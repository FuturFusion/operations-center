import { Table } from "react-bootstrap";
import { useQuery } from "@tanstack/react-query";
import { useParams } from "react-router";
import { fetchClusterTemplate } from "api/cluster_template";
import { formatDate } from "util/date";

const ClusterTemplateOverview = () => {
  const { name } = useParams();

  const {
    data: template = null,
    error,
    isLoading,
  } = useQuery({
    queryKey: ["cluster-templates", name],
    queryFn: () => fetchClusterTemplate(name || ""),
  });

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (error) {
    return <div>Error while loading cluster template</div>;
  }

  return (
    <div className="container">
      <div className="row">
        <div className="col-2 detail-table-header">Name</div>
        <div className="col-10 detail-table-cell">{template?.name}</div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Description</div>
        <div className="col-10 detail-table-cell">{template?.description}</div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Last updated</div>
        <div className="col-10 detail-table-cell">
          {formatDate(template?.last_updated || "")}
        </div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">
          Service configuration template
        </div>
        <div className="col-10 detail-table-cell">
          {template?.service_config_template}
        </div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">
          Application configuration template
        </div>
        <div className="col-10 detail-table-cell">
          {template?.application_config_template}
        </div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Variables</div>
        <div className="col-10 detail-table-cell">
          {Object.keys(template?.variables || {}).length > 0 && (
            <Table borderless size="sm">
              <thead>
                <tr className="overview-table-header">
                  <th>Name</th>
                  <th>Description</th>
                  <th>Default</th>
                </tr>
              </thead>
              <tbody>
                {Object.entries(template?.variables || {}).map(
                  ([key, value], index) => (
                    <tr key={index}>
                      <td>{key}</td>
                      <td>{value.description}</td>
                      <td>{value.default}</td>
                    </tr>
                  ),
                )}
              </tbody>
            </Table>
          )}
        </div>
      </div>
    </div>
  );
};

export default ClusterTemplateOverview;
