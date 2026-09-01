// Package casbinrbac owns the minimal role and permission policy used by authentication.
package casbinrbac

import (
	_ "embed"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/casbin/casbin/v3"
	"github.com/casbin/casbin/v3/model"
	"github.com/casbin/casbin/v3/persist"
	gormadapter "github.com/casbin/gorm-adapter/v3"
	"gorm.io/gorm"
)

const (
	RoleUser                = "user"
	RoleAdmin               = "admin"
	RoleTest                = "test"
	PermissionDashboardView = "dashboard:view"

	userPrefix = "user:"
	rolePrefix = "role:"
)

var (
	ErrUnsupportedRole = errors.New("unsupported role")
	supportedRoles     = []string{RoleUser, RoleAdmin, RoleTest}
)

//go:embed model.conf
var modelText string

// NewEnforcer creates a persistent Casbin enforcer backed by the existing GORM database.
func NewEnforcer(database *gorm.DB) (*casbin.SyncedEnforcer, error) {
	gormadapter.TurnOffAutoMigrate(database)
	adapter, err := gormadapter.NewAdapterByDB(database)
	if err != nil {
		return nil, fmt.Errorf("create Casbin GORM adapter: %w", err)
	}
	enforcer, err := newEnforcer(adapter)
	if err != nil {
		return nil, err
	}
	if err := SeedDefaultPolicies(enforcer); err != nil {
		return nil, err
	}
	return enforcer, nil
}

// NewMemoryEnforcer creates the same policy engine without persistence for isolated tests.
func NewMemoryEnforcer() (*casbin.SyncedEnforcer, error) {
	enforcer, err := newEnforcer(nil)
	if err != nil {
		return nil, err
	}
	if err := SeedDefaultPolicies(enforcer); err != nil {
		return nil, err
	}
	return enforcer, nil
}

func newEnforcer(adapter persist.Adapter) (*casbin.SyncedEnforcer, error) {
	accessModel, err := model.NewModelFromString(modelText)
	if err != nil {
		return nil, fmt.Errorf("load embedded Casbin model: %w", err)
	}
	var enforcer *casbin.SyncedEnforcer
	if adapter == nil {
		enforcer, err = casbin.NewSyncedEnforcer(accessModel)
	} else {
		enforcer, err = casbin.NewSyncedEnforcer(accessModel, adapter)
	}
	if err != nil {
		return nil, fmt.Errorf("create Casbin enforcer: %w", err)
	}
	enforcer.EnableAutoSave(true)
	return enforcer, nil
}

// SeedDefaultPolicies grants only dashboard viewing to the three supported roles.
func SeedDefaultPolicies(enforcer *casbin.SyncedEnforcer) error {
	for _, roleCode := range supportedRoles {
		subject := RoleSubject(roleCode)
		permissions, err := enforcer.GetPermissionsForUser(subject)
		if err != nil {
			return fmt.Errorf("load permissions for %s: %w", roleCode, err)
		}
		if len(permissions) == 1 && len(permissions[0]) >= 2 && permissions[0][1] == PermissionDashboardView {
			continue
		}
		if _, err := enforcer.DeletePermissionsForUser(subject); err != nil {
			return fmt.Errorf("clear permissions for %s: %w", roleCode, err)
		}
		if _, err := enforcer.AddPermissionForUser(subject, PermissionDashboardView); err != nil {
			return fmt.Errorf("grant dashboard permission to %s: %w", roleCode, err)
		}
	}
	return nil
}

// AuthorizationForUser synchronizes the database role and returns effective Casbin claims.
func AuthorizationForUser(enforcer *casbin.SyncedEnforcer, userID uint64, roleCode string) ([]string, []string, error) {
	roleCode = strings.TrimSpace(roleCode)
	if !IsSupportedRole(roleCode) {
		return nil, nil, fmt.Errorf("%w: %s", ErrUnsupportedRole, roleCode)
	}
	subject := UserSubject(userID)
	desiredRole := RoleSubject(roleCode)
	currentRoles, err := enforcer.GetRolesForUser(subject)
	if err != nil {
		return nil, nil, fmt.Errorf("load direct user roles: %w", err)
	}
	if len(currentRoles) != 1 || currentRoles[0] != desiredRole {
		if _, err := enforcer.DeleteRolesForUser(subject); err != nil {
			return nil, nil, fmt.Errorf("clear user roles: %w", err)
		}
		if _, err := enforcer.AddRoleForUser(subject, desiredRole); err != nil {
			return nil, nil, fmt.Errorf("assign user role: %w", err)
		}
	}
	roleSubjects, err := enforcer.GetImplicitRolesForUser(subject)
	if err != nil {
		return nil, nil, fmt.Errorf("load user roles: %w", err)
	}
	permissionRules, err := enforcer.GetImplicitPermissionsForUser(subject)
	if err != nil {
		return nil, nil, fmt.Errorf("load user permissions: %w", err)
	}
	return RoleCodes(roleSubjects), PermissionCodes(permissionRules), nil
}

func IsSupportedRole(roleCode string) bool {
	switch strings.TrimSpace(roleCode) {
	case RoleUser, RoleAdmin, RoleTest:
		return true
	default:
		return false
	}
}

func UserSubject(userID uint64) string {
	return userPrefix + strconv.FormatUint(userID, 10)
}

func RoleSubject(roleCode string) string {
	return rolePrefix + strings.TrimSpace(roleCode)
}

func RoleCodes(subjects []string) []string {
	roles := make([]string, 0, len(subjects))
	for _, subject := range subjects {
		if strings.HasPrefix(subject, rolePrefix) {
			roles = append(roles, strings.TrimPrefix(subject, rolePrefix))
		}
	}
	sort.Strings(roles)
	return roles
}

func PermissionCodes(rules [][]string) []string {
	set := make(map[string]struct{})
	for _, rule := range rules {
		if len(rule) >= 2 && strings.TrimSpace(rule[1]) != "" {
			set[rule[1]] = struct{}{}
		}
	}
	permissions := make([]string, 0, len(set))
	for permissionCode := range set {
		permissions = append(permissions, permissionCode)
	}
	sort.Strings(permissions)
	return permissions
}
