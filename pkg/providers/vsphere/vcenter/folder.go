// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vcenter

import (
	"context"
	"fmt"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25"
	vimtypes "github.com/vmware/govmomi/vim25/types"
)

// GetFolderByMoID returns the vim Folder for the MoID.
func GetFolderByMoID(
	ctx context.Context,
	finder *find.Finder,
	folderMoID string) (*object.Folder, error) {

	o, err := finder.ObjectReference(ctx, vimtypes.ManagedObjectReference{Type: "Folder", Value: folderMoID})
	if err != nil {
		return nil, err
	}

	return o.(*object.Folder), nil
}

// GetChildFolder gets the named child Folder from the parent Folder.
func GetChildFolder(
	ctx context.Context,
	parentFolder *object.Folder,
	childName string) (*object.Folder, error) {

	childFolder, err := findChildFolder(ctx, parentFolder, childName)
	if err != nil {
		return nil, err
	} else if childFolder == nil {
		return nil, fmt.Errorf("folder child %s not found under parent Folder %s",
			childName, parentFolder.Reference().Value)
	}

	return childFolder, nil
}

// DoesChildFolderExist returns if the named child Folder exists under the parent Folder.
func DoesChildFolderExist(
	ctx context.Context,
	vimClient *vim25.Client,
	parentFolderMoID, childName string) (bool, error) {

	parentFolder := object.NewFolder(vimClient,
		vimtypes.ManagedObjectReference{Type: "Folder", Value: parentFolderMoID})

	childFolder, err := findChildFolder(ctx, parentFolder, childName)
	if err != nil {
		return false, err
	}

	return childFolder != nil, nil
}

// CreateFolder creates the named child Folder under the parent Folder.
func CreateFolder(
	ctx context.Context,
	vimClient *vim25.Client,
	parentFolderMoID, childName string) (string, error) {

	parentFolder := object.NewFolder(vimClient,
		vimtypes.ManagedObjectReference{Type: "Folder", Value: parentFolderMoID})

	childFolder, err := findChildFolder(ctx, parentFolder, childName)
	if err != nil {
		return "", err
	}

	if childFolder == nil {
		folder, err := parentFolder.CreateFolder(ctx, childName)
		if err != nil {
			return "", err
		}

		childFolder = folder
	}

	return childFolder.Reference().Value, nil
}

// DeleteChildFolder deletes the child Folder under the parent Folder.
func DeleteChildFolder(
	ctx context.Context,
	vimClient *vim25.Client,
	parentFolderMoID, childName string) error {

	parentFolder := object.NewFolder(vimClient,
		vimtypes.ManagedObjectReference{Type: "Folder", Value: parentFolderMoID})

	childFolder, err := findChildFolder(ctx, parentFolder, childName)
	if err != nil || childFolder == nil {
		return err
	}

	task, err := childFolder.Destroy(ctx)
	if err != nil {
		return err
	}

	if taskResult, err := task.WaitForResult(ctx); err != nil {
		if taskResult == nil || taskResult.Error == nil {
			return err
		}
		return fmt.Errorf("destroy Folder %s task failed: %w: %s",
			childFolder.Reference().Value, err, taskResult.Error.LocalizedMessage)
	}

	return nil
}

func findChildFolder(
	ctx context.Context,
	parentFolder *object.Folder,
	childName string) (*object.Folder, error) {

	objRef, err := object.NewSearchIndex(parentFolder.Client()).FindChild(ctx, parentFolder.Reference(), childName)
	if err != nil {
		return nil, err
	} else if objRef == nil {
		return nil, nil
	}

	folder, ok := objRef.(*object.Folder)
	if !ok {
		return nil, fmt.Errorf("Folder child %q is not Folder but a %T", childName, objRef) //nolint:staticcheck
	}

	return folder, nil
}
