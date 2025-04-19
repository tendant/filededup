package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tendant/filededup/pkg/record"
	"github.com/tendant/filededup/pkg/record/recorddb"
)

func main() {
	// Set up structured logging
	logHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	slog.SetDefault(slog.New(logHandler))

	// Get database connection string from environment variable or use default
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://filededup:pwd@localhost:5432/filededup?sslmode=disable&host=localhost"
	}

	slog.Info("Connecting to database", "url", dbURL)

	// Connect using pgx
	dbConn, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		slog.Error("Failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer dbConn.Close()

	// Test database connection
	if err := dbConn.Ping(context.Background()); err != nil {
		slog.Error("Failed to ping database", "error", err)
		os.Exit(1)
	}
	slog.Info("Database connection successful")

	dbQueries := recorddb.New(dbConn)

	r := chi.NewRouter()
	r.Post("/files", record.UploadFilesHandler(dbQueries))
	r.Get("/duplicates", record.FindDuplicatesHandler(dbQueries))

	slog.Info("Server running", "port", 8080)
	http.ListenAndServe("0.0.0.0:8080", r)
}
