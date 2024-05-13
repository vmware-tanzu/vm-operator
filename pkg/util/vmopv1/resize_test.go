// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmopv1_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("SetResizeAnnotation", func() {

	var (
		vm      *vmopv1.VirtualMachine
		vmClass vmopv1.VirtualMachineClass
	)

	BeforeEach(func() {
		vm = builder.DummyVirtualMachine()

		vmClass = *builder.DummyVirtualMachineClass("my-class")
		vmClass.UID = "my-uid"
		vmClass.Generation = 42
	})

	It("Sets expected annotation", func() {
		err := vmopv1util.SetResizeAnnotation(vm, vmClass)
		Expect(err).ToNot(HaveOccurred())
		val, ok := vm.Annotations[vmopv1util.LastResizedAnnotationKey]
		Expect(ok).To(BeTrue())
		Expect(val).To(MatchJSON(`{"Name":"my-class","UID":"my-uid","Generation":42}`))
	})
})

var _ = Describe("GetResizeAnnotation", func() {

	var (
		vm      *vmopv1.VirtualMachine
		vmClass vmopv1.VirtualMachineClass
	)

	BeforeEach(func() {
		vm = builder.DummyVirtualMachine()

		vmClass = *builder.DummyVirtualMachineClass("my-class")
		vmClass.UID = "my-uid"
		vmClass.Generation = 42
	})

	When("Resize annotation is not present", func() {
		It("Returns expected values", func() {
			className, uid, generation, ok := vmopv1util.GetResizeAnnotation(*vm)
			Expect(ok).To(BeFalse())
			Expect(className).To(BeEmpty())
			Expect(uid).To(BeEmpty())
			Expect(generation).To(BeZero())
		})
	})

	When("Resize annotation is present", func() {
		BeforeEach(func() {
			Expect(vmopv1util.SetResizeAnnotation(vm, vmClass)).To(Succeed())
		})

		It("Returns expected values", func() {
			className, uid, generation, ok := vmopv1util.GetResizeAnnotation(*vm)
			Expect(ok).To(BeTrue())
			Expect(className).To(Equal(vmClass.Name))
			Expect(uid).To(BeEquivalentTo(vmClass.UID))
			Expect(generation).To(Equal(vmClass.Generation))
		})
	})
})
