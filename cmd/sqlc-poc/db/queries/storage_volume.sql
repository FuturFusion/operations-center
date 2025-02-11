-- name: ListStorageVolumes :many
SELECT * FROM storage_volumes;

-- name: ListStorageVolumesFiltered :many
SELECT storage_volumes.*, servers.hostname as server_hostname, clusters.name as cluster_name
FROM
  storage_volumes
  INNER JOIN servers ON storage_volumes.server_id = servers.id
  INNER JOIN clusters ON servers.cluster_id = clusters.id
WHERE
(sqlc.narg('cluster_id') IS NULL OR clusters.id = sqlc.narg('cluster_id'))
AND (sqlc.narg('server_id') IS NULL OR servers.id = sqlc.narg('server_id') OR sqlc.narg('server_id'))
AND (sqlc.narg('project_id') IS NULL OR storage_volumes.project_id = sqlc.narg('project_id'))
AND (sqlc.narg('name') IS NULL OR storage_volumes.name = sqlc.narg('name'));
