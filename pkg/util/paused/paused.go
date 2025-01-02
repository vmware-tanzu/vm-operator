// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package paused

import (
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
)

func ByDevOps(vm *vmopv1.VirtualMachine) bool {
	return metav1.HasAnnotation(vm.ObjectMeta, vmopv1.PauseAnnotation)
}

func ByAdmin(moVM mo.VirtualMachine) bool {
	if moVM.Config == nil {
		return false
	}
	return object.OptionValueList(moVM.Config.ExtraConfig).
		IsTrue(vmopv1.PauseVMExtraConfigKey)
}
