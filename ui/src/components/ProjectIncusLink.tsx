import { FC } from "react";
import ObjectIncusLink from "components/ObjectIncusLink";

type Props = {
  cluster: string;
  project: string;
};

const ProjectIncusLink: FC<Props> = ({ cluster, project }) => {
  return (
    <ObjectIncusLink
      cluster={cluster}
      objectName={project}
      incusPath={`/ui/project/${project}/instances`}
    />
  );
};

export default ProjectIncusLink;
