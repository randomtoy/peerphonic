package domain

import "testing"

func TestSourceManagementIncludesDashboardSoulseekCapabilities(t *testing.T) {
	t.Parallel()

	user := User{Role: UserRoleUser, Permissions: []Permission{PermissionSourcesManage}}
	if !user.HasPermission(PermissionSoulseekSearch) || !user.HasPermission(PermissionSoulseekAdd) {
		t.Fatalf("source manager permissions = %#v", user.EffectivePermissions())
	}
	if user.HasPermission(PermissionSoulseekClient) {
		t.Fatal("source management unexpectedly enabled OpenSubsonic Soulseek search")
	}
}

func TestSoulseekCapabilitiesCanBeDelegatedIndependently(t *testing.T) {
	t.Parallel()

	user := User{Role: UserRoleUser, Permissions: []Permission{
		PermissionDashboardAccess, PermissionSoulseekSearch, PermissionSoulseekClient,
	}}
	if !user.HasPermission(PermissionSoulseekSearch) || !user.HasPermission(PermissionSoulseekClient) {
		t.Fatalf("delegated permissions = %#v", user.EffectivePermissions())
	}
	if user.HasPermission(PermissionSoulseekAdd) || user.HasPermission(PermissionSourcesManage) {
		t.Fatalf("unexpected delegated permissions = %#v", user.EffectivePermissions())
	}
}
