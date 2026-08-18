package services

import (
	"context"
	"crypto/md5"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

var ErrInvalidUser = errors.New("invalid user")

type UserService struct {
	store ports.UserStore
	codec ports.CredentialCodec
	mu    sync.Mutex
}

func NewUserService(store ports.UserStore, codec ports.CredentialCodec) *UserService {
	return &UserService{store: store, codec: codec}
}

func (s *UserService) EnsureBootstrapAdmin(ctx context.Context, username, password string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	users, err := s.store.Users(ctx)
	if err != nil {
		return fmt.Errorf("list users before bootstrap: %w", err)
	}
	if len(users) != 0 {
		return nil
	}
	username, err = normalizeUsername(username)
	if err != nil {
		return err
	}
	if password == "" || len([]byte(password)) > 72 {
		return fmt.Errorf("%w: bootstrap password must contain 1 to 72 bytes", ErrInvalidUser)
	}
	credential, err := s.newCredential(username, password, domain.UserRoleAdmin)
	if err != nil {
		return err
	}
	if err := s.store.CreateUser(ctx, credential); err != nil {
		return fmt.Errorf("create bootstrap administrator: %w", err)
	}
	return nil
}

func (s *UserService) AuthenticatePassword(
	ctx context.Context, username, password string,
) (domain.User, error) {
	username, err := normalizeUsername(username)
	if err != nil {
		return domain.User{}, ports.ErrAuthenticationFailed
	}
	credential, err := s.store.UserCredential(ctx, username)
	if err != nil {
		return domain.User{}, ports.ErrAuthenticationFailed
	}
	if err := s.codec.Verify(credential.PasswordHash, password); err != nil {
		return domain.User{}, ports.ErrAuthenticationFailed
	}
	return credential.User, nil
}

func (s *UserService) AuthenticateToken(
	ctx context.Context, username, token, salt string,
) (domain.User, error) {
	username, err := normalizeUsername(username)
	if err != nil || salt == "" || len(token) != md5.Size*2 {
		return domain.User{}, ports.ErrAuthenticationFailed
	}
	if _, err := hex.DecodeString(token); err != nil {
		return domain.User{}, ports.ErrAuthenticationFailed
	}
	credential, err := s.store.UserCredential(ctx, username)
	if err != nil {
		return domain.User{}, ports.ErrAuthenticationFailed
	}
	password, err := s.codec.DecryptToken(credential.EncryptedToken)
	if err != nil {
		return domain.User{}, ports.ErrAuthenticationFailed
	}
	expected := fmt.Sprintf("%x", md5.Sum(append(password, []byte(salt)...)))
	if subtle.ConstantTimeCompare([]byte(strings.ToLower(token)), []byte(expected)) != 1 {
		return domain.User{}, ports.ErrAuthenticationFailed
	}
	return credential.User, nil
}

func (s *UserService) Users(ctx context.Context, actor domain.User) ([]domain.User, error) {
	if !actor.IsAdmin() {
		return nil, ports.ErrForbidden
	}
	return s.store.Users(ctx)
}

func (s *UserService) CreateUser(
	ctx context.Context,
	actor domain.User,
	username, password string,
	role domain.UserRole,
) (domain.User, error) {
	if !actor.IsAdmin() {
		return domain.User{}, ports.ErrForbidden
	}
	username, err := normalizeUsername(username)
	if err != nil {
		return domain.User{}, err
	}
	if err := validatePassword(password); err != nil {
		return domain.User{}, err
	}
	if role != domain.UserRoleAdmin && role != domain.UserRoleUser {
		return domain.User{}, fmt.Errorf("%w: role must be admin or user", ErrInvalidUser)
	}
	credential, err := s.newCredential(username, password, role)
	if err != nil {
		return domain.User{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.store.CreateUser(ctx, credential); err != nil {
		return domain.User{}, err
	}
	return credential.User, nil
}

func (s *UserService) UpdatePassword(
	ctx context.Context, actor domain.User, username, password string,
) error {
	username, err := normalizeUsername(username)
	if err != nil {
		return err
	}
	if !actor.IsAdmin() && actor.Username != username {
		return ports.ErrForbidden
	}
	if err := validatePassword(password); err != nil {
		return err
	}
	passwordHash, encryptedToken, err := s.codec.Encode(password)
	if err != nil {
		return fmt.Errorf("encode user credentials: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.store.UpdateUserPassword(ctx, username, passwordHash, encryptedToken)
}

func (s *UserService) DeleteUser(ctx context.Context, actor domain.User, username string) error {
	if !actor.IsAdmin() {
		return ports.ErrForbidden
	}
	username, err := normalizeUsername(username)
	if err != nil {
		return err
	}
	if actor.Username == username {
		return fmt.Errorf("%w: administrators cannot delete their own account", ErrInvalidUser)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	users, err := s.store.Users(ctx)
	if err != nil {
		return err
	}
	for _, user := range users {
		if user.Username != username || !user.IsAdmin() {
			continue
		}
		admins := 0
		for _, candidate := range users {
			if candidate.IsAdmin() {
				admins++
			}
		}
		if admins == 1 {
			return ports.ErrLastAdministrator
		}
		break
	}
	return s.store.DeleteUser(ctx, username)
}

func (s *UserService) newCredential(
	username, password string, role domain.UserRole,
) (ports.UserCredential, error) {
	passwordHash, encryptedToken, err := s.codec.Encode(password)
	if err != nil {
		return ports.UserCredential{}, fmt.Errorf("encode user credentials: %w", err)
	}
	now := time.Now().UTC()
	return ports.UserCredential{
		User:         domain.User{Username: username, Role: role, CreatedAt: now, UpdatedAt: now},
		PasswordHash: passwordHash, EncryptedToken: encryptedToken,
	}, nil
}

func normalizeUsername(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	count := utf8.RuneCountInString(value)
	if count < 1 || count > 64 {
		return "", fmt.Errorf("%w: username must contain 1 to 64 characters", ErrInvalidUser)
	}
	for _, character := range value {
		if unicode.IsLetter(character) || unicode.IsDigit(character) || strings.ContainsRune("._-", character) {
			continue
		}
		return "", fmt.Errorf("%w: username contains an unsupported character", ErrInvalidUser)
	}
	return value, nil
}

func validatePassword(password string) error {
	length := len([]byte(password))
	if length < 8 || length > 72 {
		return fmt.Errorf("%w: password must contain 8 to 72 bytes", ErrInvalidUser)
	}
	return nil
}
