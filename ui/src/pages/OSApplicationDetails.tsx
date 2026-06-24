import type { FC } from "react";
import { Link } from "react-router";
import OSConfigSection from "components/OSConfigSection";

interface Props {
  name: string;
}

const OSApplicationDetails: FC<Props> = ({ name }) => {
  return (
    <div className="d-flex flex-column">
      <div className="mb-3">
        <Link to="/ui/os/applications" className="data-table-link">
          &larr; Applications
        </Link>
      </div>
      <OSConfigSection
        endpoint={`applications/${name}`}
        queryKey={`os-application-${name}`}
        label="Application"
      />
    </div>
  );
};

export default OSApplicationDetails;
