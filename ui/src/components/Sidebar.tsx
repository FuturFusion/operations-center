import { Nav, Navbar, Container } from "react-bootstrap";
import { Link } from "react-router";
import { MdLogin, MdLogout } from "react-icons/md";
import { useAuth } from "context/auth";

const Sidebar = () => {
  const { isAuthenticated } = useAuth();

  const logout = () => {
    fetch("/oidc/logout").then(() => {
      window.location.href = "/ui/";
    });
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
                <li>
                  <Nav.Link as={Link} to="/ui/inventory/profiles">
                    Profiles
                  </Nav.Link>
                </li>
                <li>
                  <Nav.Link as={Link} to="/ui/inventory/projects">
                    Projects
                  </Nav.Link>
                </li>
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
