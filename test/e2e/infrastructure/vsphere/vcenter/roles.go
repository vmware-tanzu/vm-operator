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

// GrantExtraPrivileges grants extraPrivileges to whoever holds baseRoleName on each entity,
// by cloning baseRoleName into tempRoleName, swapping that role onto every entity, then
// adding extraPrivileges to the clone's definition (unlike SetEntityPermissions, a role edit
// isn't checked for privilege escalation). restore (always non-nil) undoes everything.
func GrantExtraPrivileges(
	ctx context.Context,
	vimClient *vim25.Client,
	baseRoleName, tempRoleName string,
	extraPrivileges []string,
	entities ...types.ManagedObjectReference,
) (restore func(context.Context) error, err error) {
	noop := func(context.Context) error { return nil }

	baseRole, err := GetRoleByName(ctx, vimClient, baseRoleName)
	if err != nil {
		return noop, fmt.Errorf("failed to look up %q role: %w", baseRoleName, err)
	}
	if baseRole == nil {
		return noop, fmt.Errorf("role %q not found", baseRoleName)
	}

	// CreateOrUpdateRole resets tempRoleName to exactly baseRole's current privileges,
	// whether newly created or left stale by an interrupted run.
	tempRoleID, err := CreateOrUpdateRole(ctx, vimClient, tempRoleName, baseRole.Privilege)
	if err != nil {
		return noop, fmt.Errorf("failed to create %q role: %w", tempRoleName, err)
	}

	var undos []func(context.Context) error
	restore = func(ctx context.Context) error {
		errs := make([]error, 0, len(undos)+1)
		for _, undo := range undos {
			errs = append(errs, undo(ctx))
		}
		// failIfUsed=true: if an undo above failed, the permission still points at
		// tempRoleID, and failIfUsed=false would have VC delete that permission along
		// with the role -- erasing WCP's real grant instead of just leaking a temp role.
		errs = append(errs, RemoveRole(ctx, vimClient, tempRoleID, true))
		return errors.Join(errs...)
	}

	for _, entity := range entities {
		undo, err := swapEntityPermissionRole(ctx, vimClient, entity, baseRole.RoleId, tempRoleID)
		if err != nil {
			return restore, err
		}
		undos = append(undos, undo)
	}

	// tempRoleName currently holds exactly baseRole.Privilege (see CreateOrUpdateRole above),
	// so merging against it here -- rather than re-reading the role back -- is sufficient.
	merged := slices.Clone(baseRole.Privilege)
	for _, p := range extraPrivileges {
		if !slices.Contains(merged, p) {
			merged = append(merged, p)
		}
	}
	if len(merged) != len(baseRole.Privilege) {
		if err := UpdateRole(ctx, vimClient, tempRoleID, tempRoleName, merged); err != nil {
			return restore, fmt.Errorf("failed to add privileges to role %q: %w", tempRoleName, err)
		}
	}

	return restore, nil
}

// swapEntityPermissionRole finds the entity permissions that currently grant fromRoleID and
// re-points them at toRoleID, leaving the principal, group, and propagate settings unchanged.
// It returns a func that reverts those permissions back to fromRoleID.
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

	// Every permission granting fromRoleID is swapped, not just the first: WCP typically
	// grants the role to a group, so the test account may reach the entity through any one
	// of several group permissions. A permission already on toRoleID is a leftover from a
	// run interrupted mid-swap; treat it as originally fromRoleID so the undo below still
	// restores the pre-interruption state.
	var originals, swapped []types.Permission
	for _, p := range perms {
		if p.RoleId != fromRoleID && p.RoleId != toRoleID {
			continue
		}

		original := p
		original.RoleId = fromRoleID
		originals = append(originals, original)

		s := p
		s.RoleId = toRoleID
		swapped = append(swapped, s)
	}
	if len(originals) == 0 {
		return nil, fmt.Errorf("no permission granting role %d found on %s %s", fromRoleID, entity.Type, entity.Value)
	}

	if err := authzManager.SetEntityPermissions(ctx, entity, swapped); err != nil {
		return nil, fmt.Errorf("failed to set role %d on %s %s: %w", toRoleID, entity.Type, entity.Value, err)
	}

	return func(ctx context.Context) error {
		return authzManager.SetEntityPermissions(ctx, entity, originals)
	}, nil
}

// RemoveRole removes the role with specified id in VC. When failIfUsed is true VC
// rejects the removal if any permission still references the role; when false VC
// deletes every permission that references it.
func RemoveRole(ctx context.Context, vimClient *vim25.Client, roleID int32, failIfUsed bool) error {
	authzManager := object.NewAuthorizationManager(vimClient)

	err := authzManager.RemoveRole(ctx, roleID, failIfUsed)
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
