CREATE TABLE IF NOT EXISTS schema (
  id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
  version INTEGER NOT NULL,
  updated_at DATETIME NOT NULL,
  UNIQUE (version)
);

CREATE TABLE tokens (
  id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
  uuid TEXT NOT NULL,
  uses_remaining INTEGER NOT NULL,
  expire_at DATETIME NOT NULL,
  description TEXT NOT NULL,
  UNIQUE(uuid)
);

CREATE TABLE clusters (
  id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
  name TEXT NOT NULL,
  connection_url TEXT NOT NULL,
  certificate TEXT NOT NULL,
  status TEXT NOT NULL,
  last_updated DATETIME NOT NULL,
  UNIQUE (name),
  UNIQUE (certificate)
);

CREATE TABLE servers (
  id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
  cluster_id INTEGER,
  name TEXT NOT NULL,
  type TEXT NOT NULL,
  connection_url TEXT NOT NULL,
  certificate TEXT NOT NULL,
  status TEXT NOT NULL,
  hardware_data TEXT NOT NULL,
  os_data TEXT NOT NULL,
  last_updated DATETIME NOT NULL,
  last_seen DATETIME NOT NULL DEFAULT '0000-01-01 00:00:00.0+00:00',
  UNIQUE (name),
  UNIQUE (certificate),
  FOREIGN KEY (cluster_id) REFERENCES clusters(id)
);

CREATE TABLE updates (
  id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
  uuid TEXT NOT NULL,
  origin TEXT NOT NULL,
  external_id TEXT NOT NULL,
  "version" TEXT NOT NULL,
  published_at DATETIME NOT NULL,
  severity TEXT NOT NULL,
  channel TEXT NOT NULL,
  changelog TEXT NOT NULL,
  files TEXT NOT NULL,
  UNIQUE(uuid)
);

CREATE TABLE images (
  id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
  uuid TEXT NOT NULL,
  cluster_id INTEGER NOT NULL,
  project_name TEXT NOT NULL,
  name TEXT NOT NULL,
  object TEXT NOT NULL,
  last_updated DATETIME NOT NULL,
  UNIQUE (uuid),
  UNIQUE (cluster_id, project_name, name),
  FOREIGN KEY (cluster_id) REFERENCES clusters(id)
);

CREATE TABLE instances (
  id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
  uuid TEXT NOT NULL,
  cluster_id INTEGER NOT NULL,
  server_id INTEGER NOT NULL,
  project_name TEXT NOT NULL,
  name TEXT NOT NULL,
  object TEXT NOT NULL,
  last_updated DATETIME NOT NULL,
  UNIQUE (uuid),
  UNIQUE (cluster_id, server_id, project_name, name),
  FOREIGN KEY (cluster_id) REFERENCES clusters(id),
  FOREIGN KEY (server_id) REFERENCES servers(id)
);

CREATE TABLE networks (
  id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
  uuid TEXT NOT NULL,
  cluster_id INTEGER NOT NULL,
  project_name TEXT NOT NULL,
  name TEXT NOT NULL,
  object TEXT NOT NULL,
  last_updated DATETIME NOT NULL,
  UNIQUE (uuid),
  UNIQUE (cluster_id, project_name, name),
  FOREIGN KEY (cluster_id) REFERENCES clusters(id)
);

CREATE TABLE network_acls (
  id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
  uuid TEXT NOT NULL,
  cluster_id INTEGER NOT NULL,
  project_name TEXT NOT NULL,
  name TEXT NOT NULL,
  object TEXT NOT NULL,
  last_updated DATETIME NOT NULL,
  UNIQUE (uuid),
  UNIQUE (cluster_id, project_name, name),
  FOREIGN KEY (cluster_id) REFERENCES clusters(id)
);

CREATE TABLE network_address_sets (
  id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
  uuid TEXT NOT NULL,
  cluster_id INTEGER NOT NULL,
  project_name TEXT NOT NULL,
  name TEXT NOT NULL,
  object TEXT NOT NULL,
  last_updated DATETIME NOT NULL,
  UNIQUE (uuid),
  UNIQUE (cluster_id, project_name, name),
  FOREIGN KEY (cluster_id) REFERENCES clusters(id)
);

CREATE TABLE network_forwards (
  id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
  uuid TEXT NOT NULL,
  cluster_id INTEGER NOT NULL,
  network_name TEXT NOT NULL,
  name TEXT NOT NULL,
  object TEXT NOT NULL,
  last_updated DATETIME NOT NULL,
  UNIQUE (uuid),
  UNIQUE (cluster_id, network_name, name),
  FOREIGN KEY (cluster_id) REFERENCES clusters(id)
);

