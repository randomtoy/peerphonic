package sqlite

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

func TestUserStorageLifecycle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	catalog, err := Open(ctx, filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Close()
	now := time.Now().UTC().Truncate(time.Microsecond)
	credential := ports.UserCredential{
		User: domain.User{
			Username: "alice", Role: domain.UserRoleUser,
			Permissions: []domain.Permission{domain.PermissionDashboardAccess},
			CreatedAt:   now, UpdatedAt: now,
		},
		PasswordHash: []byte("hash"), EncryptedToken: []byte("encrypted"),
	}
	if err := catalog.CreateUser(ctx, credential); err != nil {
		t.Fatal(err)
	}
	if err := catalog.CreateUser(ctx, credential); !errors.Is(err, ports.ErrAlreadyExists) {
		t.Fatalf("duplicate CreateUser() error = %v", err)
	}
	stored, err := catalog.UserCredential(ctx, "alice")
	if err != nil || stored.User.Role != domain.UserRoleUser ||
		!stored.User.HasPermission(domain.PermissionDashboardAccess) ||
		!bytes.Equal(stored.PasswordHash, credential.PasswordHash) ||
		!bytes.Equal(stored.EncryptedToken, credential.EncryptedToken) {
		t.Fatalf("UserCredential() = %#v, %v", stored, err)
	}
	if err := catalog.UpdateUserPassword(ctx, "alice", []byte("new-hash"), []byte("new-token")); err != nil {
		t.Fatal(err)
	}
	if err := catalog.UpdateUserPermissions(ctx, "alice", []domain.Permission{
		domain.PermissionMonitoringView, domain.PermissionSourcesManage,
	}); err != nil {
		t.Fatal(err)
	}
	stored, err = catalog.UserCredential(ctx, "alice")
	if err != nil || string(stored.PasswordHash) != "new-hash" || string(stored.EncryptedToken) != "new-token" ||
		!stored.User.HasPermission(domain.PermissionMonitoringView) ||
		!stored.User.HasPermission(domain.PermissionSourcesManage) {
		t.Fatalf("updated UserCredential() = %#v, %v", stored, err)
	}
	users, err := catalog.Users(ctx)
	if err != nil || len(users) != 1 || users[0].Username != "alice" {
		t.Fatalf("Users() = %#v, %v", users, err)
	}
	if err := catalog.DeleteUser(ctx, "alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.UserCredential(ctx, "alice"); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("deleted UserCredential() error = %v", err)
	}
}
