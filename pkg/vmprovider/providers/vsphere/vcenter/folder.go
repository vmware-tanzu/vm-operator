// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vcenter

import (
	goctx "context"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/types"
)

// GetFolderByMoID returns the vim Folder for the MoID.
func GetFolderByMoID(
	ctx goctx.Context,
	finder *find.Finder,
	moID string) (*object.Folder, error) {

	o, err := finder.ObjectReference(ctx, types.ManagedObjectReference{Type: "Folder", Value: moID})
	if err != nil {
		return nil, err
	}

	return o.(*object.Folder), nil
}
