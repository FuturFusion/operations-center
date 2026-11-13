import { useEffect } from "react";
import { Routes, Route, useLocation } from "react-router";
import { Container } from "react-bootstrap";
import ErrorBoundary from "components/ErrorBoundary";
import Sidebar from "components/Sidebar";
import Notification from "components/Notification";
import { useAuth } from "context/authContext";
import { routes } from "util/routes";

function App() {
  const { isAuthenticated, isAuthLoading, authError } = useAuth();
  const { pathname } = useLocation();

  useEffect(() => {
    if (
      !isAuthLoading &&
      !isAuthenticated &&
      !authError &&
      window.location.pathname !== "/ui/"
    ) {
      window.location.href = "/ui/";
    }
  }, [isAuthLoading, isAuthenticated, authError]);

  if (isAuthLoading) {
    return <div>Loading...</div>;
  }

  // Not reaching the server says nothing about the authentication of the user.
  if (authError) {
    return (
      <div className="p-3">
        Error while connecting to Operations Center: {authError.message}
      </div>
    );
  }

  return (
    <>
      <div style={{ display: "flex" }}>
        <ErrorBoundary label="the navigation">
          <Sidebar />
        </ErrorBoundary>
        <Container
          fluid
          style={{
            paddingLeft: "30px",
            paddingTop: "30px",
            transition: "padding-left 0.3s",
          }}
        >
          {/* Keyed by path, so navigating away from a failed page recovers. */}
          <ErrorBoundary key={pathname} label="the page">
            <Routes>
              {routes.map((r) => (
                <Route key={r.path} path={r.path} element={<r.component />} />
              ))}
            </Routes>
          </ErrorBoundary>
          <Notification />
        </Container>
      </div>
    </>
  );
}

export default App;
