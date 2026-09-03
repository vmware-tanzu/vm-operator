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
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/test/builder"

	vimtypes "github.com/vmware/govmomi/vim25/types"
)

func vmHardwareVersionTests() {
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
	})

	AfterEach(func() {
		vmTestAfterEach(ctx, vm)

		vmClass = nil
		vm = nil

		ctx = nil
		initObjects = nil
		vmProvider = nil
	})

	// The VM asks for a minimum hardware version so that the version under test
	// is the one VM Operator chose. DetermineHardwareVersion picks no version at
	// all unless the VM, its class, or its devices call for one; the platform
	// then decides, and what it decides depends on the deploy path — the content
	// library deploy applies the version the OVF declares, whereas a Fast Deploy
	// create leaves it to the host.
	const minHardwareVersion = 17

	BeforeEach(func() {
		vm.Spec.MinHardwareVersion = minHardwareVersion
	})

	JustBeforeEach(func() {
		Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
	})

	It("should return the expected version", func() {
		version, err := vmProvider.GetVirtualMachineHardwareVersion(ctx, vm)
		Expect(err).NotTo(HaveOccurred())
		Expect(version).To(Equal(vimtypes.HardwareVersion(minHardwareVersion)))
	})
}
