# File Dedup

A high-performance file deduplication system that scans directories, identifies duplicate files, and provides a web interface to view results.

## Quick Start

### 1. Setup Database

```sh
go install github.com/tendant/dbstrap/cmd/dbstrap@latest
export FILEDEDUP_USER_PASSWORD=pwd 
export DATABASE_URL="postgres://postgres:pwd@localhost:5432/postgres?sslmode=disable"
dbstrap run --config=bootstrap.yaml
```

### 2. Run Server

```sh
export DATABASE_URL="postgres://filededup:pwd@localhost:5432/filededup?sslmode=disable"
DATABASE_URL="$DATABASE_URL" go run ./cmd/server
```

### 3. Run Agent

```sh
go run ./cmd/agent -dir "/path/to/scan" -server "http://localhost:8080" -machine-id "my-machine"
```

## Agent Options

```
-dir string           Directory to scan (default ".")
-server string        Server URL (default "http://localhost:8080")
-machine-id string    Unique machine identifier (default "default")
-batch int            Number of files per batch (default 1000)
-workers int          Number of parallel workers (0 = auto)
-queue-size int       Size of processing queues (0 = auto)
-verbose              Enable verbose logging
-skip-large           Skip files larger than the size limit
-max-size int64       Maximum file size to process (default 1GB)
```

## API Endpoints

- `POST /files` - Upload file records
- `GET /duplicates` - View duplicate files
