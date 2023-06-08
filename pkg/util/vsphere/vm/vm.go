// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vm

import (
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

func ManagedObjectFromMoRef(moRef types.ManagedObjectReference) mo.VirtualMachine {
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
