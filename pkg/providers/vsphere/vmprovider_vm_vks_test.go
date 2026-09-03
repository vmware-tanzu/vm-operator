// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	backupapi "github.com/vmware-tanzu/vm-operator/pkg/backup/api"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func vmVKSTests() {
	var (
		parentCtx   context.Context
		initObjects []client.Object
		testConfig  builder.VCSimTestConfig
		ctx         *builder.TestContextForVCSim
		vmProvider  providers.VirtualMachineProviderInterface

		vm      *vmopv1.VirtualMachine
		vmClass *vmopv1.VirtualMachineClass

		vcVM *object.VirtualMachine
		moVM mo.VirtualMachine

		optIntoBackup bool
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

		if vm.Labels == nil {
			vm.Labels = make(map[string]string)
		}
		vm.Labels[kubeutil.CAPVClusterRoleLabelKey] = ""
		vm.Labels[kubeutil.CAPWClusterRoleLabelKey] = ""

		if optIntoBackup {
			if vm.Annotations == nil {
				vm.Annotations = make(map[string]string)
			}
			vm.Annotations[vmopv1.ForceEnableBackupAnnotation] = "true"
		}

		var err error
		vcVM, err = createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
		Expect(err).ToNot(HaveOccurred())

		Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &moVM)).To(Succeed())

	})

	AfterEach(func() {
		optIntoBackup = false

		vmTestAfterEach(ctx, vm)

		vmClass = nil
		vm = nil
		vcVM = nil
		moVM = mo.VirtualMachine{}

		ctx = nil
		initObjects = nil
		vmProvider = nil
	})

	It("should not have any backup ExtraConfig key", func() {
		Expect(moVM.Config.ExtraConfig).ToNot(BeNil())
		ecMap := pkgutil.OptionValues(moVM.Config.ExtraConfig).StringMap()
		Expect(ecMap).ToNot(HaveKey(backupapi.VMResourceYAMLExtraConfigKey))
	})

	When("node opts into backup", func() {
		BeforeEach(func() {
			optIntoBackup = true
		})
		It("should have backup ExtraConfig key", func() {
			Expect(moVM.Config.ExtraConfig).ToNot(BeNil())
			ecMap := pkgutil.OptionValues(moVM.Config.ExtraConfig).StringMap()
			Expect(ecMap).To(HaveKey(backupapi.VMResourceYAMLExtraConfigKey))
		})
	})
}
