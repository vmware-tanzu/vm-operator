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
)

func vmNPETests() {
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
		testConfig.WithNetworkEnv = builder.NetworkEnvVDS

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

	DescribeTable("npe checks",
		func(fn func(vm *vmopv1.VirtualMachine)) {
			fn(vm)
			Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())

			Expect(vm.Status.UniqueID).ToNot(BeEmpty())
			vcVM := ctx.GetVMFromMoID(vm.Status.UniqueID)
			Expect(vcVM).ToNot(BeNil())
		},
		Entry(
			"nil spec.advanced",
			func(vm *vmopv1.VirtualMachine) {
				vm.Spec.Advanced = nil
			},
		),
		Entry(
			"nil spec.bootstrap",
			func(vm *vmopv1.VirtualMachine) {
				vm.Spec.Bootstrap = nil
			},
		),
		Entry(
			"nil spec.network",
			func(vm *vmopv1.VirtualMachine) {
				vm.Spec.Network = nil
			},
		),
		Entry(
			"nil spec.reserved",
			func(vm *vmopv1.VirtualMachine) {
				vm.Spec.Reserved = nil
			},
		),
	)
}
