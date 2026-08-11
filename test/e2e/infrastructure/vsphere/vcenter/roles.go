// Copyright (c) 2020-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vcenter

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strings"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/ssoadmin"
	"github.com/vmware/govmomi/sts"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/testbed"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/vcenter/invsvc"
)

// roles contains helpers to manage roles with privilege on a vCenter instance.
// This can be useful when testing against authorization.

// AdminRoleName is the name of vCenter's built-in, full-access role. Looking
// the role up by this name yields the role whose privileges cover every
// operation, without hardcoding a role ID that varies between testbeds.
const AdminRoleName = "Admin"

// CreateOrUpdateRole adds a new role or updates an existing role in VC with the specified privileges. Returns the role id.
func CreateOrUpdateRole(ctx context.Context, vimClient *vim25.Client, roleName string, privilegeIDs []string) (int32, error) {
	role, err := GetRoleByName(ctx, vimClient, roleName)
	if err != nil {
		return 0, err
	}

	if role == nil {
		return CreateRole(ctx, vimClient, roleName, privilegeIDs)
	} else {
		err = UpdateRole(ctx, vimClient, role.RoleId, roleName, privilegeIDs)
		return role.RoleId, err
	}
}

// GetRoleByName returns the AuthorizationRole with the given name or nil if the role is not found.
func GetRoleByName(ctx context.Context, vimClient *vim25.Client, roleName string) (*types.AuthorizationRole, error) {
	authzManager := object.NewAuthorizationManager(vimClient)

	roleList, err := authzManager.RoleList(ctx)
	if err != nil {
		return nil, err
	}

	role := roleList.ByName(roleName)

	return role, nil
}

// CreateRole adds a new role with the specified privileges in VC, and returns the id of the role.
func CreateRole(ctx context.Context, vimClient *vim25.Client, roleName string, privilegeIDs []string) (int32, error) {
	authzManager := object.NewAuthorizationManager(vimClient)

	roleID, err := authzManager.AddRole(ctx, roleName, privilegeIDs)
	if err != nil {
		return 0, err
	}

	return roleID, nil
}

// UpdateRole updated the specified role with specified privileges in VC.
func UpdateRole(ctx context.Context, vimClient *vim25.Client, roleID int32, roleName string, privilegeIDs []string) error {
	authzManager := object.NewAuthorizationManager(vimClient)

	err := authzManager.UpdateRole(ctx, roleID, roleName, privilegeIDs)
	if err != nil {
		log.Printf("Failed to update the role, newer privileges might not be present: %v", err)
		return err
	}

	return nil
}

// EnsureRolePrivileges makes sure the role with the given id grants at least the specified
// privileges, without removing any privileges it already has. It is a no-op if the role
// already grants every requested privilege.
func EnsureRolePrivileges(ctx context.Context, vimClient *vim25.Client, roleID int32, privilegeIDs []string) error {
	authzManager := object.NewAuthorizationManager(vimClient)

	roleList, err := authzManager.RoleList(ctx)
	if err != nil {
		return fmt.Errorf("failed to list roles: %w", err)
	}

	role := roleList.ById(roleID)
	if role == nil {
		return fmt.Errorf("role %d not found", roleID)
	}

	have := make(map[string]struct{}, len(role.Privilege))
	for _, p := range role.Privilege {
		have[p] = struct{}{}
	}

	merged := role.Privilege
	changed := false
	for _, p := range privilegeIDs {
		if _, ok := have[p]; !ok {
			merged = append(merged, p)
			changed = true
		}
	}

	if !changed {
		return nil
	}

	if err := authzManager.UpdateRole(ctx, roleID, role.Name, merged); err != nil {
		return fmt.Errorf("failed to update role %d (%s): %w", roleID, role.Name, err)
	}

	return nil
}

// RemoveRole removes the role with specified id in VC.
func RemoveRole(ctx context.Context, vimClient *vim25.Client, roleID int32) error {
	authzManager := object.NewAuthorizationManager(vimClient)

	err := authzManager.RemoveRole(ctx, roleID, false)
	if err != nil {
		return err
	}

	return nil
}

func AddToGroup(ctx context.Context, vimClient *vim25.Client, userName, groupName string) error {
	return withSSO(ctx, vimClient, func(c *ssoadmin.Client) error {
		user, err := c.FindUser(ctx, userName)
		if err != nil {
			return err
		}

		if user == nil {
			return fmt.Errorf("user %q not found", userName)
		}

		if err = c.AddUsersToGroup(ctx, groupName, user.Id); err != nil {
			return err
		}

		return nil
	})
}

