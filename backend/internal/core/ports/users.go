package ports

import (
	"context"
	"errors"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
)

var (
	ErrAuthenticationFailed = errors.New("authentication failed")
	ErrAlreadyExists        = errors.New("already exists")
	ErrForbidden            = errors.New("forbidden")
	ErrLastAdministrator    = errors.New("cannot remove the last administrator")
)

type UserCredential struct {
	User           domain.User
	PasswordHash   []byte
	EncryptedToken []byte
}

type UserStore interface {
	Users(ctx context.Context) ([]domain.User, error)
	UserCredential(ctx context.Context, username string) (UserCredential, error)
	CreateUser(ctx context.Context, credential UserCredential) error
	UpdateUserPassword(ctx context.Context, username string, passwordHash, encryptedToken []byte) error
	DeleteUser(ctx context.Context, username string) error
}

type CredentialCodec interface {
	Encode(password string) (passwordHash, encryptedToken []byte, err error)
	Verify(passwordHash []byte, password string) error
	DecryptToken(encryptedToken []byte) ([]byte, error)
}

type Authenticator interface {
	AuthenticatePassword(ctx context.Context, username, password string) (domain.User, error)
	AuthenticateToken(ctx context.Context, username, token, salt string) (domain.User, error)
}

type UserManager interface {
	Users(ctx context.Context, actor domain.User) ([]domain.User, error)
	CreateUser(ctx context.Context, actor domain.User, username, password string, role domain.UserRole) (domain.User, error)
	UpdatePassword(ctx context.Context, actor domain.User, username, password string) error
	DeleteUser(ctx context.Context, actor domain.User, username string) error
}
