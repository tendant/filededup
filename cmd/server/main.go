package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tendant/filededup/pkg/record"
	"github.com/tendant/filededup/pkg/record/recorddb"
)

func main() {
	// Get database connection string from environment variable or use default
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://filededup:pwd@localhost:5432/filededup?sslmode=disable"
	}
	
	log.Printf("Connecting to database: %s", dbURL)
	
	// Connect using pgx
	dbConn, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		log.Fatal(err)
	}
	defer dbConn.Close()
	
	dbQueries := recorddb.New(dbConn)

	r := chi.NewRouter()
	r.Post("/files", record.UploadFilesHandler(dbQueries))
	r.Get("/duplicates", record.FindDuplicatesHandler(dbQueries))

	log.Println("Server running on :8080")
	http.ListenAndServe(":8080", r)
}
