CREATE TABLE torrent_transfer_settings (
    id                               INTEGER PRIMARY KEY CHECK (id = 1),
    upload_limit_bytes_per_second    INTEGER NOT NULL CHECK (upload_limit_bytes_per_second >= 0),
    download_limit_bytes_per_second  INTEGER NOT NULL CHECK (download_limit_bytes_per_second >= 0),
    updated_at                       TEXT NOT NULL
);
