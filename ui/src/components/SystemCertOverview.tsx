import { FC } from "react";
import { formatDate } from "util/date";
import { SystemCertificate } from "types/settings";

interface Props {
  certificate?: SystemCertificate;
}

const SystemCertOverview: FC<Props> = ({ certificate }) => {
  const joinOrDash = (values: string[] | undefined): string => {
    if (!values || values.length === 0) {
      return "-";
    }

    return values.join(", ");
  };

  return (
    <div className="container">
      <div className="row">
        <div className="col-2 detail-table-header">Fingerprint</div>
        <div className="col-10 detail-table-cell word-wrap">
          {certificate?.fingerprint}
        </div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Subject</div>
        <div className="col-10 detail-table-cell">{certificate?.subject}</div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Issuer</div>
        <div className="col-10 detail-table-cell">{certificate?.issuer}</div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Valid from</div>
        <div className="col-10 detail-table-cell">
          {formatDate(certificate?.not_before)}
        </div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Valid until</div>
        <div className="col-10 detail-table-cell">
          {formatDate(certificate?.not_after)}
        </div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">DNS names</div>
        <div className="col-10 detail-table-cell">
          {joinOrDash(certificate?.dns_names)}
        </div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">IP addresses</div>
        <div className="col-10 detail-table-cell">
          {joinOrDash(certificate?.ip_addresses)}
        </div>
      </div>
      <div className="row">
        <div className="col-2 detail-table-header">Certificate</div>
        <div className="col-10 detail-table-cell">
          <pre>{certificate?.certificate}</pre>
        </div>
      </div>
    </div>
  );
};

export default SystemCertOverview;
