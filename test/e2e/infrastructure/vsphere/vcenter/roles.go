// Copyright (c) 2020-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vcenter

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"slices"

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

	merged := slices.Clone(role.Privilege)
	for _, p := range privilegeIDs {
		if !slices.Contains(merged, p) {
			merged = append(merged, p)
		}
	}

	if len(merged) == len(role.Privilege) {
		return nil
	}

	if err := authzManager.UpdateRole(ctx, roleID, role.Name, merged); err != nil {
		return fmt.Errorf("failed to update role %d (%s): %w", roleID, role.Name, err)
	}

	return nil
}

// GrantExtraPrivileges grants extraPrivileges, on top of the privileges the role named
// baseRoleName already carries, to whichever principal currently holds baseRoleName on each
// of the given entities.
//
// WCP pins roles such as VM-Service-VM-Management directly onto the objects it manages, and
// that grant overrides whatever role those principals inherit from higher up the inventory.
// Mutating the pinned role in place would affect every object it is granted on across the
// whole vCenter, so this creates a role named tempRoleName and swaps it in for baseRoleName
// on just the given entities instead.
//
// The temporary role starts as an exact clone of baseRoleName and only gains extraPrivileges
// once every entity has been swapped over: vCenter refuses to grant a role carrying
// privileges the acting principal does not already hold on that same entity, whereas editing
// a role's definition afterwards is not subject to that check and takes effect on every
// entity at once. Passing all the entities to a single call is therefore required -- granting
// them across two calls would fail the check on the entities of the second call.
//
// The returned restore func reverts every swap and removes the temporary role. It is always
// non-nil, including on error so that partial work can be unwound, so register it before
// inspecting err. Callers must invoke it -- e.g. via DeferCleanup -- while their vCenter
// session is still authenticated.
func GrantExtraPrivileges(
	ctx context.Context,
	vimClient *vim25.Client,
	baseRoleName, tempRoleName string,
	extraPrivileges []string,
	entities ...types.ManagedObjectReference,
) (restore func(context.Context) error, err error) {
	authzManager := object.NewAuthorizationManager(vimClient)

	// A single RoleList serves both lookups below.
	roleList, err := authzManager.RoleList(ctx)
	if err != nil {
		return func(context.Context) error { return nil }, fmt.Errorf("failed to list roles: %w", err)
	}

	baseRole := roleList.ByName(baseRoleName)
	if baseRole == nil {
		return func(context.Context) error { return nil }, fmt.Errorf("role %q not found", baseRoleName)
	}

	var tempRoleID int32
	if existing := roleList.ByName(tempRoleName); existing != nil {
		// Left behind by an interrupted run; reset it to the clone state.
		tempRoleID = existing.RoleId
		if err := authzManager.UpdateRole(ctx, tempRoleID, tempRoleName, baseRole.Privilege); err != nil {
			return func(context.Context) error { return nil },
				fmt.Errorf("failed to reset role %q: %w", tempRoleName, err)
		}
	} else if tempRoleID, err = authzManager.AddRole(ctx, tempRoleName, baseRole.Privilege); err != nil {
		return func(context.Context) error { return nil },
			fmt.Errorf("failed to create role %q: %w", tempRoleName, err)
	}

	var undos []func(context.Context) error
	restore = func(ctx context.Context) error {
		errs := make([]error, 0, len(undos)+1)
		for _, undo := range undos {
			errs = append(errs, undo(ctx))
		}
		errs = append(errs, RemoveRole(ctx, vimClient, tempRoleID))
		return errors.Join(errs...)
	}

	for _, entity := range entities {
		undo, err := swapEntityPermissionRole(ctx, vimClient, entity, baseRole.RoleId, tempRoleID)
		if err != nil {
			return restore, err
		}
		undos = append(undos, undo)
	}

	if err := EnsureRolePrivileges(ctx, vimClient, tempRoleID, extraPrivileges); err != nil {
		return restore, fmt.Errorf("failed to add privileges to role %q: %w", tempRoleName, err)
	}

	return restore, nil
}

// swapEntityPermissionRole finds the entity permission that currently grants fromRoleID and
// re-points it at toRoleID, leaving the principal, group, and propagate settings unchanged.
// It returns a func that reverts the entity's permission back to fromRoleID.
func swapEntityPermissionRole(
	ctx context.Context,
	vimClient *vim25.Client,
	entity types.ManagedObjectReference,
	fromRoleID, toRoleID int32,
) (func(context.Context) error, error) {
	authzManager := object.NewAuthorizationManager(vimClient)

	perms, err := authzManager.RetrieveEntityPermissions(ctx, entity, false)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve permissions on %s %s: %w", entity.Type, entity.Value, err)
	}

	i := slices.IndexFunc(perms, func(p types.Permission) bool { return p.RoleId == fromRoleID })
	if i < 0 {
		return nil, fmt.Errorf("no permission granting role %d found on %s %s", fromRoleID, entity.Type, entity.Value)
	}

	original := perms[i]
	swapped := original
	swapped.RoleId = toRoleID

	if err := authzManager.SetEntityPermissions(ctx, entity, []types.Permission{swapped}); err != nil {
		return nil, fmt.Errorf("failed to set role %d on %s %s: %w", toRoleID, entity.Type, entity.Value, err)
	}

	return func(ctx context.Context) error {
		return authzManager.SetEntityPermissions(ctx, entity, []types.Permission{original})
	}, nil
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
