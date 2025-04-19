package main

import (
	"database/sql"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/tendant/filededup/pkg/record/recorddb"
)

func main() {
	dbConn, err := sql.Open("postgres", "postgres://user:pass@localhost:5432/yourdb?sslmode=disable")
	if err != nil {
		log.Fatal(err)
	}
	dbQueries := recorddb.New(dbConn)

	r := chi.NewRouter()
	r.Post("/files", recorddb.uploadFilesHandler(dbQueries))
	r.Get("/duplicates", recorddb.findDuplicatesHandler(dbQueries))

	log.Println("Server running on :8080")
	http.ListenAndServe(":8080", r)
}
