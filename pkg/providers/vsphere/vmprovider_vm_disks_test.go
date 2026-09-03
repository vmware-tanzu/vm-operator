// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/vmware/govmomi/vim25/mo"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func vmDisksTests() {
	var (
		parentCtx   context.Context
		initObjects []client.Object
		testConfig  builder.VCSimTestConfig
		ctx         *builder.TestContextForVCSim
		vmProvider  providers.VirtualMachineProviderInterface

		vm      *vmopv1.VirtualMachine
		vmClass *vmopv1.VirtualMachineClass
	)

	BeforeEach(func() {
		parentCtx = newVMTestParentContext()
		testConfig = newVMTestConfig()

		vmClass, vm = newVMTestObjects("test-vm")
	})

	JustBeforeEach(func() {
		ctx, vmProvider, _ = setupVMTest(
			parentCtx, testConfig, vmClass, vm, initObjects...)

		pinVMToFirstZone(ctx, vm)
	})

	AfterEach(func() {
		vmTestAfterEach(ctx, vm)

		vmClass = nil
		vm = nil

		ctx = nil
		initObjects = nil
		vmProvider = nil
	})

	Context("VM has thin provisioning", func() {
		BeforeEach(func() {
			if vm.Spec.Advanced == nil {
				vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{}
			}
			vm.Spec.Advanced.DefaultVolumeProvisioningMode = vmopv1.VolumeProvisioningModeThin
		})

		It("Succeeds", func() {
			vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
			Expect(err).ToNot(HaveOccurred())

			var o mo.VirtualMachine
			Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

			_, backing := getVMHomeDisk(o)
			Expect(backing.ThinProvisioned).To(PointTo(BeTrue()))
		})
	})

	XContext("VM has thick provisioning", func() {
		BeforeEach(func() {
			vm.Spec.Advanced.DefaultVolumeProvisioningMode = vmopv1.VolumeProvisioningModeThick
		})

		It("Succeeds", func() {
			vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
			Expect(err).ToNot(HaveOccurred())

			var o mo.VirtualMachine
			Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

			/* vcsim CL deploy has "thick" but that isn't reflected for this disk. */
			_, backing := getVMHomeDisk(o)
			Expect(backing.ThinProvisioned).To(PointTo(BeFalse()))
		})
	})

	XContext("VM has eager zero provisioning", func() {
		BeforeEach(func() {
			if vm.Spec.Advanced == nil {
				vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{}
			}
			vm.Spec.Advanced.DefaultVolumeProvisioningMode = vmopv1.VolumeProvisioningModeThickEagerZero
		})

		It("Succeeds", func() {
			vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
			Expect(err).ToNot(HaveOccurred())

			var o mo.VirtualMachine
			Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

			/* vcsim CL deploy has "eagerZeroedThick" but that isn't reflected for this disk. */
			_, backing := getVMHomeDisk(o)
			Expect(backing.EagerlyScrub).To(PointTo(BeTrue()))
		})
	})

	Context("Should resize root disk", func() {
		It("Succeeds", func() {
			newSize := resource.MustParse("4242Gi")

			if vm.Spec.Advanced == nil {
				vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{}
			}
			vm.Spec.Advanced.BootDiskCapacity = &newSize
			vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
			vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
			Expect(err).ToNot(HaveOccurred())

			var o mo.VirtualMachine
			Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
			disk, _ := getVMHomeDisk(o)
			Expect(disk.CapacityInBytes).To(BeEquivalentTo(newSize.Value()))
		})
	})
}
