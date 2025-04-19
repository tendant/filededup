// diagnose.go - A diagnostic tool for the filededup system
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("Starting diagnostic tool for filededup")

	// Get database connection string from environment variable or use default
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://filededup:pwd@localhost:5432/filededup?sslmode=disable&host=localhost"
	}

	log.Printf("Connecting to database: %s", dbURL)

	// Connect using pgx
	dbConn, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer dbConn.Close()

	// Test database connection
	if err := dbConn.Ping(context.Background()); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}
	log.Println("Database connection successful")

	// Check if the files table exists
	var tableExists bool
	err = dbConn.QueryRow(context.Background(), `
		SELECT EXISTS (
			SELECT FROM information_schema.tables 
			WHERE table_schema = 'public' 
			AND table_name = 'files'
		)
	`).Scan(&tableExists)

	if err != nil {
		log.Fatalf("Failed to check if files table exists: %v", err)
	}

	if !tableExists {
		log.Fatalf("The 'files' table does not exist in the database")
	}
	log.Println("The 'files' table exists in the database")

	// Count files in the database
	var count int
	err = dbConn.QueryRow(context.Background(), "SELECT COUNT(*) FROM files").Scan(&count)
	if err != nil {
		log.Fatalf("Failed to count files: %v", err)
	}
	log.Printf("Found %d files in the database", count)

	// If there are files, try to query for duplicates
	if count > 0 {
		log.Println("Attempting to run the duplicate files query...")

		rows, err := dbConn.Query(context.Background(), `
			SELECT hash, COUNT(*) AS duplicate_count, array_agg(path || '/' || filename ORDER BY path, filename) AS paths
			FROM files
			GROUP BY hash
			HAVING COUNT(*) > 1
		`)
		if err != nil {
			log.Fatalf("Failed to query duplicates: %v", err)
		}
		defer rows.Close()

		// Count duplicate sets
		var dupeCount int
		for rows.Next() {
			dupeCount++
			var hash string
			var count int64
			var paths interface{}

			if err := rows.Scan(&hash, &count, &paths); err != nil {
				log.Printf("Error scanning row: %v", err)
				continue
			}

			fmt.Printf("Duplicate set #%d: Hash=%s, Count=%d\n", dupeCount, hash, count)
		}

		if dupeCount == 0 {
			log.Println("No duplicate files found")
		} else {
			log.Printf("Found %d sets of duplicate files", dupeCount)
		}
	}

	log.Println("Diagnostic complete")
}
