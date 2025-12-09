import { useEffect, useState } from "react";
import { Nav, Navbar, Collapse } from "react-bootstrap";
import { useLocation } from "react-router";
import { AiOutlineCluster } from "react-icons/ai";
import { GoProjectTemplate, GoServer } from "react-icons/go";
import { IoChevronDownOutline, IoChevronForward } from "react-icons/io5";
import {
  MdLogin,
  MdLogout,
  MdOutlineInventory2,
  MdOutlineSettings,
  MdOutlineSystemUpdateAlt,
  MdOutlineToken,
} from "react-icons/md";
import {
  RiArticleLine,
  RiBox3Line,
  RiDatabase2Line,
  RiHardDrive2Line,
  RiOrganizationChart,
  RiPassPendingLine,
} from "react-icons/ri";
import { MenuItem, NavItemLink } from "components/NavItemLink";
import { useAuth } from "context/authContext";

const Sidebar = () => {
  const { isAuthenticated } = useAuth();
  const [openSubmenu, setOpenSubmenu] = useState(["", ""]);
  const { pathname } = useLocation();

  const logout = () => {
    fetch("/oidc/logout").then(() => {
      window.location.href = "/ui/";
    });
  };

  const toggleSubmenu = (menuKey: string, level: number) => {
    const updated = openSubmenu.map((item, i) => {
      if (i !== level) {
        return item;
      }

      return openSubmenu[level] === menuKey ? "" : menuKey;
    });

    setOpenSubmenu(updated);
  };

  const menuFoldIcon = (menuKey: string, level: number) => {
    if (openSubmenu[level] === menuKey) {
      return <IoChevronDownOutline />;
    }

    return <IoChevronForward />;
  };

  const menuItems: Record<string, MenuItem> = {
    images: {
      id: "images",
      to: "/ui/inventory/images",
      menu: ["inventory", ""],
    },
    instances: {
      id: "instances",
      to: "/ui/inventory/instances",
      menu: ["inventory", ""],
    },
    networks: {
      id: "networks",
      to: "/ui/inventory/networks",
      menu: ["inventory", "networking"],
    },
    network_acls: {
      id: "network_acls",
      to: "/ui/inventory/network_acls",
      menu: ["inventory", "networking"],
    },
    network_forwards: {
      id: "network_forwards",
      to: "/ui/inventory/network_forwards",
      menu: ["inventory", "networking"],
    },
    network_integrations: {
      id: "network_integrations",
      to: "/ui/inventory/network_integrations",
      menu: ["inventory", "networking"],
    },
    network_load_balancers: {
      id: "network_load_balancers",
      to: "/ui/inventory/network_load_balancers",
      menu: ["inventory", "networking"],
    },
    network_peers: {
      id: "network_peers",
      to: "/ui/inventory/network_peers",
      menu: ["inventory", "networking"],
    },
    network_zones: {
      id: "network_zones",
      to: "/ui/inventory/network_zones",
      menu: ["inventory", "networking"],
    },
    storage_buckets: {
      id: "storage_buckets",
      to: "/ui/inventory/storage_buckets",
      menu: ["inventory", "storage"],
    },
    storage_pools: {
      id: "storage_pools",
      to: "/ui/inventory/storage_pools",
      menu: ["inventory", "storage"],
    },
    storage_volumes: {
      id: "storage_volumes",
      to: "/ui/inventory/storage_volumes",
      menu: ["inventory", "storage"],
    },
    profiles: {
      id: "profiles",
      to: "/ui/inventory/profiles",
      menu: ["inventory", ""],
    },
    projects: {
      id: "projects",
      to: "/ui/inventory/projects",
      menu: ["inventory", ""],
    },
    clusters: {
      id: "clusters",
      to: "/ui/provisioning/clusters",
      menu: ["", ""],
    },
    cluster_templates: {
      id: "cluster_templates",
      to: "/ui/provisioning/cluster-templates",
      menu: ["", ""],
    },
    servers: {
      id: "servers",
      to: "/ui/provisioning/servers",
      menu: ["", ""],
    },
    tokens: {
      id: "tokens",
      to: "/ui/provisioning/tokens",
      menu: ["", ""],
    },
    updates: {
      id: "updates",
      to: "/ui/provisioning/updates",
      menu: ["", ""],
    },
    settings: {
      id: "settings",
      to: "/ui/settings",
      menu: ["", ""],
    },
  };

  const getActiveMenuItem = (items: Record<string, MenuItem>): string => {
    const item = Object.values(items).find((item) =>
      pathname.startsWith(item?.to ?? ""),
    );
    return item?.id ?? "";
  };

  const activeMenuItem = getActiveMenuItem(menuItems);

  const isItemActive = (name: string): boolean => {
    return activeMenuItem == name;
  };

  useEffect(() => {
    setOpenSubmenu(menuItems[activeMenuItem]?.menu ?? ["", ""]);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <>
      <Navbar bg="dark" variant="dark" className="d-flex flex-column vh-100">
        <Navbar.Brand href="/ui/" style={{ margin: "5px 15px" }}>
          Operations Center
        </Navbar.Brand>

        <Nav className="flex-column w-100 flex-grow-1 overflow-auto">
          {isAuthenticated && (
            <>
              <Nav.Item>
                <NavItemLink onClick={() => toggleSubmenu("inventory", 0)}>
                  <MdOutlineInventory2 /> Inventory{" "}
                  {menuFoldIcon("inventory", 0)}
                </NavItemLink>
                <Collapse in={openSubmenu[0] === "inventory"}>
                  <div>
                    <Nav className="flex-column ms-2">
                      <NavItemLink
                        item={menuItems["images"]}
                        isActive={isItemActive("images")}
                      >
                        <RiHardDrive2Line /> Images
                      </NavItemLink>
                      <NavItemLink
                        item={menuItems["instances"]}
                        isActive={isItemActive("instances")}
                      >
                        <RiBox3Line /> Instances
                      </NavItemLink>
                      <Nav.Item>
                        <NavItemLink
                          onClick={() => toggleSubmenu("networking", 1)}
                        >
                          <RiOrganizationChart /> Networking
                          {menuFoldIcon("networking", 1)}
                        </NavItemLink>
                        <Collapse in={openSubmenu[1] === "networking"}>
                          <div>
                            <Nav className="flex-column ms-2">
                              <NavItemLink
                                item={menuItems["networks"]}
                                isActive={isItemActive("networks")}
                              >
                                Networks
                              </NavItemLink>
                              <NavItemLink
                                item={menuItems["network_acls"]}
                                isActive={isItemActive("network_acls")}
                              >
                                ACLs
                              </NavItemLink>
                              <NavItemLink
                                item={menuItems["network_forwards"]}
                                isActive={isItemActive("network_forwards")}
                              >
                                Forwards
                              </NavItemLink>
                              <NavItemLink
                                item={menuItems["network_integrations"]}
                                isActive={isItemActive("network_integrations")}
                              >
                                Integrations
                              </NavItemLink>
                              <NavItemLink
                                item={menuItems["network_load_balancers"]}
                                isActive={isItemActive(
                                  "network_load_balancers",
                                )}
                              >
                                Load Balancers
                              </NavItemLink>
                              <NavItemLink
                                item={menuItems["network_peers"]}
                                isActive={isItemActive("network_peers")}
                              >
                                Peers
                              </NavItemLink>
                              <NavItemLink
                                item={menuItems["network_zones"]}
                                isActive={isItemActive("network_zones")}
                              >
                                Zones
                              </NavItemLink>
                            </Nav>
                          </div>
                        </Collapse>
                      </Nav.Item>
                      <Nav.Item>
                        <NavItemLink
                          onClick={() => toggleSubmenu("storage", 1)}
                        >
                          <RiDatabase2Line /> Storage
                          {menuFoldIcon("storage", 1)}
                        </NavItemLink>
                        <Collapse in={openSubmenu[1] === "storage"}>
                          <div>
                            <Nav className="flex-column ms-2">
                              <NavItemLink
                                item={menuItems["storage_buckets"]}
                                isActive={isItemActive("storage_buckets")}
                              >
                                Buckets
                              </NavItemLink>
                              <NavItemLink
                                item={menuItems["storage_pools"]}
                                isActive={isItemActive("storage_pools")}
                              >
                                Pools
                              </NavItemLink>
                              <NavItemLink
                                item={menuItems["storage_volumes"]}
                                isActive={isItemActive("storage_volumes")}
                              >
                                Volumes
                              </NavItemLink>
                            </Nav>
                          </div>
                        </Collapse>
                      </Nav.Item>
                      <NavItemLink
                        item={menuItems["profiles"]}
                        isActive={isItemActive("profiles")}
                      >
                        <RiPassPendingLine /> Profiles
                      </NavItemLink>
                      <NavItemLink
                        item={menuItems["projects"]}
                        isActive={isItemActive("projects")}
                      >
                        <RiArticleLine /> Projects
                      </NavItemLink>
                    </Nav>
                  </div>
                </Collapse>
              </Nav.Item>
              <Nav.Item>
                <NavItemLink
                  item={menuItems["clusters"]}
                  isActive={isItemActive("clusters")}
                >
                  <AiOutlineCluster /> Clusters
                </NavItemLink>
              </Nav.Item>
              <Nav.Item>
                <NavItemLink
                  item={menuItems["cluster_templates"]}
                  isActive={isItemActive("cluster_templates")}
                >
                  <GoProjectTemplate /> Cluster templates
                </NavItemLink>
              </Nav.Item>
              <Nav.Item>
                <NavItemLink
                  item={menuItems["servers"]}
                  isActive={isItemActive("servers")}
                >
                  <GoServer /> Servers
                </NavItemLink>
              </Nav.Item>
              <Nav.Item>
                <NavItemLink
                  item={menuItems["tokens"]}
                  isActive={isItemActive("tokens")}
                >
                  <MdOutlineToken /> Tokens
                </NavItemLink>
              </Nav.Item>
              <Nav.Item>
                <NavItemLink
                  item={menuItems["updates"]}
                  isActive={isItemActive("updates")}
                >
                  <MdOutlineSystemUpdateAlt /> Updates
                </NavItemLink>
              </Nav.Item>
            </>
          )}
          {!isAuthenticated && (
            <>
              <NavItemLink href="/oidc/login">
                <MdLogin /> Login
              </NavItemLink>
            </>
          )}
        </Nav>
        <Nav className="flex-column w-100 flex-shrink-0 border-top border-secondary pt-2">
          {isAuthenticated && (
            <>
              <NavItemLink
                item={menuItems["settings"]}
                isActive={isItemActive("settings")}
              >
                <MdOutlineSettings /> Settings
              </NavItemLink>
              <NavItemLink
                onClick={() => {
                  logout();
                }}
              >
                <MdLogout /> Logout
              </NavItemLink>
            </>
          )}
        </Nav>
      </Navbar>
    </>
  );
};

export default Sidebar;
