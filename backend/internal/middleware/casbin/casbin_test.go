package casbinrbac

import (
	"errors"
	"testing"

	"erent/internal/testdatabase"

	"github.com/casbin/casbin/v3"
	gormadapter "github.com/casbin/gorm-adapter/v3"
)

func TestDefaultRolesCanOnlyViewDashboard(t *testing.T) {
	database := testdatabase.Open(t)
	if err := database.AutoMigrate(&gormadapter.CasbinRule{}); err != nil {
		t.Fatalf("migrate Casbin rules: %v", err)
	}
	enforcer, err := NewEnforcer(database)
	if err != nil {
		t.Fatalf("create persistent enforcer: %v", err)
	}

	for index, roleCode := range []string{RoleUser, RoleAdmin, RoleTest} {
		userID := uint64(index + 1)
		roles, permissions, err := AuthorizationForUser(enforcer, userID, roleCode)
		if err != nil {
			t.Fatalf("authorize %s: %v", roleCode, err)
		}
		if len(roles) != 1 || roles[0] != roleCode {
			t.Fatalf("%s roles = %v", roleCode, roles)
		}
		if len(permissions) != 1 || permissions[0] != PermissionDashboardView {
			t.Fatalf("%s permissions = %v", roleCode, permissions)
		}
		allowed, err := enforcer.Enforce(UserSubject(userID), PermissionDashboardView)
		if err != nil || !allowed {
			t.Fatalf("%s dashboard permission: allowed=%v err=%v", roleCode, allowed, err)
		}
		allowed, err = enforcer.Enforce(UserSubject(userID), "system:user:read")
		if err != nil || allowed {
			t.Fatalf("%s unexpected permission: allowed=%v err=%v", roleCode, allowed, err)
		}
	}
}

func TestAuthorizationRejectsUnsupportedRole(t *testing.T) {
	enforcer, err := NewMemoryEnforcer()
	if err != nil {
		t.Fatalf("create enforcer: %v", err)
	}
	if _, _, err := AuthorizationForUser(enforcer, 1, "operator"); !errors.Is(err, ErrUnsupportedRole) {
		t.Fatalf("error = %v, want %v", err, ErrUnsupportedRole)
	}
}

func TestSeedDefaultPoliciesRemovesExtraPermission(t *testing.T) {
	enforcer, err := NewMemoryEnforcer()
	if err != nil {
		t.Fatalf("create enforcer: %v", err)
	}
	if _, err := enforcer.AddPermissionForUser(RoleSubject(RoleAdmin), "system:user:read"); err != nil {
		t.Fatalf("add extra permission: %v", err)
	}
	if err := SeedDefaultPolicies(enforcer); err != nil {
		t.Fatalf("reseed policies: %v", err)
	}
	permissions := PermissionCodes(mustPermissionsForUser(t, enforcer, RoleSubject(RoleAdmin)))
	if len(permissions) != 1 || permissions[0] != PermissionDashboardView {
		t.Fatalf("permissions = %v", permissions)
	}
}

func mustPermissionsForUser(t *testing.T, enforcer *casbin.SyncedEnforcer, subject string) [][]string {
	t.Helper()
	permissions, err := enforcer.GetPermissionsForUser(subject)
	if err != nil {
		t.Fatalf("get permissions: %v", err)
	}
	return permissions
}
