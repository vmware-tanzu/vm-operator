// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha5_test

import (
	"fmt"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha5/common"
)

func ExampleVirtualMachine_ClassAndInstanceName() {
	obj := vmopv1.VirtualMachine{
		Spec: vmopv1.VirtualMachineSpec{
			ClassName: "my-class-1",
			Class: &vmopv1common.LocalObjectRef{
				Name: "my-class-instance-1",
			},
		},
	}
	fmt.Println(obj.ClassAndInstanceName())
	// Output: my-class-1/my-class-instance-1
}

func ExampleVirtualMachine_ClassAndInstanceName_emptyClassNameAndInstance() {
	obj := vmopv1.VirtualMachine{}
	fmt.Println(obj.ClassAndInstanceName())
	// Output:
}

func ExampleVirtualMachine_ClassAndInstanceName_emptyClassName() {
	obj := vmopv1.VirtualMachine{
		Spec: vmopv1.VirtualMachineSpec{
			Class: &vmopv1common.LocalObjectRef{
				Name: "my-class-instance-1",
			},
		},
	}
	fmt.Println(obj.ClassAndInstanceName())
	// Output: /my-class-instance-1
}

func ExampleVirtualMachine_ClassAndInstanceName_emptyClassInstance() {
	obj := vmopv1.VirtualMachine{
		Spec: vmopv1.VirtualMachineSpec{
			ClassName: "my-class-1",
		},
	}
	fmt.Println(obj.ClassAndInstanceName())
	// Output: my-class-1
}
