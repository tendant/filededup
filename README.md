# File Dedup



## Setup

### Bootstrap

```sh
export FILEDEDUP_USER_PASSWORD=pwd 
export DATABASE_URL="postgres://postgres:pwd@localhost:5432/postgres?sslmode=disable"
dbstrap run --config=bootstrap.yaml
```

### Run

```sh
export DATABASE_URL="postgres://filededup:pwd@localhost:5432/filededup?sslmode=disable"
DATABASE_URL="$DATABASE_URL" go run ./cmd/server
```

```sh
go run ./cmd/agent -dir "." -server "http://localhost:8080" -machine-id "my-machine"
```
