CREATE TABLE files (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    machine_id TEXT NOT NULL,
    path TEXT NOT NULL,
    filename TEXT NOT NULL,
    size BIGINT NOT NULL,
    mtime TIMESTAMP NOT NULL,
    hash TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT now(),
    UNIQUE (machine_id, path, filename)
);