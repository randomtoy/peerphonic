package domain

import "time"

type UserRole string

const (
	UserRoleAdmin UserRole = "admin"
	UserRoleUser  UserRole = "user"
)

type User struct {
	Username  string
	Role      UserRole
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (u User) IsAdmin() bool { return u.Role == UserRoleAdmin }
