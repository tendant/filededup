-- name: UpsertFile :exec
INSERT INTO files (machine_id, path, filename, size, mtime, hash)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (machine_id, path, filename)
DO UPDATE SET size = EXCLUDED.size, mtime = EXCLUDED.mtime, hash = EXCLUDED.hash;