CREATE TABLE network_integrations (
  id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
  uuid TEXT NOT NULL,
  cluster_id INTEGER NOT NULL,
  name TEXT NOT NULL,
  object TEXT NOT NULL,
  last_updated DATETIME NOT NULL,
  UNIQUE (uuid),
  UNIQUE (cluster_id, name),
  FOREIGN KEY (cluster_id) REFERENCES clusters(id)
);

CREATE TABLE network_load_balancers (
  id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
  uuid TEXT NOT NULL,
  cluster_id INTEGER NOT NULL,
  network_name TEXT NOT NULL,
  name TEXT NOT NULL,
  object TEXT NOT NULL,
  last_updated DATETIME NOT NULL,
  UNIQUE (uuid),
  UNIQUE (cluster_id, network_name, name),
  FOREIGN KEY (cluster_id) REFERENCES clusters(id)
);

CREATE TABLE network_peers (
  id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
  uuid TEXT NOT NULL,
  cluster_id INTEGER NOT NULL,
  network_name TEXT NOT NULL,
  name TEXT NOT NULL,
  object TEXT NOT NULL,
  last_updated DATETIME NOT NULL,
  UNIQUE (uuid),
  UNIQUE (cluster_id, network_name, name),
  FOREIGN KEY (cluster_id) REFERENCES clusters(id)
);

CREATE TABLE network_zones (
  id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
  uuid TEXT NOT NULL,
  cluster_id INTEGER NOT NULL,
  project_name TEXT NOT NULL,
  name TEXT NOT NULL,
  object TEXT NOT NULL,
  last_updated DATETIME NOT NULL,
  UNIQUE (uuid),
  UNIQUE (cluster_id, project_name, name),
  FOREIGN KEY (cluster_id) REFERENCES clusters(id)
);

CREATE TABLE profiles (
  id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
  uuid TEXT NOT NULL,
  cluster_id INTEGER NOT NULL,
  project_name TEXT NOT NULL,
  name TEXT NOT NULL,
  object TEXT NOT NULL,
  last_updated DATETIME NOT NULL,
  UNIQUE (uuid),
  UNIQUE (cluster_id, project_name, name),
  FOREIGN KEY (cluster_id) REFERENCES clusters(id)
);

CREATE TABLE projects (
  id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
  uuid TEXT NOT NULL,
  cluster_id INTEGER NOT NULL,
  name TEXT NOT NULL,
  object TEXT NOT NULL,
  last_updated DATETIME NOT NULL,
  UNIQUE (uuid),
  UNIQUE (cluster_id, name),
  FOREIGN KEY (cluster_id) REFERENCES clusters(id)
);

CREATE TABLE storage_buckets (
  id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
  uuid TEXT NOT NULL,
  cluster_id INTEGER NOT NULL,
  server_id INTEGER NOT NULL,
  project_name TEXT NOT NULL,
  storage_pool_name TEXT NOT NULL,
  name TEXT NOT NULL,
  object TEXT NOT NULL,
  last_updated DATETIME NOT NULL,
  UNIQUE (uuid),
  UNIQUE (cluster_id, server_id, project_name, storage_pool_name, name),
  FOREIGN KEY (cluster_id) REFERENCES clusters(id),
  FOREIGN KEY (server_id) REFERENCES servers(id)
);

CREATE TABLE storage_pools (
  id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
  uuid TEXT NOT NULL,
  cluster_id INTEGER NOT NULL,
  name TEXT NOT NULL,
  object TEXT NOT NULL,
  last_updated DATETIME NOT NULL,
  UNIQUE (uuid),
  UNIQUE (cluster_id, name),
  FOREIGN KEY (cluster_id) REFERENCES clusters(id)
);

CREATE TABLE storage_volumes (
  id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
  uuid TEXT NOT NULL,
  cluster_id INTEGER NOT NULL,
  server_id INTEGER,
  project_name TEXT NOT NULL,
  storage_pool_name TEXT NOT NULL,
  name TEXT NOT NULL,
  type TEXT NOT NULL,
  object TEXT NOT NULL,
  last_updated DATETIME NOT NULL,
  UNIQUE (uuid),
  UNIQUE (cluster_id, server_id, project_name, storage_pool_name, name, type),
  FOREIGN KEY (cluster_id) REFERENCES clusters(id),
  FOREIGN KEY (server_id) REFERENCES servers(id)
);

