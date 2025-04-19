package main

import (
	"context"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tendant/filededup/pkg/record"
	"github.com/tendant/filededup/pkg/record/recorddb"
)

func main() {
	// Connect using pgx instead of database/sql
	dbConn, err := pgxpool.New(context.Background(), "postgres://user:pass@localhost:5432/yourdb?sslmode=disable")
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
