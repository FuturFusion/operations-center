CREATE TABLE clusters (
  id INTEGER PRIMARY KEY NOT NULL,
  name TEXT NOT NULL,
  connection_url TEXT NOT NULL,
  server_hostnames TEXT NOT NULL,
  last_updated DATETIME NOT NULL,
  UNIQUE (name)
);

INSERT INTO clusters (name, connection_url, server_hostnames, last_updated) VALUES ('one', 'http://localhost/', 'srv1,srv2', '2025-02-11T09:36:00Z');

CREATE TABLE servers (
  id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
  cluster_id INTEGER NOT NULL,
  hostname TEXT NOT NULL,
  type TEXT NOT NULL,
  connection_url TEXT NOT NULL,
  last_updated DATETIME NOT NULL,
  UNIQUE (hostname),
  FOREIGN KEY(cluster_id) REFERENCES clusters(id)
);

INSERT INTO servers (cluster_id, hostname, type, connection_url, last_updated) VALUES (1, 'srv1', 'incus', 'http://srv1/', '2025-02-11T09:40:00Z');

CREATE TABLE storage_volumes (
  server_id    INTEGER NOT NULL,
  project_id   INTEGER NOT NULL,
  name         TEXT NOT NULL,
  object       TEXT NOT NULL,
  last_updated TEXT NOT NULL,
  UNIQUE (server_id, project_id, name),
  FOREIGN KEY (server_id) REFERENCES servers(id)
);

INSERT INTO storage_volumes (server_id, project_id, name, object, last_updated) VALUES
(1, 1, 'one', '{"config":{},"description":"","name":"foo","type":"custom","used_by":[],"location":"none","content_type":"filesystem","project":"default","created_at":"2025-02-10T14:21:28.576301337Z"}', '2025-02-11T09:41:00Z'),
(1, 2, 'two', '{"config":{},"description":"","name":"foo2","type":"custom","used_by":[],"location":"none","content_type":"filesystem","project":"default","created_at":"2025-02-10T16:00:00.576301337Z"}', '2025-02-11T09:42:00Z');