func withSSO(ctx context.Context, vc *vim25.Client, f func(*ssoadmin.Client) error) error {
	c, err := ssoadmin.NewClient(ctx, vc)
	if err != nil {
		return err
	}

	token, err := sts.NewClient(ctx, vc)
	if err != nil {
		return err
	}

	req := sts.TokenRequest{
		Userinfo: url.UserPassword(testbed.AdminUsername, testbed.AdminPassword),
	}

	header := soap.Header{}

	header.Security, err = token.Issue(ctx, req)
	if err != nil {
		return err
	}

	if err = c.Login(c.WithHeader(ctx, header)); err != nil {
		return err
	}

	defer func() {
		err := c.Logout(ctx)
		if err != nil {
			log.Printf("user logout error: %v", err)
		}
	}()

	return f(c)
}

func withInvSvc(ctx context.Context, vc *vim25.Client, f func(*invsvc.Client) error) error {
	c := invsvc.NewClient(ctx, vc)

	user := url.UserPassword(testbed.AdminUsername, testbed.AdminPassword)

	err := c.Login(ctx, user)
	if err != nil {
		return err
	}

	defer func() {
		err := c.Logout(ctx)
		if err != nil {
			log.Printf("user logout error: %v", err)
		}
	}()

	return f(c)
}

func SetGlobalPermission(ctx context.Context, vimClient *vim25.Client, roleID int32, user string) error {
	return withInvSvc(ctx, vimClient, func(c *invsvc.Client) error {
		return c.AddGlobalAccessControlList(ctx, invsvc.AccessControl{
			Principal: invsvc.Principal{Name: user},
			Roles:     []int64{int64(roleID)},
			Propagate: true,
		})
	})
}

func RemoveGlobalPermission(ctx context.Context, vimClient *vim25.Client, user string) error {
	return withInvSvc(ctx, vimClient, func(c *invsvc.Client) error {
		return c.RemoveGlobalAccess(ctx, invsvc.Principal{Name: user})
	})
}

// SetEntityPermission grants principal the given role directly on a single
// inventory entity. A permission assigned directly to a user takes precedence
// over any permission the user inherits or holds via group membership on that
// same entity, so this can override a more restrictive group-level role (e.g.
// the VM-Service-VM-Management role WCP grants to the Administrators group on a
// namespace's resource pools and folders). Set isGroup when principal names a
// group rather than a user. Pair with RemoveEntityPermission to revert it.
func SetEntityPermission(
	ctx context.Context,
	vimClient *vim25.Client,
	entity types.ManagedObjectReference,
	principal string,
	roleID int32,
	isGroup, propagate bool) error {

	authzManager := object.NewAuthorizationManager(vimClient)

	if err := authzManager.SetEntityPermissions(ctx, entity, []types.Permission{{
		Principal: formatPrincipal(principal),
		Group:     isGroup,
		RoleId:    roleID,
		Propagate: propagate,
	}}); err != nil {
		return fmt.Errorf("failed to set permission for %q on %s %s: %w",
			principal, entity.Type, entity.Value, err)
	}

	return nil
}

// RemoveEntityPermission removes the permission principal holds directly on the
// given inventory entity. Set isGroup when principal names a group rather than a
// user. It is the inverse of SetEntityPermission.
func RemoveEntityPermission(
	ctx context.Context,
	vimClient *vim25.Client,
	entity types.ManagedObjectReference,
	principal string,
	isGroup bool) error {

	authzManager := object.NewAuthorizationManager(vimClient)

	if err := authzManager.RemoveEntityPermission(
		ctx, entity, formatPrincipal(principal), isGroup); err != nil {
		return fmt.Errorf("failed to remove permission for %q on %s %s: %w",
			principal, entity.Type, entity.Value, err)
	}

	return nil
}

// formatPrincipal converts an SSO principal from UPN form (user@domain) to the
// domain\user form the vCenter authorization APIs expect, mirroring the
// conversion the global-permissions client in the invsvc package performs. A
// principal that is not in UPN form is returned unchanged.
func formatPrincipal(principal string) string {
	if parts := strings.SplitN(principal, "@", 2); len(parts) == 2 {
		return parts[1] + `\` + parts[0]
	}
	return principal
}
