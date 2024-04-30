// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vm

import (
	"strings"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
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

func IsPausedByAdmin(moVM *mo.VirtualMachine) bool {
	for i := range moVM.Config.ExtraConfig {
		if o := moVM.Config.ExtraConfig[i].GetOptionValue(); o != nil {
			if o.Key == vmopv1.PauseVMExtraConfigKey {
				if value, ok := o.Value.(string); ok {
					return strings.ToUpper(value) == constants.ExtraConfigTrue
				}
			}
		}
	}
	return false
}
