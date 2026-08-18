package services

import (
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"sort"
	"testing"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

type userStoreStub struct {
	credentials map[string]ports.UserCredential
}

func newUserStoreStub() *userStoreStub {
	return &userStoreStub{credentials: make(map[string]ports.UserCredential)}
}

func (s *userStoreStub) Users(context.Context) ([]domain.User, error) {
	users := make([]domain.User, 0, len(s.credentials))
	for _, credential := range s.credentials {
		users = append(users, credential.User)
	}
	sort.Slice(users, func(i, j int) bool { return users[i].Username < users[j].Username })
	return users, nil
}

func (s *userStoreStub) UserCredential(_ context.Context, username string) (ports.UserCredential, error) {
	credential, ok := s.credentials[username]
	if !ok {
		return ports.UserCredential{}, ports.ErrNotFound
	}
	return credential, nil
}

func (s *userStoreStub) CreateUser(_ context.Context, credential ports.UserCredential) error {
	if _, exists := s.credentials[credential.User.Username]; exists {
		return ports.ErrAlreadyExists
	}
	s.credentials[credential.User.Username] = credential
	return nil
}

func (s *userStoreStub) UpdateUserPassword(
	_ context.Context, username string, hash, encrypted []byte,
) error {
	credential, ok := s.credentials[username]
	if !ok {
		return ports.ErrNotFound
	}
	credential.PasswordHash = hash
	credential.EncryptedToken = encrypted
	s.credentials[username] = credential
	return nil
}

func (s *userStoreStub) UpdateUserPermissions(
	_ context.Context, username string, permissions []domain.Permission,
) error {
	credential, ok := s.credentials[username]
	if !ok {
		return ports.ErrNotFound
	}
	credential.User.Permissions = append([]domain.Permission(nil), permissions...)
	s.credentials[username] = credential
	return nil
}

func (s *userStoreStub) DeleteUser(_ context.Context, username string) error {
	if _, ok := s.credentials[username]; !ok {
		return ports.ErrNotFound
	}
	delete(s.credentials, username)
	return nil
}

type credentialCodecStub struct{}

func (credentialCodecStub) Encode(password string) ([]byte, []byte, error) {
	return []byte("hash:" + password), []byte(password), nil
}

func (credentialCodecStub) Verify(hash []byte, password string) error {
	if string(hash) != "hash:"+password {
		return errors.New("password mismatch")
	}
	return nil
}

func (credentialCodecStub) DecryptToken(encrypted []byte) ([]byte, error) {
	return append([]byte(nil), encrypted...), nil
}

func TestUserServiceBootstrapAuthenticationAndAdministration(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newUserStoreStub()
	service := NewUserService(store, credentialCodecStub{})
	if err := service.EnsureBootstrapAdmin(ctx, "Admin", "admin"); err != nil {
		t.Fatal(err)
	}
	if err := service.EnsureBootstrapAdmin(ctx, "other", "different"); err != nil {
		t.Fatal(err)
	}
	admin, err := service.AuthenticatePassword(ctx, "ADMIN", "admin")
	if err != nil || admin.Username != "admin" || !admin.IsAdmin() {
		t.Fatalf("AuthenticatePassword() = %#v, %v", admin, err)
	}
	created, err := service.CreateUser(ctx, admin, "Алиса", "password-123", domain.UserRoleUser,
		[]domain.Permission{domain.PermissionDashboardAccess})
	if err != nil || created.Username != "алиса" || created.Role != domain.UserRoleUser {
		t.Fatalf("CreateUser() = %#v, %v", created, err)
	}
	if _, err := service.CreateUser(ctx, admin, "Алиса", "password-123", domain.UserRoleUser, nil); !errors.Is(err, ports.ErrAlreadyExists) {
		t.Fatalf("duplicate CreateUser() error = %v", err)
	}
	salt := "random-salt"
	token := fmt.Sprintf("%x", md5.Sum([]byte("password-123"+salt)))
	user, err := service.AuthenticateToken(ctx, "алиса", token, salt)
	if err != nil || user.Username != "алиса" {
		t.Fatalf("AuthenticateToken() = %#v, %v", user, err)
	}
	if !user.HasPermission(domain.PermissionDashboardAccess) {
		t.Fatalf("authenticated user permissions = %#v", user.Permissions)
	}
	if _, err := service.Users(ctx, user); !errors.Is(err, ports.ErrForbidden) {
		t.Fatalf("non-admin Users() error = %v", err)
	}
	updated, err := service.UpdatePermissions(ctx, admin, user.Username, []domain.Permission{
		domain.PermissionUsersManage, domain.PermissionDashboardAccess, domain.PermissionUsersManage,
	})
	if err != nil || len(updated.Permissions) != 2 || updated.Permissions[0] != domain.PermissionDashboardAccess {
		t.Fatalf("UpdatePermissions() = %#v, %v", updated, err)
	}
	user, err = service.AuthenticatePassword(ctx, user.Username, "password-123")
	if err != nil || !user.HasPermission(domain.PermissionUsersManage) {
		t.Fatalf("delegated AuthenticatePassword() = %#v, %v", user, err)
	}
	if _, err := service.Users(ctx, user); err != nil {
		t.Fatalf("delegated Users() error = %v", err)
	}
	if _, err := service.UpdatePermissions(ctx, user, user.Username, nil); !errors.Is(err, ports.ErrForbidden) {
		t.Fatalf("delegated UpdatePermissions() error = %v", err)
	}
	if _, err := service.CreateUser(ctx, user, "second-admin", "password-123", domain.UserRoleAdmin, nil); !errors.Is(err, ports.ErrForbidden) {
		t.Fatalf("delegated administrator CreateUser() error = %v", err)
	}
	if err := service.UpdatePassword(ctx, user, admin.Username, "new-admin-password"); !errors.Is(err, ports.ErrForbidden) {
		t.Fatalf("delegated administrator UpdatePassword() error = %v", err)
	}
	if err := service.DeleteUser(ctx, user, admin.Username); !errors.Is(err, ports.ErrForbidden) {
		t.Fatalf("delegated administrator DeleteUser() error = %v", err)
	}
	if err := service.UpdatePassword(ctx, user, user.Username, "new-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AuthenticatePassword(ctx, user.Username, "new-password"); err != nil {
		t.Fatalf("updated password authentication: %v", err)
	}
	if err := service.DeleteUser(ctx, admin, admin.Username); !errors.Is(err, ErrInvalidUser) {
		t.Fatalf("self DeleteUser() error = %v", err)
	}
	if err := service.DeleteUser(ctx, admin, user.Username); err != nil {
		t.Fatal(err)
	}
}

func TestUserServiceValidatesNewCredentials(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	service := NewUserService(newUserStoreStub(), credentialCodecStub{})
	admin := domain.User{Username: "admin", Role: domain.UserRoleAdmin}
	for _, test := range []struct {
		username string
		password string
		role     domain.UserRole
	}{
		{username: "bad name", password: "password", role: domain.UserRoleUser},
		{username: "user", password: "short", role: domain.UserRoleUser},
		{username: "user", password: "password", role: "owner"},
	} {
		if _, err := service.CreateUser(ctx, admin, test.username, test.password, test.role, nil); !errors.Is(err, ErrInvalidUser) {
			t.Fatalf("CreateUser(%q) error = %v", test.username, err)
		}
	}
}
