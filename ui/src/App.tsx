import { Routes, Route } from "react-router";
import { Container } from "react-bootstrap";
import Sidebar from "components/Sidebar";
import Notification from "components/Notification";
import { useAuth } from "context/authContext";
import Home from "pages/Home";
import Image from "pages/Image";
import Instance from "pages/Instance";
import Network from "pages/Network";
import NetworkACL from "pages/NetworkACL";
import NetworkForward from "pages/NetworkForward";
import NetworkIntegration from "pages/NetworkIntegration";
import NetworkLoadBalancer from "pages/NetworkLoadBalancer";
import NetworkPeer from "pages/NetworkPeer";
import NetworkZone from "pages/NetworkZone";
import StorageBucket from "pages/StorageBucket";
import StoragePool from "pages/StoragePool";
import StorageVolume from "pages/StorageVolume";
import Profile from "pages/Profile";
import Project from "pages/Project";
import Server from "pages/Server";
import ServerDetail from "pages/ServerDetail";
import Token from "pages/Token";
import TokenCreate from "pages/TokenCreate";
import TokenDetail from "pages/TokenDetail";
import Update from "pages/Update";
import UpdateDetail from "pages/UpdateDetail";

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
            <Route path="/ui/inventory/images" element={<Image />} />
            <Route path="/ui/inventory/instances" element={<Instance />} />
            <Route path="/ui/inventory/networks" element={<Network />} />
            <Route path="/ui/inventory/network_acls" element={<NetworkACL />} />
            <Route
              path="/ui/inventory/network_forwards"
              element={<NetworkForward />}
            />
            <Route
              path="/ui/inventory/network_integrations"
              element={<NetworkIntegration />}
            />
            <Route
              path="/ui/inventory/network_load_balancers"
              element={<NetworkLoadBalancer />}
            />
            <Route
              path="/ui/inventory/network_peers"
              element={<NetworkPeer />}
            />
            <Route
              path="/ui/inventory/network_zones"
              element={<NetworkZone />}
            />
            <Route
              path="/ui/inventory/storage_buckets"
              element={<StorageBucket />}
            />
            <Route
              path="/ui/inventory/storage_pools"
              element={<StoragePool />}
            />
            <Route
              path="/ui/inventory/storage_volumes"
              element={<StorageVolume />}
            />
            <Route path="/ui/inventory/profiles" element={<Profile />} />
            <Route path="/ui/inventory/projects" element={<Project />} />
            <Route path="/ui/provisioning/servers" element={<Server />} />
            <Route
              path="/ui/provisioning/servers/:name"
              element={<ServerDetail />}
            />
            <Route
              path="/ui/provisioning/servers/:name/:activeTab"
              element={<ServerDetail />}
            />
            <Route path="/ui/provisioning/tokens" element={<Token />} />
            <Route
              path="/ui/provisioning/tokens/create"
              element={<TokenCreate />}
            />
            <Route
              path="/ui/provisioning/tokens/:uuid"
              element={<TokenDetail />}
            />
            <Route
              path="/ui/provisioning/tokens/:uuid/:activeTab"
              element={<TokenDetail />}
            />
            <Route path="/ui/provisioning/updates" element={<Update />} />
            <Route
              path="/ui/provisioning/updates/:uuid"
              element={<UpdateDetail />}
            />
            <Route
              path="/ui/provisioning/updates/:uuid/:activeTab"
              element={<UpdateDetail />}
            />
          </Routes>
          <Notification />
        </Container>
      </div>
    </>
  );
}

export default App;
