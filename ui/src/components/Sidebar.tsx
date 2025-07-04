import { useState } from "react";
import { Nav, Navbar, Collapse, Container } from "react-bootstrap";
import { Link } from "react-router";
import { GoServer } from "react-icons/go";
import {
  MdLogin,
  MdLogout,
  MdOutlineInventory2,
  MdOutlineToken,
  MdSettings,
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

  return (
    <>
      {/* Sidebar Navbar */}
      <Navbar bg="dark" variant="dark" className="flex-column vh-100">
        <Navbar.Brand href="/ui/" style={{ margin: "5px 15px" }}>
          Operations Center
        </Navbar.Brand>

        {/* Sidebar content */}
        <Container className="flex-column" style={{ padding: "0px" }}>
          <Nav className="flex-column w-100">
            {isAuthenticated && (
              <>
                <Nav.Item>
                  <li>
                    <Nav.Link onClick={() => toggleSubmenu("inventory", 0)}>
                      <MdOutlineInventory2 /> Inventory
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
                            </Nav.Link>
                          </li>
                          <Collapse in={openSubmenu[1] === "networking"}>
                            <div>
                              <Nav className="flex-column ms-2">
                                <li>
                                  <Nav.Link
                                    as={Link}
                                    to="/ui/inventory/networks"
                                  >
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
                            <Nav.Link
                              onClick={() => toggleSubmenu("storage", 1)}
                            >
                              <RiDatabase2Line /> Storage
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
                    <Nav.Link onClick={() => toggleSubmenu("provisioning", 0)}>
                      <MdSettings /> Provisioning
                    </Nav.Link>
                  </li>
                  <Collapse in={openSubmenu[0] === "provisioning"}>
                    <div>
                      <Nav className="flex-column ms-2">
                        <li>
                          <Nav.Link as={Link} to="/ui/provisioning/servers">
                            <GoServer /> Servers
                          </Nav.Link>
                        </li>
                        <li>
                          <Nav.Link as={Link} to="/ui/provisioning/tokens">
                            <MdOutlineToken /> Tokens
                          </Nav.Link>
                        </li>
                      </Nav>
                    </div>
                  </Collapse>
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
          <div
            className="w-100"
            style={{ position: "absolute", bottom: "20px" }}
          >
            <Nav className="flex-column">
              {isAuthenticated && (
                <>
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
          </div>
        </Container>
      </Navbar>
    </>
  );
};

export default Sidebar;
