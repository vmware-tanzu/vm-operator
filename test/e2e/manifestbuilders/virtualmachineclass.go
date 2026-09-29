// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package manifestbuilders

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	e2eframework "k8s.io/kubernetes/test/e2e/framework"

	vmopv1a2 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
)

// GetVirtualMachineClassYaml returns a v1alpha2 VirtualMachineClass YAML
// manifest that has no spec.
//
// The manifest is applied to make a class visible in a namespace, and the
// class may already exist. The spec is omitted, rather than rendered with its
// zero values, so that applying the manifest does not overwrite the spec of an
// existing class.
func GetVirtualMachineClassYaml(namespace, vmClassName string) []byte {
	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(
		VirtualMachineClassA2(namespace, vmClassName))
	if err != nil {
		e2eframework.Failf("Failed to convert VirtualMachineClass to unstructured: %v", err)
	}
	delete(u, "spec")

	return ToYAML(&unstructured.Unstructured{Object: u})
}

// VirtualMachineClassA2 returns an empty v1alpha2 VirtualMachineClass.
func VirtualMachineClassA2(namespace, vmClassName string) *vmopv1a2.VirtualMachineClass {
	return &vmopv1a2.VirtualMachineClass{
		TypeMeta: typeMeta(vmopv1a2.GroupVersion.String(), "VirtualMachineClass"),
		ObjectMeta: metav1.ObjectMeta{
			Name:      vmClassName,
			Namespace: namespace,
		},
	}
}
