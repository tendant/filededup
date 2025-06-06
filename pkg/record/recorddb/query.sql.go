// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.27.0
// source: query.sql

package recorddb

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

const countFiles = `-- name: CountFiles :one
SELECT COUNT(*) FROM files
`

func (q *Queries) CountFiles(ctx context.Context) (int64, error) {
	row := q.db.QueryRow(ctx, countFiles)
	var count int64
	err := row.Scan(&count)
	return count, err
}

const findDuplicateFiles = `-- name: FindDuplicateFiles :many
SELECT hash, COUNT(*) AS duplicate_count, array_agg(path || '/' || filename ORDER BY path, filename) AS paths
FROM files
GROUP BY hash
HAVING COUNT(*) > 1
`

type FindDuplicateFilesRow struct {
	Hash           string
	DuplicateCount int64
	Paths          interface{}
}

func (q *Queries) FindDuplicateFiles(ctx context.Context) ([]FindDuplicateFilesRow, error) {
	rows, err := q.db.Query(ctx, findDuplicateFiles)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []FindDuplicateFilesRow
	for rows.Next() {
		var i FindDuplicateFilesRow
		if err := rows.Scan(&i.Hash, &i.DuplicateCount, &i.Paths); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const upsertFile = `-- name: UpsertFile :exec
INSERT INTO files (machine_id, path, filename, size, mtime, hash)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (machine_id, path, filename)
DO UPDATE SET size = EXCLUDED.size, mtime = EXCLUDED.mtime, hash = EXCLUDED.hash
`

type UpsertFileParams struct {
	MachineID string
	Path      string
	Filename  string
	Size      int64
	Mtime     pgtype.Timestamp
	Hash      string
}

func (q *Queries) UpsertFile(ctx context.Context, arg UpsertFileParams) error {
	_, err := q.db.Exec(ctx, upsertFile,
		arg.MachineID,
		arg.Path,
		arg.Filename,
		arg.Size,
		arg.Mtime,
		arg.Hash,
	)
	return err
}
