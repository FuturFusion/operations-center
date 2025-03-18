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

INSERT INTO schema (version, updated_at) VALUES (2, strftime("%s"))
