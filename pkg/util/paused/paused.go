// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package paused

import (
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"github.com/vmware-tanzu/vm-operator/pkg/util/annotations"
)

func ByDevOps(vm *vmopv1.VirtualMachine) bool {
	return annotations.HasPaused(vm)
}

func ByAdmin(moVM mo.VirtualMachine) bool {
	if moVM.Config == nil {
		return false
	}
	return object.OptionValueList(moVM.Config.ExtraConfig).
		IsTrue(vmopv1.PauseVMExtraConfigKey)
}
