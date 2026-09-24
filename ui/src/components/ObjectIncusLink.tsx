import { FC } from "react";
import { Link } from "react-router";
import { useClusterMap } from "context/useClusters";

type Props = {
  cluster: string;
  incusPath: string;
  objectName: string;
};

const incusURL = (incusPath: string, clusterURL?: string): string => {
  if (!clusterURL) return "";

  try {
    return new URL(incusPath, clusterURL).toString();
  } catch {
    return "";
  }
};

const ObjectIncusLink: FC<Props> = ({ cluster, incusPath, objectName }) => {
  const { clusterMap, isLoading } = useClusterMap();

  if (isLoading) return <>{objectName}</>;

  const href = incusURL(incusPath, clusterMap[cluster]);
  if (!href) return <>{objectName}</>;

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
