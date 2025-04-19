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
	// Set up logging
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	
	// Get database connection string from environment variable or use default
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://filededup:pwd@localhost:5432/filededup?sslmode=disable&host=localhost"
	}
	
	log.Printf("Connecting to database: %s", dbURL)
	
	// Connect using pgx
	dbConn, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		log.Fatal(err)
	}
	defer dbConn.Close()
	
	// Test database connection
	if err := dbConn.Ping(context.Background()); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}
	log.Println("Database connection successful")
	
	dbQueries := recorddb.New(dbConn)

	r := chi.NewRouter()
	r.Post("/files", record.UploadFilesHandler(dbQueries))
	r.Get("/duplicates", record.FindDuplicatesHandler(dbQueries))

	log.Println("Server running on :8080")
	http.ListenAndServe(":8080", r)
}
