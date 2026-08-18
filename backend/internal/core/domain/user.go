package domain

import "time"

type UserRole string

type Permission string

const (
	UserRoleAdmin UserRole = "admin"
	UserRoleUser  UserRole = "user"
)

const (
	PermissionDashboardAccess Permission = "dashboard.access"
	PermissionMonitoringView  Permission = "monitoring.view"
	PermissionSourcesManage   Permission = "sources.manage"
	PermissionUsersManage     Permission = "users.manage"
)

var availablePermissions = [...]Permission{
	PermissionDashboardAccess,
	PermissionMonitoringView,
	PermissionSourcesManage,
	PermissionUsersManage,
}

type User struct {
	Username    string
	Role        UserRole
	Permissions []Permission
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (u User) IsAdmin() bool { return u.Role == UserRoleAdmin }

func (u User) HasPermission(permission Permission) bool {
	if u.IsAdmin() {
		return true
	}
	for _, candidate := range u.Permissions {
		if candidate == permission {
			return true
		}
	}
	return false
}

func (u User) EffectivePermissions() []Permission {
	if u.IsAdmin() {
		return append([]Permission(nil), availablePermissions[:]...)
	}
	return append([]Permission(nil), u.Permissions...)
}

func ValidPermission(permission Permission) bool {
	for _, candidate := range availablePermissions {
		if candidate == permission {
			return true
		}
	}
	return false
}
