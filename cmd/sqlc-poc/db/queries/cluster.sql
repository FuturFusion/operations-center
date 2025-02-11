-- name: GetCluster :one
SELECT * FROM clusters
WHERE id = :id LIMIT 1;

-- name: ListClusters :many
SELECT * FROM clusters
ORDER BY name;

-- name: CreateCluster :one
INSERT INTO clusters (
  name, connection_url, server_hostnames, last_updated
) VALUES (
  :name, :connection_url, :server_hostnames, :last_updated
)
RETURNING *;

-- name: UpdateCluster :exec
UPDATE clusters
SET
  name = :name,
  connection_url = :connection_url,
  server_hostnames = :server_hostnames,
  last_updated = :last_updated
WHERE
  id = :id
RETURNING *;

-- name: DeleteCluster :exec
DELETE FROM clusters
WHERE id = :id;
