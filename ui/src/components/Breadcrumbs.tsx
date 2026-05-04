import { Link, useLocation, matchPath } from "react-router";
import { routes } from "util/routes";

const breadcrumbPathMap: Record<string, string> = {
  "/ui/provisioning/clusters": "/ui/provisioning/clusters-view",
  "/ui/provisioning/cluster-templates":
    "/ui/provisioning/clusters-view/templates",
  "/ui/provisioning/servers": "/ui/provisioning/servers-view",
  "/ui/provisioning/tokens": "/ui/provisioning/servers-view/tokens",
  "/ui/provisioning/updates": "/ui/provisioning/updates-view",
  "/ui/provisioning/channels": "/ui/provisioning/updates-view/channels",
};

const customLabels: Record<string, string> = {
  "/ui/provisioning/clusters-view": "Clusters",
  "/ui/provisioning/clusters-view/clusters": "Clusters",
  "/ui/provisioning/clusters-view/templates": "Cluster templates",
  "/ui/provisioning/servers-view": "Servers",
  "/ui/provisioning/servers-view/servers": "Servers",
  "/ui/provisioning/servers-view/tokens": "Tokens",
  "/ui/provisioning/updates-view": "Images",
  "/ui/provisioning/updates-view/images": "Images",
  "/ui/provisioning/updates-view/channels": "Channels",
};

const Breadcrumbs = () => {
  const location = useLocation();
  const pathnames = location.pathname.split("/").filter((x) => x);

  const replacePath = (path: string) => {
    return breadcrumbPathMap[path] ?? path;
  };

  const formatBreadcrumb = (str: string) => {
    return (str.charAt(0).toUpperCase() + str.slice(1)).replace("-", " ");
  };

  const customLabel = customLabels[location.pathname];

  return (
    <nav aria-label="breadcrumb" className="w-100">
      {customLabel && <span className="breadcrumb-text">{customLabel}</span>}
      {!customLabel &&
        pathnames.map((_, index) => {
          const last = index === pathnames.length - 1;
          const to = `/${pathnames.slice(0, index + 1).join("/")}`;

          const match = routes.find((r) =>
            matchPath({ path: r.path }, replacePath(to)),
          );

          // Omit /ui.
          if (index == 0) {
            return "";
          }

          if (!match) {
            return "";
          }

          const matchedItems = match.path.split("/").filter((x) => x);
          const lastMatched = matchedItems[matchedItems.length - 1];
          const routeName =
            lastMatched.startsWith(":") && lastMatched != ":activeTab"
              ? pathnames[index]
              : formatBreadcrumb(pathnames[index]);

          return (
            <span>
              {last ? (
                <span className="breadcrumb-text">{routeName}</span>
              ) : (
                <>
                  <Link className="breadcrumb-link" to={replacePath(to)}>
                    {routeName}
                  </Link>{" "}
                  /{" "}
                </>
              )}
            </span>
          );
        })}
    </nav>
  );
};

export default Breadcrumbs;
