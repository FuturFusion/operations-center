import { Routes, Route } from "react-router";
import { Container } from "react-bootstrap";
import Sidebar from "components/Sidebar";
import { useAuth } from "context/auth";
import Home from "pages/Home";
import Profile from "pages/Profile";
import Project from "pages/Project";

function App() {
  const { isAuthenticated, isAuthLoading } = useAuth();

  if (isAuthLoading) {
    return <div>Loading...</div>;
  }

  if (!isAuthenticated) {
    if (window.location.pathname !== "/ui/") {
      window.location.href = "/ui/";
    }
  }

  return (
    <>
      <div style={{ display: "flex" }}>
        <Sidebar />
        <Container
          fluid
          style={{
            paddingLeft: "30px",
            paddingTop: "30px",
            transition: "padding-left 0.3s",
          }}
        >
          <Routes>
            <Route path="/ui" element={<Home />} />
            <Route path="/ui/inventory/profiles" element={<Profile />} />
            <Route path="/ui/inventory/projects" element={<Project />} />
          </Routes>
        </Container>
      </div>
    </>
  );
}

export default App;
