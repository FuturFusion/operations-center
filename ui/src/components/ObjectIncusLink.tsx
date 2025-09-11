import { FC } from "react";
import { Link } from "react-router";
import { useClusterMap } from "context/useClusters";

type Props = {
  cluster: string;
  incusPath: string;
  objectName: string;
};

const ObjectIncusLink: FC<Props> = ({ cluster, incusPath, objectName }) => {
  const { clusterMap, isLoading } = useClusterMap();

  if (isLoading) return <>{objectName}</>;

  const href = new URL(incusPath, clusterMap[cluster]).toString();

  return (
    <Link
      to={href}
      target="_blank"
      rel="noopener noreferrer"
      className="data-table-link"
    >
      {objectName}
    </Link>
  );
};

export default ObjectIncusLink;
