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

const vmClassUID = "my-uid"

var _ = Describe("SetLastResizedAnnotation", func() {

	var (
		vm      *vmopv1.VirtualMachine
		vmClass vmopv1.VirtualMachineClass
	)

	BeforeEach(func() {
		vm = builder.DummyVirtualMachine()

		vmClass = *builder.DummyVirtualMachineClass("my-class")
		vmClass.UID = vmClassUID
		vmClass.Generation = 42
	})

	It("Sets expected annotation", func() {
		err := vmopv1util.SetLastResizedAnnotation(vm, vmClass)
		Expect(err).ToNot(HaveOccurred())
		val, ok := vm.Annotations[vmopv1util.LastResizedAnnotationKey]
		Expect(ok).To(BeTrue())
		Expect(val).To(MatchJSON(`{"name":"my-class","uid":"my-uid","generation":42}`))
	})
})

var _ = Describe("SetLastResizedAnnotationClassName", func() {

	var (
		vm      *vmopv1.VirtualMachine
		vmClass vmopv1.VirtualMachineClass
	)

	BeforeEach(func() {
		vm = builder.DummyVirtualMachine()

		vmClass = *builder.DummyVirtualMachineClass("my-class")
		vmClass.UID = vmClassUID
		vmClass.Generation = 42
	})

	It("Sets expected annotation", func() {
		err := vmopv1util.SetLastResizedAnnotationClassName(vm, vmClass.Name)
		Expect(err).ToNot(HaveOccurred())
		val, ok := vm.Annotations[vmopv1util.LastResizedAnnotationKey]
		Expect(ok).To(BeTrue())
		Expect(val).To(MatchJSON(`{"name":"my-class"}`))

		By("GetLastResizedAnnotation also returns expected values", func() {
			className, uid, generation, ok := vmopv1util.GetLastResizedAnnotation(*vm)
			Expect(ok).To(BeTrue())
			Expect(className).To(Equal(vmClass.Name))
			Expect(uid).To(BeEmpty())
			Expect(generation).To(BeZero())
		})
	})
})

var _ = Describe("GetLastResizedAnnotation", func() {

	var (
		vm      *vmopv1.VirtualMachine
		vmClass vmopv1.VirtualMachineClass
	)

	BeforeEach(func() {
		vm = builder.DummyVirtualMachine()

		vmClass = *builder.DummyVirtualMachineClass("my-class")
		vmClass.UID = vmClassUID
		vmClass.Generation = 42
	})

	When("Resize annotation is not present", func() {
		It("Returns expected values", func() {
			className, uid, generation, ok := vmopv1util.GetLastResizedAnnotation(*vm)
			Expect(ok).To(BeFalse())
			Expect(className).To(BeEmpty())
			Expect(uid).To(BeEmpty())
			Expect(generation).To(BeZero())
		})
	})

	When("Resize annotation is present", func() {
		BeforeEach(func() {
			Expect(vmopv1util.SetLastResizedAnnotation(vm, vmClass)).To(Succeed())
		})

		It("Returns expected values", func() {
			className, uid, generation, ok := vmopv1util.GetLastResizedAnnotation(*vm)
			Expect(ok).To(BeTrue())
			Expect(className).To(Equal(vmClass.Name))
			Expect(uid).To(BeEquivalentTo(vmClass.UID))
			Expect(generation).To(Equal(vmClass.Generation))
		})
	})
})

var _ = Describe("ResizeNeeded", func() {

	var (
		vm      vmopv1.VirtualMachine
		vmClass vmopv1.VirtualMachineClass
	)

	BeforeEach(func() {
		vmClass = *builder.DummyVirtualMachineClass("my-class")
		vmClass.UID = vmClassUID
		vmClass.Generation = 42

		vm = *builder.DummyVirtualMachine()
		vm.Name = "vm-need-resize-test"
		vm.Spec.ClassName = vmClass.Name
	})

	Context("Resize annotation is not present", func() {
		It("returns false", func() {
			Expect(vmopv1util.ResizeNeeded(vm, vmClass)).To(BeFalse())
		})

		When("same-class annotation is present", func() {
			BeforeEach(func() {
				vm.Annotations[vmopv1.VirtualMachineSameVMClassResizeAnnotation] = ""
			})

			It("returns true", func() {
				Expect(vmopv1util.ResizeNeeded(vm, vmClass)).To(BeTrue())
			})
		})
	})

	Context("Resize annotation is present", func() {
		BeforeEach(func() {
			Expect(vmopv1util.SetLastResizedAnnotation(&vm, vmClass)).To(Succeed())
		})

		When("Spec.ClassName is the same", func() {
			When("same-class annotation is not present", func() {
				It("returns false", func() {
					Expect(vmopv1util.ResizeNeeded(vm, vmClass)).To(BeFalse())
				})
			})

			When("same-class annotation is present", func() {
				BeforeEach(func() {
					vm.Annotations[vmopv1.VirtualMachineSameVMClassResizeAnnotation] = ""
				})

				When("class is unchanged", func() {
					It("returns false", func() {
						Expect(vmopv1util.ResizeNeeded(vm, vmClass)).To(BeFalse())
					})
				})
				When("class is changed", func() {
					BeforeEach(func() {
						vmClass.Generation++
					})
					It("returns true", func() {
						Expect(vmopv1util.ResizeNeeded(vm, vmClass)).To(BeTrue())
					})
				})
			})
		})

		When("Spec.ClassName changes", func() {
			BeforeEach(func() {
				vm.Spec.ClassName = "my-new-class"
			})

			It("returns true", func() {
				Expect(vmopv1util.ResizeNeeded(vm, vmClass)).To(BeTrue())
			})
		})
	})
})
