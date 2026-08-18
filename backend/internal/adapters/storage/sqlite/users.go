package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

func (c *Catalog) Users(ctx context.Context) ([]domain.User, error) {
	rows, err := c.db.QueryContext(ctx, `SELECT username, role, created_at, updated_at
		FROM users ORDER BY username COLLATE NOCASE`)
	if err != nil {
		return nil, fmt.Errorf("query users: %w", err)
	}
	defer rows.Close()
	var users []domain.User
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate users: %w", err)
	}
	return users, nil
}

func (c *Catalog) UserCredential(ctx context.Context, username string) (ports.UserCredential, error) {
	row := c.db.QueryRowContext(ctx, `SELECT username, role, created_at, updated_at,
		password_hash, encrypted_token FROM users WHERE username = ?`, username)
	var createdAt, updatedAt string
	var credential ports.UserCredential
	if err := row.Scan(
		&credential.User.Username, &credential.User.Role, &createdAt, &updatedAt,
		&credential.PasswordHash, &credential.EncryptedToken,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ports.UserCredential{}, ports.ErrNotFound
		}
		return ports.UserCredential{}, fmt.Errorf("query user credential: %w", err)
	}
	var err error
	credential.User.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return ports.UserCredential{}, fmt.Errorf("parse user creation time: %w", err)
	}
	credential.User.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return ports.UserCredential{}, fmt.Errorf("parse user update time: %w", err)
	}
	return credential, nil
}

func (c *Catalog) CreateUser(ctx context.Context, credential ports.UserCredential) error {
	result, err := c.db.ExecContext(ctx, `INSERT INTO users (
		username, role, password_hash, encrypted_token, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT(username) DO NOTHING`,
		credential.User.Username, credential.User.Role, credential.PasswordHash, credential.EncryptedToken,
		credential.User.CreatedAt.UTC().Format(time.RFC3339Nano),
		credential.User.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read created user count: %w", err)
	}
	if changed == 0 {
		return ports.ErrAlreadyExists
	}
	return nil
}

func (c *Catalog) UpdateUserPassword(
	ctx context.Context, username string, passwordHash, encryptedToken []byte,
) error {
	result, err := c.db.ExecContext(ctx, `UPDATE users SET
		password_hash = ?, encrypted_token = ?, updated_at = ? WHERE username = ?`,
		passwordHash, encryptedToken, time.Now().UTC().Format(time.RFC3339Nano), username,
	)
	if err != nil {
		return fmt.Errorf("update user password: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read updated user count: %w", err)
	}
	if changed == 0 {
		return ports.ErrNotFound
	}
	return nil
}

func (c *Catalog) DeleteUser(ctx context.Context, username string) error {
	result, err := c.db.ExecContext(ctx, "DELETE FROM users WHERE username = ?", username)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read deleted user count: %w", err)
	}
	if changed == 0 {
		return ports.ErrNotFound
	}
	return nil
}

type userScanner interface {
	Scan(dest ...any) error
}

func scanUser(row userScanner) (domain.User, error) {
	var user domain.User
	var createdAt, updatedAt string
	if err := row.Scan(&user.Username, &user.Role, &createdAt, &updatedAt); err != nil {
		return domain.User{}, fmt.Errorf("scan user: %w", err)
	}
	var err error
	user.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return domain.User{}, fmt.Errorf("parse user creation time: %w", err)
	}
	user.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return domain.User{}, fmt.Errorf("parse user update time: %w", err)
	}
	return user, nil
}