CREATE VIEW resources AS
    SELECT 'image' AS kind, images.id, clusters.name AS cluster_name, NULL AS server_name, images.project_name, NULL AS parent_name, images.name, images.object, images.last_updated
    FROM images
    INNER JOIN clusters ON images.cluster_id = clusters.id
  UNION
    SELECT 'instance' AS kind, instances.id, clusters.name AS cluster_name, servers.name AS server_name, instances.project_name, NULL AS parent_name, instances.name, instances.object, instances.last_updated
    FROM instances
    INNER JOIN clusters ON instances.cluster_id = clusters.id
    LEFT JOIN servers ON instances.server_id = servers.id
  UNION
    SELECT 'network' AS kind, networks.id, clusters.name AS cluster_name, NULL AS server_name, networks.project_name, NULL AS parent_name, networks.name, networks.object, networks.last_updated
    FROM networks
    INNER JOIN clusters ON networks.cluster_id = clusters.id
  UNION
    SELECT 'network_acl' AS kind, network_acls.id, clusters.name AS cluster_name, NULL AS server_name, network_acls.project_name, NULL AS parent_name, network_acls.name, network_acls.object, network_acls.last_updated
    FROM network_acls
    INNER JOIN clusters ON network_acls.cluster_id = clusters.id
  UNION
    SELECT 'network_forward' AS kind, network_forwards.id, clusters.name AS cluster_name, NULL AS server_name, networks.project_name, network_forwards.network_name AS parent_name, network_forwards.name, network_forwards.object, network_forwards.last_updated
    FROM network_forwards
    INNER JOIN clusters ON network_forwards.cluster_id = clusters.id
    LEFT JOIN networks ON network_forwards.network_name = networks.name
  UNION
    SELECT 'network_integration' AS kind, network_integrations.id, clusters.name AS cluster_name, NULL AS server_name, NULL AS project_name, NULL AS parent_name, network_integrations.name, network_integrations.object, network_integrations.last_updated
    FROM network_integrations
    INNER JOIN clusters ON network_integrations.cluster_id = clusters.id
  UNION
    SELECT 'network_load_balancer' AS kind, network_load_balancers.id, clusters.name AS cluster_name, NULL AS server_name, networks.project_name, network_load_balancers.network_name AS parent_name, network_load_balancers.name, network_load_balancers.object, network_load_balancers.last_updated
    FROM network_load_balancers
    INNER JOIN clusters ON network_load_balancers.cluster_id = clusters.id
    LEFT JOIN networks ON network_load_balancers.network_name = networks.name
  UNION
    SELECT 'network_peer' AS kind, network_peers.id, clusters.name AS cluster_name, NULL AS server_name, networks.project_name, network_peers.network_name AS parent_name, network_peers.name, network_peers.object, network_peers.last_updated
    FROM network_peers
    INNER JOIN clusters ON network_peers.cluster_id = clusters.id
    LEFT JOIN networks ON network_peers.network_name = networks.name
  UNION
    SELECT 'network_zone' AS kind, network_zones.id, clusters.name AS cluster_name, NULL AS server_name, network_zones.project_name, NULL AS parent_name, network_zones.name, network_zones.object, network_zones.last_updated
    FROM network_zones
    INNER JOIN clusters ON network_zones.cluster_id = clusters.id
  UNION
    SELECT 'profile' AS kind, profiles.id, clusters.name AS cluster_name, NULL AS server_name, profiles.project_name, NULL AS parent_name, profiles.name, profiles.object, profiles.last_updated
    FROM profiles
    INNER JOIN clusters ON profiles.cluster_id = clusters.id
  UNION
    SELECT 'project' AS kind, projects.id, clusters.name AS cluster_name, NULL AS server_name, projects.name AS project_name, NULL AS parent_name, projects.name, projects.object, projects.last_updated
    FROM projects
    INNER JOIN clusters ON projects.cluster_id = clusters.id
  UNION
    SELECT 'storage_bucket' AS kind, storage_buckets.id, clusters.name AS cluster_name, servers.name AS server_name, storage_buckets.project_name, storage_buckets.storage_pool_name AS parent_name, storage_buckets.name, storage_buckets.object, storage_buckets.last_updated
    FROM storage_buckets
    INNER JOIN clusters ON storage_buckets.cluster_id = clusters.id
    LEFT JOIN servers ON storage_buckets.server_id = servers.id
  UNION
    SELECT 'storage_pool' AS kind, storage_pools.id, clusters.name AS cluster_name, NULL AS server_name, NULL AS project_name, NULL AS parent_name, storage_pools.name, storage_pools.object, storage_pools.last_updated
    FROM storage_pools
    INNER JOIN clusters ON storage_pools.cluster_id = clusters.id
  UNION
    SELECT 'storage_volume' AS kind, storage_volumes.id, clusters.name AS cluster_name, servers.name AS server_name, storage_volumes.project_name, storage_volumes.storage_pool_name AS parent_name, storage_volumes.type || "/" || storage_volumes.name AS name, storage_volumes.object, storage_volumes.last_updated
    FROM storage_volumes
    INNER JOIN clusters ON storage_volumes.cluster_id = clusters.id
    LEFT JOIN servers ON storage_volumes.server_id = servers.id
;

INSERT INTO schema (version, updated_at) VALUES (5, strftime("%s"))
