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
	PermissionSoulseekSearch  Permission = "soulseek.search"
	PermissionSoulseekAdd     Permission = "soulseek.add"
	PermissionSoulseekClient  Permission = "soulseek.client-search"
	PermissionUsersManage     Permission = "users.manage"
	PermissionCatalogManage   Permission = "catalog.manage"
)

var availablePermissions = [...]Permission{
	PermissionDashboardAccess,
	PermissionMonitoringView,
	PermissionSourcesManage,
	PermissionSoulseekSearch,
	PermissionSoulseekAdd,
	PermissionSoulseekClient,
	PermissionUsersManage,
	PermissionCatalogManage,
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
	hasSourceManagement := false
	for _, candidate := range u.Permissions {
		if candidate == permission {
			return true
		}
		if candidate == PermissionSourcesManage {
			hasSourceManagement = true
		}
	}
	return hasSourceManagement && (permission == PermissionSoulseekSearch || permission == PermissionSoulseekAdd)
}

func (u User) EffectivePermissions() []Permission {
	permissions := make([]Permission, 0, len(availablePermissions))
	for _, permission := range availablePermissions {
		if u.HasPermission(permission) {
			permissions = append(permissions, permission)
		}
	}
	return permissions
}

func ValidPermission(permission Permission) bool {
	for _, candidate := range availablePermissions {
		if candidate == permission {
			return true
		}
	}
	return false
}
