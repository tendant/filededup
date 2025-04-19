// main.go (API server using go-chi, updated to use path + filename split)
package record

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/tendant/filededup/pkg/record/recorddb"
)

type FileRecord struct {
	MachineID string    `json:"machine_id"`
	Path      string    `json:"path"`
	Filename  string    `json:"filename"`
	Size      int64     `json:"size"`
	MTime     time.Time `json:"mtime"`
	Hash      string    `json:"hash"`
}

// UploadFilesHandler handles HTTP requests to upload file records
func UploadFilesHandler(q *recorddb.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var reader io.Reader = r.Body
		if r.Header.Get("Content-Encoding") == "gzip" {
			gz, err := gzip.NewReader(r.Body)
			if err != nil {
				http.Error(w, "Failed to decompress", http.StatusBadRequest)
				return
			}
			defer gz.Close()
			reader = gz
		}

		var files []FileRecord
		if err := json.NewDecoder(reader).Decode(&files); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		for _, f := range files {
			var pgTime pgtype.Timestamp
			pgTime.Time = f.MTime
			pgTime.Valid = true

			_ = q.UpsertFile(r.Context(), recorddb.UpsertFileParams{
				MachineID: f.MachineID,
				Path:      f.Path,
				Filename:  f.Filename,
				Size:      f.Size,
				Mtime:     pgTime,
				Hash:      f.Hash,
			})
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// FindDuplicatesHandler handles HTTP requests to find duplicate files
func FindDuplicatesHandler(q *recorddb.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dupes, err := q.FindDuplicateFiles(r.Context())
		if err != nil {
			http.Error(w, "Failed to query duplicates", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(dupes)
	}
}
