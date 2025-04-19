// main.go (API server using go-chi, updated to use path + filename split)
package record

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"log/slog"
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
		slog.Info("Handling request for duplicates")
		
		// First, check if we have any files in the database
		count, err := q.CountFiles(r.Context())
		if err != nil {
			slog.Error("Error counting files", "error", err)
			http.Error(w, "Failed to query database", http.StatusInternalServerError)
			return
		}
		slog.Info("Files in database", "count", count)
		
		if count == 0 {
			// No files, return empty array
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]struct{}{})
			return
		}
		
		// Query for duplicates
		slog.Info("Querying for duplicate files")
		dupes, err := q.FindDuplicateFiles(r.Context())
		if err != nil {
			slog.Error("Error querying duplicates", "error", err)
			http.Error(w, "Failed to query duplicates", http.StatusInternalServerError)
			return
		}
		slog.Info("Found duplicate files", "sets", len(dupes))
		
		// Convert to a more JSON-friendly format
		type DuplicateFile struct {
			Hash           string   `json:"hash"`
			DuplicateCount int64    `json:"duplicate_count"`
			Paths          []string `json:"paths"`
		}
		
		var result []DuplicateFile
		for _, d := range dupes {
			// Convert the array_agg result to a string slice
			paths, ok := d.Paths.([]interface{})
			if !ok {
				slog.Warn("Could not convert paths", "type", d.Paths)
				continue
			}
			
			pathStrings := make([]string, 0, len(paths))
			for _, p := range paths {
				if str, ok := p.(string); ok {
					pathStrings = append(pathStrings, str)
				}
			}
			
			result = append(result, DuplicateFile{
				Hash:           d.Hash,
				DuplicateCount: d.DuplicateCount,
				Paths:          pathStrings,
			})
		}
		
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(result); err != nil {
			slog.Error("Error encoding JSON response", "error", err)
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		}
	}
}
