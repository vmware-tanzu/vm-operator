// Copyright (c) 2023-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vm_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/object"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmutil "github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/vm"
)

const doesNotExist = "does-not-exist"

func managedObjectTests() {
	getVMMoRef := func() vimtypes.ManagedObjectReference {
		return vimtypes.ManagedObjectReference{
			Type:  "VirtualMachine",
			Value: "vm-44",
		}
	}

	Context("ManagedObjectFromMoRef", func() {
		It("should return a mo.VirtualMachine from the provided ManagedObjectReference", func() {
			Expect(vmutil.ManagedObjectFromMoRef(getVMMoRef()).Self).To(Equal(getVMMoRef()))
		})
	})
	Context("ManagedObjectFromObject", func() {
		It("should return a mo.VirtualMachine from the provided object.VirtualMachine", func() {
			Expect(vmutil.ManagedObjectFromObject(object.NewVirtualMachine(nil, getVMMoRef())).Reference()).To(Equal(getVMMoRef()))
		})
	})
}
