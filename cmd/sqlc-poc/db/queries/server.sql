-- name: GetServer :one
SELECT * FROM servers
WHERE id = :id LIMIT 1;

-- name: ListServers :many
SELECT * FROM servers
ORDER BY hostname;

-- name: CreateServer :one
INSERT INTO servers (
  cluster_id, hostname, type, connection_url, last_updated
) VALUES (
  :cluster_id, :hostname, :type, :connection_url, :last_updated
)
RETURNING *;

-- name: UpdateServer :exec
UPDATE servers
SET
  cluster_id = :cluster_id,
  hostname = :hostname,
  type = :type,
  connection_url = :connection_url,
  last_updated = :last_updated
WHERE
  id = :id
RETURNING *;

-- name: DeleteServer :exec
DELETE FROM servers
WHERE id = :id;
