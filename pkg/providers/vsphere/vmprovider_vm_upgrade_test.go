// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func vmUpgradeTests() {
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

	JustBeforeEach(func() {
		pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
			config.Features.VMSharedDisks = true
			config.Features.AllDisksArePVCs = false
		})
	})
	JustBeforeEach(func() {
		// Create the VM.
		Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())

		// Clear its annotations and update it in K8s.
		vm.Annotations = nil
		Expect(ctx.Client.Update(ctx, vm)).To(Succeed())
	})

	It("should return ErrUpgradeSchema, then ErrUpgradeObject, then ErrBackup, then success", func() {
		Expect(vm.Annotations).To(HaveLen(0))

		// Update the VM and expect ErrUpgradeSchema.
		Expect(vmProvider.CreateOrUpdateVirtualMachine(ctx, vm)).To(
			MatchError(vsphere.ErrUpgradeSchema))

		// Assert that the VM was schema upgraded.
		Expect(vm.Annotations).To(HaveKeyWithValue(
			pkgconst.UpgradedToBuildVersionAnnotationKey,
			pkgcfg.FromContext(ctx).BuildVersion))
		Expect(vm.Annotations).To(HaveKeyWithValue(
			pkgconst.UpgradedToSchemaVersionAnnotationKey,
			vmopv1.GroupVersion.Version))
		Expect(vm.Annotations).ToNot(HaveKey(
			pkgconst.UpgradedToFeatureVersionAnnotationKey))

		// Update the VM again and expect ErrUpgradeObject.
		Expect(vmProvider.CreateOrUpdateVirtualMachine(ctx, vm)).To(
			MatchError(vsphere.ErrUpgradeObject))

		// Assert that the VM was object upgraded.
		Expect(vm.Annotations).To(HaveKeyWithValue(
			pkgconst.UpgradedToBuildVersionAnnotationKey,
			pkgcfg.FromContext(ctx).BuildVersion))
		Expect(vm.Annotations).To(HaveKeyWithValue(
			pkgconst.UpgradedToSchemaVersionAnnotationKey,
			vmopv1.GroupVersion.Version))
		Expect(vm.Annotations).To(HaveKeyWithValue(
			pkgconst.UpgradedToFeatureVersionAnnotationKey,
			vmopv1util.ActivatedFeatureVersion(ctx).String()))

		// Update the VM again and expect ErrBackup.
		Expect(vmProvider.CreateOrUpdateVirtualMachine(ctx, vm)).To(
			MatchError(vsphere.ErrBackup))

		// Update the VM again and expect no error.
		Expect(vmProvider.CreateOrUpdateVirtualMachine(ctx, vm)).To(
			Succeed())
	})
}
