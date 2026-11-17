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

// Use these labels instead of the path element.
const pathLabels: Record<string, string> = {
  "/ui/images/incus": "Images",
};

// Omit these paths unless they are the last element. The image detail shows
// the source as parent, not the images tab of the source.
const skippedPaths = ["/ui/images/incus/:source/images"];

const Breadcrumbs = () => {
  const location = useLocation();
  const pathnames = location.pathname.split("/").filter((x) => x);

  const replacePath = (path: string) => {
    return breadcrumbPathMap[path] ?? path;
  };

  const formatBreadcrumb = (str: string) => {
    if (str === "os") {
      return "OS";
    }

    if (str === "bmc") {
      return "BMC";
    }

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

          if (
            !last &&
            skippedPaths.some((path) => matchPath({ path: path }, to))
          ) {
            return "";
          }

          const matchedItems = match.path.split("/").filter((x) => x);
          const lastMatched = matchedItems[matchedItems.length - 1];
          const isNameParam =
            (lastMatched.startsWith(":") &&
              lastMatched != ":activeTab" &&
              lastMatched != ":subTab") ||
            (lastMatched === ":subTab" &&
              (to.includes("/ui/os/applications") ||
                to.includes("/ui/os/services")));
          const routeName =
            pathLabels[to] ??
            (isNameParam
              ? pathnames[index]
              : formatBreadcrumb(pathnames[index]));

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
