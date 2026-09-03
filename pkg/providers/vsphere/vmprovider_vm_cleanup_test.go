// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func vmCleanupTests() {
	const zoneName = "az-1"

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

		// Explicitly place the VM into one of the zones that the test context will create.
		vm.Labels[corev1.LabelTopologyZone] = zoneName
	})

	JustBeforeEach(func() {
		ctx, vmProvider, _ = setupVMTest(
			parentCtx, testConfig, vmClass, vm, initObjects...)

		Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
	})

	AfterEach(func() {
		vmTestAfterEach(ctx, vm)

		vmClass = nil
		vm = nil

		ctx = nil
		initObjects = nil
		vmProvider = nil
	})

	It("successfully cleans up VM service state", func() {
		vcVM, err := ctx.Finder.VirtualMachine(ctx, vm.Name)
		Expect(err).NotTo(HaveOccurred())

		// Set up some VM Operator managed fields
		configSpec := vimtypes.VirtualMachineConfigSpec{
			ExtraConfig: []vimtypes.BaseOptionValue{
				&vimtypes.OptionValue{
					Key:   constants.ExtraConfigVMServiceNamespacedName,
					Value: vm.Namespace + "/" + vm.Name,
				},
				&vimtypes.OptionValue{
					Key:   "guestinfo.userdata",
					Value: "test-data",
				},
			},
			ManagedBy: &vimtypes.ManagedByInfo{
				ExtensionKey: vmopv1.ManagedByExtensionKey,
				Type:         vmopv1.ManagedByExtensionType,
			},
		}

		task, err := vcVM.Reconfigure(ctx, configSpec)
		Expect(err).NotTo(HaveOccurred())
		Expect(task.Wait(ctx)).To(Succeed())

		// Verify fields are set before cleanup
		var moVMBefore mo.VirtualMachine
		err = vcVM.Properties(ctx, vcVM.Reference(), []string{"config"}, &moVMBefore)
		Expect(err).NotTo(HaveOccurred())
		Expect(moVMBefore.Config).ToNot(BeNil())
		Expect(moVMBefore.Config.ManagedBy).ToNot(BeNil())
		Expect(moVMBefore.Config.ManagedBy.ExtensionKey).To(Equal(vmopv1.ManagedByExtensionKey))

		// Run cleanup
		Expect(vmProvider.CleanupVirtualMachine(ctx, vm)).To(Succeed())

		// Verify VM Operator managed fields were removed
		var moVMAfter mo.VirtualMachine
		err = vcVM.Properties(ctx, vcVM.Reference(), []string{"config"}, &moVMAfter)
		Expect(err).NotTo(HaveOccurred())
		Expect(moVMAfter.Config).ToNot(BeNil())

		// Check ExtraConfig
		ecList := object.OptionValueList(moVMAfter.Config.ExtraConfig)
		val, ok := ecList.Get(constants.ExtraConfigVMServiceNamespacedName)
		Expect(!ok || val == "").To(BeTrue(), "Expected vmservice.namespacedName to be removed")

		val2, ok2 := ecList.Get("guestinfo.userdata")
		Expect(!ok2 || val2 == "").To(BeTrue(), "Expected guestinfo.userdata to be removed")

		// Check ManagedBy
		Expect(moVMAfter.Config.ManagedBy).To(BeNil())

		// Verify VM still exists in vCenter
		Expect(ctx.GetVMFromMoID(vm.Status.UniqueID)).ToNot(BeNil())
	})

	It("returns success when VM does not exist in vCenter", func() {
		// Delete the VM from vCenter first
		Expect(vmProvider.DeleteVirtualMachine(ctx, vm)).To(Succeed())

		// Cleanup should succeed even though VM doesn't exist
		Expect(vmProvider.CleanupVirtualMachine(ctx, vm)).To(Succeed())
	})
}
