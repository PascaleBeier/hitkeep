CREATE TABLE control_migrations (
    name TEXT PRIMARY KEY,
    checksum TEXT NOT NULL,
    applied_at TEXT NOT NULL
);

CREATE TABLE control_imports (
    name TEXT PRIMARY KEY,
    source_sha256 TEXT NOT NULL,
    source_schema_sha256 TEXT NOT NULL,
    imported_at TEXT NOT NULL
);
