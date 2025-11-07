import { useState } from "react";
import { Nav, Navbar, Collapse } from "react-bootstrap";
import { Link } from "react-router";
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
import { useAuth } from "context/authContext";

const Sidebar = () => {
  const { isAuthenticated } = useAuth();

  const logout = () => {
    fetch("/oidc/logout").then(() => {
      window.location.href = "/ui/";
    });
  };

  const [openSubmenu, setOpenSubmenu] = useState(["", ""]);

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

  return (
    <>
      {/* Sidebar Navbar */}
      <Navbar bg="dark" variant="dark" className="d-flex flex-column vh-100">
        <Navbar.Brand href="/ui/" style={{ margin: "5px 15px" }}>
          Operations Center
        </Navbar.Brand>

        {/* Sidebar content */}
        <Nav className="flex-column w-100 flex-grow-1 overflow-auto">
          {isAuthenticated && (
            <>
              <Nav.Item>
                <li>
                  <Nav.Link onClick={() => toggleSubmenu("inventory", 0)}>
                    <MdOutlineInventory2 /> Inventory{" "}
                    {menuFoldIcon("inventory", 0)}
                  </Nav.Link>
                </li>
                <Collapse in={openSubmenu[0] === "inventory"}>
                  <div>
                    <Nav className="flex-column ms-2">
                      <li>
                        <Nav.Link as={Link} to="/ui/inventory/images">
                          <RiHardDrive2Line /> Images
                        </Nav.Link>
                      </li>
                      <li>
                        <Nav.Link as={Link} to="/ui/inventory/instances">
                          <RiBox3Line /> Instances
                        </Nav.Link>
                      </li>
                      <Nav.Item>
                        <li>
                          <Nav.Link
                            onClick={() => toggleSubmenu("networking", 1)}
                          >
                            <RiOrganizationChart /> Networking
                            {menuFoldIcon("networking", 1)}
                          </Nav.Link>
                        </li>
                        <Collapse in={openSubmenu[1] === "networking"}>
                          <div>
                            <Nav className="flex-column ms-2">
                              <li>
                                <Nav.Link as={Link} to="/ui/inventory/networks">
                                  Networks
                                </Nav.Link>
                              </li>
                              <li>
                                <Nav.Link
                                  as={Link}
                                  to="/ui/inventory/network_acls"
                                >
                                  ACLs
                                </Nav.Link>
                              </li>
                              <li>
                                <Nav.Link
                                  as={Link}
                                  to="/ui/inventory/network_forwards"
                                >
                                  Forwards
                                </Nav.Link>
                              </li>
                              <li>
                                <Nav.Link
                                  as={Link}
                                  to="/ui/inventory/network_integrations"
                                >
                                  Integrations
                                </Nav.Link>
                              </li>
                              <li>
                                <Nav.Link
                                  as={Link}
                                  to="/ui/inventory/network_load_balancers"
                                >
                                  Load Balancers
                                </Nav.Link>
                              </li>
                              <li>
                                <Nav.Link
                                  as={Link}
                                  to="/ui/inventory/network_peers"
                                >
                                  Peers
                                </Nav.Link>
                              </li>
                              <li>
                                <Nav.Link
                                  as={Link}
                                  to="/ui/inventory/network_zones"
                                >
                                  Zones
                                </Nav.Link>
                              </li>
                            </Nav>
                          </div>
                        </Collapse>
                      </Nav.Item>
                      <Nav.Item>
                        <li>
                          <Nav.Link onClick={() => toggleSubmenu("storage", 1)}>
                            <RiDatabase2Line /> Storage
                            {menuFoldIcon("storage", 1)}
                          </Nav.Link>
                        </li>
                        <Collapse in={openSubmenu[1] === "storage"}>
                          <div>
                            <Nav className="flex-column ms-2">
                              <li>
                                <Nav.Link
                                  as={Link}
                                  to="/ui/inventory/storage_buckets"
                                >
                                  Buckets
                                </Nav.Link>
                              </li>
                              <li>
                                <Nav.Link
                                  as={Link}
                                  to="/ui/inventory/storage_pools"
                                >
                                  Pools
                                </Nav.Link>
                              </li>
                              <li>
                                <Nav.Link
                                  as={Link}
                                  to="/ui/inventory/storage_volumes"
                                >
                                  Volumes
                                </Nav.Link>
                              </li>
                            </Nav>
                          </div>
                        </Collapse>
                      </Nav.Item>
                      <li>
                        <Nav.Link as={Link} to="/ui/inventory/profiles">
                          <RiPassPendingLine /> Profiles
                        </Nav.Link>
                      </li>
                      <li>
                        <Nav.Link as={Link} to="/ui/inventory/projects">
                          <RiArticleLine /> Projects
                        </Nav.Link>
                      </li>
                    </Nav>
                  </div>
                </Collapse>
              </Nav.Item>
              <Nav.Item>
                <li>
                  <Nav.Link as={Link} to="/ui/provisioning/clusters">
                    <AiOutlineCluster /> Clusters
                  </Nav.Link>
                </li>
              </Nav.Item>
              <Nav.Item>
                <li>
                  <Nav.Link as={Link} to="/ui/provisioning/cluster-templates">
                    <GoProjectTemplate /> Cluster templates
                  </Nav.Link>
                </li>
              </Nav.Item>
              <Nav.Item>
                <li>
                  <Nav.Link as={Link} to="/ui/provisioning/servers">
                    <GoServer /> Servers
                  </Nav.Link>
                </li>
              </Nav.Item>
              <Nav.Item>
                <li>
                  <Nav.Link as={Link} to="/ui/provisioning/tokens">
                    <MdOutlineToken /> Tokens
                  </Nav.Link>
                </li>
              </Nav.Item>
              <Nav.Item>
                <li>
                  <Nav.Link as={Link} to="/ui/provisioning/updates">
                    <MdOutlineSystemUpdateAlt /> Updates
                  </Nav.Link>
                </li>
              </Nav.Item>
            </>
          )}
          {!isAuthenticated && (
            <>
              <li>
                <Nav.Link href="/oidc/login">
                  <MdLogin /> Login
                </Nav.Link>
              </li>
            </>
          )}
        </Nav>
        {/* Bottom Element */}
        <Nav className="flex-column w-100 flex-shrink-0 border-top border-secondary pt-2">
          {isAuthenticated && (
            <>
              <li>
                <Nav.Link as={Link} to="/ui/settings">
                  <MdOutlineSettings /> Settings
                </Nav.Link>
              </li>
              <li>
                <Nav.Link
                  onClick={() => {
                    logout();
                  }}
                >
                  <MdLogout /> Logout
                </Nav.Link>
              </li>
            </>
          )}
        </Nav>
      </Navbar>
    </>
  );
};

export default Sidebar;
