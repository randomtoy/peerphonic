CREATE TABLE users (
    username          TEXT PRIMARY KEY,
    role              TEXT NOT NULL CHECK (role IN ('admin', 'user')),
    password_hash     BLOB NOT NULL,
    encrypted_token   BLOB NOT NULL,
    created_at        TEXT NOT NULL,
    updated_at        TEXT NOT NULL
);
