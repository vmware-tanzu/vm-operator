// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package manifestbuilders

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1a1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
)

// GetVirtualMachineClassBindingYaml returns a v1alpha1
// VirtualMachineClassBinding YAML manifest.
func GetVirtualMachineClassBindingYaml(namespace, vmClassName string) []byte {
	return ToYAML(VirtualMachineClassBindingA1(namespace, vmClassName))
}

// VirtualMachineClassBindingA1 returns a v1alpha1 VirtualMachineClassBinding
// for the VirtualMachineClass vmClassName.
func VirtualMachineClassBindingA1(namespace, vmClassName string) *vmopv1a1.VirtualMachineClassBinding {
	return &vmopv1a1.VirtualMachineClassBinding{
		TypeMeta: typeMeta(vmopv1a1.GroupVersion.String(), "VirtualMachineClassBinding"),
		ObjectMeta: metav1.ObjectMeta{
			Name:      vmClassName,
			Namespace: namespace,
		},
		ClassRef: vmopv1a1.ClassReference{
			APIVersion: vmopv1a1.GroupVersion.String(),
			Kind:       "VirtualMachineClass",
			Name:       vmClassName,
		},
	}
}
