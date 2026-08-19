CREATE TABLE user_permissions (
    username    TEXT NOT NULL REFERENCES users(username) ON DELETE CASCADE,
    permission  TEXT NOT NULL,
    PRIMARY KEY (username, permission)
);
