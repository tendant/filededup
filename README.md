# File Dedup



## Setup

### Bootstrap

```sh
export FILEDEDUP_USER_PASSWORD=pwd 
export DATABASE_URL="postgres://postgres:pwd@localhost:5432/postgres?sslmode=disable"
dbstrap run --config=bootstrap.yaml
```