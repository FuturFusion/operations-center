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
import Settings from "pages/Settings";
import StorageBucket from "pages/StorageBucket";
import StoragePool from "pages/StoragePool";
import StorageVolume from "pages/StorageVolume";
import Profile from "pages/Profile";
import Project from "pages/Project";
import Cluster from "pages/Cluster";
import ClusterArtifactFiles from "pages/ClusterArtifactFiles";
import ClusterCreate from "pages/ClusterCreate";
import ClusterDetail from "pages/ClusterDetail";
import ClusterTemplateCreate from "pages/ClusterTemplateCreate";
import ClusterTemplateDetail from "pages/ClusterTemplateDetail";
import Server from "pages/Server";
import ServerDetail from "pages/ServerDetail";
import TokenCreate from "pages/TokenCreate";
import TokenDetail from "pages/TokenDetail";
import TokenSeedCreate from "pages/TokenSeedCreate";
import TokenSeedDetail from "pages/TokenSeedDetail";
import Update from "pages/Update";
import UpdateDetail from "pages/UpdateDetail";
import ChannelCreate from "pages/ChannelCreate";
import ChannelDetail from "pages/ChannelDetail";
import Warning from "pages/Warning";
import WarningDetail from "pages/WarningDetail";

export const routes = [
  { path: "/ui", component: Home },

  { path: "/ui/inventory/images", component: Image },
  { path: "/ui/inventory/instances", component: Instance },
  { path: "/ui/inventory/networks", component: Network },
  { path: "/ui/inventory/network_acls", component: NetworkACL },
  { path: "/ui/inventory/network_forwards", component: NetworkForward },
  { path: "/ui/inventory/network_integrations", component: NetworkIntegration },
  {
    path: "/ui/inventory/network_load_balancers",
    component: NetworkLoadBalancer,
  },
  { path: "/ui/inventory/network_peers", component: NetworkPeer },
  { path: "/ui/inventory/network_zones", component: NetworkZone },
  { path: "/ui/inventory/storage_buckets", component: StorageBucket },
  { path: "/ui/inventory/storage_pools", component: StoragePool },
  { path: "/ui/inventory/storage_volumes", component: StorageVolume },
  { path: "/ui/inventory/profiles", component: Profile },
  { path: "/ui/inventory/projects", component: Project },

  { path: "/ui/provisioning/clusters-view", component: Cluster },
  { path: "/ui/provisioning/clusters-view/:activeTab", component: Cluster },

  {
    path: "/ui/provisioning/clusters/:clusterName/artifacts/:artifactName/files",
    component: ClusterArtifactFiles,
  },

  { path: "/ui/provisioning/clusters/create", component: ClusterCreate },
  {
    path: "/ui/provisioning/clusters/create/:activeTab",
    component: ClusterCreate,
  },

  { path: "/ui/provisioning/clusters/:name", component: ClusterDetail },
  {
    path: "/ui/provisioning/clusters/:name/:activeTab",
    component: ClusterDetail,
  },

  {
    path: "/ui/provisioning/cluster-templates/create",
    component: ClusterTemplateCreate,
  },
  {
    path: "/ui/provisioning/cluster-templates/:name",
    component: ClusterTemplateDetail,
  },
  {
    path: "/ui/provisioning/cluster-templates/:name/:activeTab",
    component: ClusterTemplateDetail,
  },

  { path: "/ui/provisioning/servers-view", component: Server },
  { path: "/ui/provisioning/servers-view/:activeTab", component: Server },

  { path: "/ui/provisioning/servers/:name", component: ServerDetail },
  {
    path: "/ui/provisioning/servers/:name/:activeTab",
    component: ServerDetail,
  },

  { path: "/ui/settings", component: Settings },
  { path: "/ui/settings/:activeTab", component: Settings },

  { path: "/ui/provisioning/tokens/create", component: TokenCreate },
  { path: "/ui/provisioning/tokens/:uuid", component: TokenDetail },
  { path: "/ui/provisioning/tokens/:uuid/:activeTab", component: TokenDetail },

  {
    path: "/ui/provisioning/tokens/:uuid/seeds/create",
    component: TokenSeedCreate,
  },
  {
    path: "/ui/provisioning/tokens/:uuid/seeds/:name",
    component: TokenSeedDetail,
  },
  {
    path: "/ui/provisioning/tokens/:uuid/seeds/:name/:activeTab",
    component: TokenSeedDetail,
  },

  { path: "/ui/provisioning/updates-view", component: Update },
  { path: "/ui/provisioning/updates-view/:activeTab", component: Update },

  { path: "/ui/provisioning/updates/:uuid", component: UpdateDetail },
  {
    path: "/ui/provisioning/updates/:uuid/:activeTab",
    component: UpdateDetail,
  },

  { path: "/ui/provisioning/channels/create", component: ChannelCreate },
  { path: "/ui/provisioning/channels/:name", component: ChannelDetail },
  {
    path: "/ui/provisioning/channels/:name/:activeTab",
    component: ChannelDetail,
  },
  { path: "/ui/provisioning/warnings", component: Warning },
  { path: "/ui/provisioning/warnings/:uuid", component: WarningDetail },
  {
    path: "/ui/provisioning/warnings/:uuid/:activeTab",
    component: WarningDetail,
  },
];
