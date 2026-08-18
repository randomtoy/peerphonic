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
	var users []domain.User
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate users: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close users: %w", err)
	}
	if err := c.loadUserPermissions(ctx, users); err != nil {
		return nil, err
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
	users := []domain.User{credential.User}
	if err := c.loadUserPermissions(ctx, users); err != nil {
		return ports.UserCredential{}, err
	}
	credential.User = users[0]
	return credential, nil
}

func (c *Catalog) CreateUser(ctx context.Context, credential ports.UserCredential) error {
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin user creation: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `INSERT INTO users (
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
	if err := insertUserPermissions(ctx, tx, credential.User.Username, credential.User.Permissions); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit user creation: %w", err)
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

func (c *Catalog) UpdateUserPermissions(
	ctx context.Context, username string, permissions []domain.Permission,
) error {
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin user permission update: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, "UPDATE users SET updated_at = ? WHERE username = ?",
		time.Now().UTC().Format(time.RFC3339Nano), username)
	if err != nil {
		return fmt.Errorf("touch updated user permissions: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read permission user count: %w", err)
	}
	if changed == 0 {
		return ports.ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM user_permissions WHERE username = ?", username); err != nil {
		return fmt.Errorf("clear user permissions: %w", err)
	}
	if err := insertUserPermissions(ctx, tx, username, permissions); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit user permission update: %w", err)
	}
	return nil
}

type sqlExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func insertUserPermissions(
	ctx context.Context, executor sqlExecutor, username string, permissions []domain.Permission,
) error {
	for _, permission := range permissions {
		if _, err := executor.ExecContext(ctx,
			"INSERT INTO user_permissions(username, permission) VALUES (?, ?)", username, permission,
		); err != nil {
			return fmt.Errorf("insert user permission %q: %w", permission, err)
		}
	}
	return nil
}

func (c *Catalog) loadUserPermissions(ctx context.Context, users []domain.User) error {
	if len(users) == 0 {
		return nil
	}
	indexes := make(map[string]int, len(users))
	for index := range users {
		indexes[users[index].Username] = index
	}
	rows, err := c.db.QueryContext(ctx, `SELECT username, permission FROM user_permissions
		ORDER BY username COLLATE NOCASE, permission`)
	if err != nil {
		return fmt.Errorf("query user permissions: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var username string
		var permission domain.Permission
		if err := rows.Scan(&username, &permission); err != nil {
			return fmt.Errorf("scan user permission: %w", err)
		}
		if index, ok := indexes[username]; ok {
			users[index].Permissions = append(users[index].Permissions, permission)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate user permissions: %w", err)
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
