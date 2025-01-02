// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vm

import (
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
)

func ManagedObjectFromMoRef(moRef vimtypes.ManagedObjectReference) mo.VirtualMachine {
	return mo.VirtualMachine{
		ManagedEntity: mo.ManagedEntity{
			ExtensibleManagedObject: mo.ExtensibleManagedObject{
				Self: moRef,
			},
		},
	}
}

func ManagedObjectFromObject(obj *object.VirtualMachine) mo.VirtualMachine {
	return ManagedObjectFromMoRef(obj.Reference())
}
