// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha5_test

import (
	"fmt"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ExampleNamespacedName() {
	obj := vmopv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "my-namespace-1",
			Name:      "my-object-1",
		},
	}
	fmt.Println(vmopv1.NamespacedName(&obj))
	// Output: my-namespace-1/my-object-1
}

func ExampleNamespacedName_nilInterface() {
	var obj *vmopv1.VirtualMachine
	fmt.Println(vmopv1.NamespacedName(obj))
	// Output:
}

func ExampleNamespacedName_nilInput() {
	fmt.Println(vmopv1.NamespacedName(nil))
	// Output:
}

func ExampleNamespacedName_emptyNamespaceAndEmptyName() {
	obj := vmopv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{},
	}
	fmt.Println(vmopv1.NamespacedName(&obj))
	// Output:
}

func ExampleNamespacedName_emptyName() {
	obj := vmopv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "my-namespace-1",
		},
	}
	fmt.Println(vmopv1.NamespacedName(&obj))
	// Output: my-namespace-1/
}

func ExampleNamespacedName_emptyNamespace() {
	obj := vmopv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-object-1",
		},
	}
	fmt.Println(vmopv1.NamespacedName(&obj))
	// Output: /my-object-1
}
