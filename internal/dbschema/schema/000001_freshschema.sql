CREATE TABLE IF NOT EXISTS schema (
  id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
  version INTEGER NOT NULL,
  updated_at DATETIME NOT NULL,
  UNIQUE (version)
);

CREATE TABLE IF NOT EXISTS tokens (
  uuid TEXT PRIMARY KEY NOT NULL,
  uses_remaining INTEGER NOT NULL,
  expire_at DATETIME NOT NULL,
  description TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS clusters (
  id INTEGER PRIMARY KEY NOT NULL,
  name TEXT NOT NULL,
  connection_url TEXT NOT NULL,
  last_updated DATETIME NOT NULL,
  UNIQUE (name)
);

CREATE TABLE IF NOT EXISTS servers (
  id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
  cluster_id INTEGER,
  name TEXT NOT NULL,
  type TEXT NOT NULL,
  connection_url TEXT NOT NULL,
  last_updated DATETIME NOT NULL,
  UNIQUE (name),
  FOREIGN KEY(cluster_id) REFERENCES clusters(id)
);

CREATE TABLE IF NOT EXISTS images (
  id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
  cluster_id INTEGER NOT NULL,
  project_name TEXT NOT NULL,
  name TEXT NOT NULL,
  object TEXT NOT NULL,
  last_updated DATETIME NOT NULL,
  UNIQUE (cluster_id, project_name, name),
  FOREIGN KEY (cluster_id) REFERENCES clusters(id)
);

CREATE TABLE IF NOT EXISTS instances (
  id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
  cluster_id INTEGER NOT NULL,
  server_id INTEGER NOT NULL,
  project_name TEXT NOT NULL,
  name TEXT NOT NULL,
  object TEXT NOT NULL,
  last_updated DATETIME NOT NULL,
  UNIQUE (cluster_id, server_id, project_name, name),
  FOREIGN KEY (cluster_id) REFERENCES clusters(id),
  FOREIGN KEY (server_id) REFERENCES servers(id)
);

CREATE TABLE IF NOT EXISTS networks (
  id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
  cluster_id INTEGER NOT NULL,
  project_name TEXT NOT NULL,
  name TEXT NOT NULL,
  object TEXT NOT NULL,
  last_updated DATETIME NOT NULL,
  UNIQUE (cluster_id, project_name, name),
  FOREIGN KEY (cluster_id) REFERENCES clusters(id)
);

CREATE TABLE IF NOT EXISTS network_acls (
  id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
  cluster_id INTEGER NOT NULL,
  project_name TEXT NOT NULL,
  name TEXT NOT NULL,
  object TEXT NOT NULL,
  last_updated DATETIME NOT NULL,
  UNIQUE (cluster_id, project_name, name),
  FOREIGN KEY (cluster_id) REFERENCES clusters(id)
);

CREATE TABLE IF NOT EXISTS network_forwards (
  id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
  cluster_id INTEGER NOT NULL,
  network_name TEXT NOT NULL,
  name TEXT NOT NULL,
  object TEXT NOT NULL,
  last_updated DATETIME NOT NULL,
  UNIQUE (cluster_id, network_name, name),
  FOREIGN KEY (cluster_id) REFERENCES clusters(id)
);

CREATE TABLE IF NOT EXISTS network_integrations (
  id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
  cluster_id INTEGER NOT NULL,
  name TEXT NOT NULL,
  object TEXT NOT NULL,
  last_updated DATETIME NOT NULL,
  UNIQUE (cluster_id, name),
  FOREIGN KEY (cluster_id) REFERENCES clusters(id)
);

CREATE TABLE IF NOT EXISTS network_load_balancers (
  id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
  cluster_id INTEGER NOT NULL,
  network_name TEXT NOT NULL,
  name TEXT NOT NULL,
  object TEXT NOT NULL,
  last_updated DATETIME NOT NULL,
  UNIQUE (cluster_id, network_name, name),
  FOREIGN KEY (cluster_id) REFERENCES clusters(id)
);

CREATE TABLE IF NOT EXISTS network_peers (
  id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
  cluster_id INTEGER NOT NULL,
  network_name TEXT NOT NULL,
  name TEXT NOT NULL,
  object TEXT NOT NULL,
  last_updated DATETIME NOT NULL,
  UNIQUE (cluster_id, network_name, name),
  FOREIGN KEY (cluster_id) REFERENCES clusters(id)
);

CREATE TABLE IF NOT EXISTS network_zones (
  id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
  cluster_id INTEGER NOT NULL,
  project_name TEXT NOT NULL,
  name TEXT NOT NULL,
  object TEXT NOT NULL,
  last_updated DATETIME NOT NULL,
  UNIQUE (cluster_id, project_name, name),
  FOREIGN KEY (cluster_id) REFERENCES clusters(id)
);

CREATE TABLE IF NOT EXISTS profiles (
  id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
  cluster_id INTEGER NOT NULL,
  project_name TEXT NOT NULL,
  name TEXT NOT NULL,
  object TEXT NOT NULL,
  last_updated DATETIME NOT NULL,
  UNIQUE (cluster_id, project_name, name),
  FOREIGN KEY (cluster_id) REFERENCES clusters(id)
);

CREATE TABLE IF NOT EXISTS projects (
  id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
  cluster_id INTEGER NOT NULL,
  name TEXT NOT NULL,
  object TEXT NOT NULL,
  last_updated DATETIME NOT NULL,
  UNIQUE (cluster_id, name),
  FOREIGN KEY (cluster_id) REFERENCES clusters(id)
);

CREATE TABLE IF NOT EXISTS storage_buckets (
  id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
  cluster_id INTEGER NOT NULL,
  server_id INTEGER NOT NULL,
  project_name TEXT NOT NULL,
  storage_pool_name TEXT NOT NULL,
  name TEXT NOT NULL,
  object TEXT NOT NULL,
  last_updated DATETIME NOT NULL,
  UNIQUE (cluster_id, server_id, project_name, storage_pool_name, name),
  FOREIGN KEY (cluster_id) REFERENCES clusters(id),
  FOREIGN KEY (server_id) REFERENCES servers(id)
);

CREATE TABLE IF NOT EXISTS storage_pools (
  id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
  cluster_id INTEGER NOT NULL,
  name TEXT NOT NULL,
  object TEXT NOT NULL,
  last_updated DATETIME NOT NULL,
  UNIQUE (cluster_id, name),
  FOREIGN KEY (cluster_id) REFERENCES clusters(id)
);

CREATE TABLE IF NOT EXISTS storage_volumes (
  id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
  cluster_id INTEGER NOT NULL,
  server_id INTEGER,
  project_name TEXT NOT NULL,
  storage_pool_name TEXT NOT NULL,
  name TEXT NOT NULL,
  type TEXT NOT NULL,
  object TEXT NOT NULL,
  last_updated DATETIME NOT NULL,
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

CREATE VIEW inventory_images AS
  SELECT
    clusters.name AS cluster_name,
    CASE json_valid(json_group_object(images.project_name, json(images.object))) WHEN 1 THEN json_group_object(images.project_name, json(images.object)) ELSE "null" END AS images
  FROM clusters
    LEFT JOIN (
      SELECT images.cluster_id, images.project_name, json_group_object(images.name, json(images.object)) AS object
      FROM images
      GROUP BY cluster_id, project_name
    ) AS images ON clusters.id = images.cluster_id
  GROUP BY clusters.id;

CREATE VIEW inventory_instances AS
  SELECT
    clusters.name AS cluster_name,
    CASE json_valid(json_group_object(instances.project_name, json(instances.object))) WHEN 1 THEN json_group_object(instances.project_name, json(instances.object)) ELSE "null" END AS instances
  FROM clusters
    LEFT JOIN (
      SELECT instances.cluster_id, instances.project_name, json_group_object(instances.name, json(instances.object)) AS object
      FROM (
        SELECT instances.cluster_id, instances.project_name, instances.name, json_group_object(servers.name, json(instances.object)) AS object
        FROM instances
        LEFT JOIN servers ON instances.server_id = servers.id
        GROUP BY instances.cluster_id, instances.project_name, instances.name
      ) AS instances
      GROUP BY cluster_id, instances.project_name
    ) AS instances ON clusters.id = instances.cluster_id
  GROUP BY clusters.id;

CREATE VIEW inventory_networks AS
  SELECT
    clusters.name AS cluster_name,
    CASE json_valid(json_group_object(networks.project_name, json(networks.object))) WHEN 1 THEN json_group_object(networks.project_name, json(networks.object)) ELSE "null" END AS networks
  FROM clusters
    LEFT JOIN (
      SELECT networks.cluster_id, networks.project_name, json_group_object(networks.name, json(networks.object)) AS object
      FROM networks
      GROUP BY cluster_id, project_name
    ) AS networks ON clusters.id = networks.cluster_id
  GROUP BY clusters.id;

CREATE VIEW inventory_network_acls AS
  SELECT
    clusters.name AS cluster_name,
    CASE json_valid(json_group_object(network_acls.project_name, json(network_acls.object))) WHEN 1 THEN json_group_object(network_acls.project_name, json(network_acls.object)) ELSE "null" END AS network_acls
  FROM clusters
    LEFT JOIN (
      SELECT network_acls.cluster_id, network_acls.project_name, json_group_object(network_acls.name, json(network_acls.object)) AS object
      FROM network_acls
      GROUP BY cluster_id, project_name
    ) AS network_acls ON clusters.id = network_acls.cluster_id
  GROUP BY clusters.id;

CREATE VIEW inventory_network_forwards AS
  SELECT
    clusters.name AS cluster_name,
    CASE json_valid(json_group_object(network_forwards.network_name, json(network_forwards.object))) WHEN 1 THEN json_group_object(network_forwards.network_name, json(network_forwards.object)) ELSE "null" END AS network_forwards
  FROM clusters
    LEFT JOIN (
      SELECT network_forwards.cluster_id, network_forwards.network_name, json_group_object(network_forwards.name, json(network_forwards.object)) AS object
      FROM network_forwards
      GROUP BY cluster_id, network_name
    ) AS network_forwards ON clusters.id = network_forwards.cluster_id
  GROUP BY clusters.id;

CREATE VIEW inventory_network_integrations AS
  SELECT
    clusters.name AS cluster_name,
    CASE json_valid(json_group_object(network_integrations.name, json(network_integrations.object))) WHEN 1 THEN json_group_object(network_integrations.name, json(network_integrations.object)) ELSE "null" END AS network_integrations
  FROM clusters
    LEFT JOIN network_integrations ON clusters.id = network_integrations.cluster_id
  GROUP BY clusters.id;

CREATE VIEW inventory_network_load_balancers AS
  SELECT
    clusters.name AS cluster_name,
    CASE json_valid(json_group_object(network_load_balancers.network_name, json(network_load_balancers.object))) WHEN 1 THEN json_group_object(network_load_balancers.network_name, json(network_load_balancers.object)) ELSE "null" END AS network_load_balancers
  FROM clusters
    LEFT JOIN (
      SELECT network_load_balancers.cluster_id, network_load_balancers.network_name, json_group_object(network_load_balancers.name, json(network_load_balancers.object)) AS object
      FROM network_load_balancers
      GROUP BY cluster_id, network_name
    ) AS network_load_balancers ON clusters.id = network_load_balancers.cluster_id
  GROUP BY clusters.id;

CREATE VIEW inventory_network_peers AS
  SELECT
    clusters.name AS cluster_name,
    CASE json_valid(json_group_object(network_peers.network_name, json(network_peers.object))) WHEN 1 THEN json_group_object(network_peers.network_name, json(network_peers.object)) ELSE "null" END AS network_peers
  FROM clusters
    LEFT JOIN (
      SELECT network_peers.cluster_id, network_peers.network_name, json_group_object(network_peers.name, json(network_peers.object)) AS object
      FROM network_peers
      GROUP BY cluster_id, network_name
    ) AS network_peers ON clusters.id = network_peers.cluster_id
  GROUP BY clusters.id;

CREATE VIEW inventory_network_zones AS
  SELECT
    clusters.name AS cluster_name,
    CASE json_valid(json_group_object(network_zones.project_name, json(network_zones.object))) WHEN 1 THEN json_group_object(network_zones.project_name, json(network_zones.object)) ELSE "null" END AS network_zones
  FROM clusters
    LEFT JOIN (
      SELECT network_zones.cluster_id, network_zones.project_name, json_group_object(network_zones.name, json(network_zones.object)) AS object
      FROM network_zones
      GROUP BY cluster_id, project_name
    ) AS network_zones ON clusters.id = network_zones.cluster_id
  GROUP BY clusters.id;

CREATE VIEW inventory_profiles AS
  SELECT
    clusters.name AS cluster_name,
    CASE json_valid(json_group_object(profiles.project_name, json(profiles.object))) WHEN 1 THEN json_group_object(profiles.project_name, json(profiles.object)) ELSE "null" END AS profiles
  FROM clusters
    LEFT JOIN (
      SELECT profiles.cluster_id, profiles.project_name, json_group_object(profiles.name, json(profiles.object)) AS object
      FROM profiles
      GROUP BY cluster_id, project_name
    ) AS profiles ON clusters.id = profiles.cluster_id
  GROUP BY clusters.id;

CREATE VIEW inventory_projects AS
  SELECT
    clusters.name AS cluster_name,
    CASE json_valid(json_group_object(projects.name, json(projects.object))) WHEN 1 THEN json_group_object(projects.name, json(projects.object)) ELSE "null" END AS projects
  FROM clusters
    LEFT JOIN projects ON clusters.id = projects.cluster_id
  GROUP BY clusters.id;

CREATE VIEW inventory_storage_buckets AS
  SELECT
    clusters.name AS cluster_name,
    CASE json_valid(json_group_object(storage_buckets.server_name, json(storage_buckets.object))) WHEN 1 THEN json_group_object(storage_buckets.server_name, json(storage_buckets.object)) ELSE "null" END AS storage_buckets
  FROM clusters
    LEFT JOIN (
      SELECT cluster_id, server_name, json_group_object(storage_buckets.project_name, json(storage_buckets.object)) AS object
      FROM (
        SELECT cluster_id, server_name, project_name, json_group_object(storage_buckets.storage_pool_name, json(storage_buckets.object)) AS object
        FROM (
          SELECT storage_buckets.cluster_id, servers.name AS server_name, storage_buckets.project_name, storage_buckets.storage_pool_name, json_group_object(storage_buckets.name, json(storage_buckets.object)) AS object
          FROM storage_buckets
          LEFT JOIN servers ON storage_buckets.server_id = servers.id
          GROUP BY storage_buckets.cluster_id, storage_buckets.server_id, storage_buckets.project_name, storage_buckets.storage_pool_name
        ) AS storage_buckets
        GROUP BY cluster_id, server_name, project_name
      ) AS storage_buckets
      GROUP BY cluster_id, server_name
    ) AS storage_buckets ON clusters.id = storage_buckets.cluster_id
  GROUP BY clusters.id;

CREATE VIEW inventory_storage_pools AS
  SELECT
    clusters.name AS cluster_name,
    CASE json_valid(json_group_object(storage_pools.name, json(storage_pools.object))) WHEN 1 THEN json_group_object(storage_pools.name, json(storage_pools.object)) ELSE "null" END AS storage_pools
  FROM clusters
    LEFT JOIN storage_pools ON clusters.id = storage_pools.cluster_id
  GROUP BY clusters.id;

CREATE VIEW inventory_storage_volumes AS
  SELECT
    clusters.name AS cluster_name,
    CASE json_valid(json_group_object(storage_volumes.project_name, json(storage_volumes.object))) WHEN 1 THEN json_group_object(storage_volumes.project_name, json(storage_volumes.object)) ELSE "null" END AS storage_volumes
  FROM clusters
    LEFT JOIN (
      SELECT cluster_id, project_name, json_group_object(storage_volumes.storage_pool_name, json(storage_volumes.object)) AS object
      FROM (
        SELECT cluster_id, project_name, storage_pool_name, json_group_object(storage_volumes.name, json(storage_volumes.object)) AS object
        FROM (
          SELECT storage_volumes.cluster_id, storage_volumes.project_name, storage_volumes.storage_pool_name, storage_volumes.type || "/" || storage_volumes.name AS name, json_group_object(coalesce(servers.name, ''), json(storage_volumes.object)) AS object
          FROM storage_volumes
          LEFT JOIN servers ON storage_volumes.server_id = servers.id
          GROUP BY storage_volumes.cluster_id, storage_volumes.project_name, storage_volumes.storage_pool_name, storage_volumes.type || "/" || storage_volumes.name
        ) AS storage_volumes
        GROUP BY cluster_id, project_name, storage_pool_name
      ) AS storage_volumes
      GROUP BY cluster_id, project_name
    ) AS storage_volumes ON clusters.id = storage_volumes.cluster_id
  GROUP BY clusters.id;

CREATE VIEW inventory AS
  SELECT
    clusters.name AS cluster_name,
    servers,
    images,
    instances,
    networks,
    network_acls,
    network_forwards,
    network_integrations,
    network_load_balancers,
    network_peers,
    network_zones,
    profiles,
    projects,
    storage_buckets,
    storage_pools,
    storage_volumes
  FROM
    clusters,
    (SELECT clusters.name AS cluster_name, json_group_array(DISTINCT servers.name) AS servers FROM servers INNER JOIN clusters ON servers.cluster_id = clusters.id GROUP BY servers.cluster_id) inventory_servers,
    inventory_images,
    inventory_instances,
    inventory_networks,
    inventory_network_acls,
    inventory_network_forwards,
    inventory_network_integrations,
    inventory_network_load_balancers,
    inventory_network_peers,
    inventory_network_zones,
    inventory_profiles,
    inventory_projects,
    inventory_storage_buckets,
    inventory_storage_pools,
    inventory_storage_volumes
  WHERE
    clusters.name = inventory_servers.cluster_name AND
    clusters.name = inventory_images.cluster_name AND
    clusters.name = inventory_instances.cluster_name AND
    clusters.name = inventory_networks.cluster_name AND
    clusters.name = inventory_network_acls.cluster_name AND
    clusters.name = inventory_network_forwards.cluster_name AND
    clusters.name = inventory_network_integrations.cluster_name AND
    clusters.name = inventory_network_load_balancers.cluster_name AND
    clusters.name = inventory_network_peers.cluster_name AND
    clusters.name = inventory_network_zones.cluster_name AND
    clusters.name = inventory_profiles.cluster_name AND
    clusters.name = inventory_projects.cluster_name AND
    clusters.name = inventory_storage_buckets.cluster_name AND
    clusters.name = inventory_storage_pools.cluster_name AND
    clusters.name = inventory_storage_volumes.cluster_name
;

INSERT INTO schema (version, updated_at) VALUES (2, strftime("%s"))
