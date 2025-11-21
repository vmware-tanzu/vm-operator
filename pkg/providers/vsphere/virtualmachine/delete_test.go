// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/object"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func deleteTests() {

	var (
		ctx   *builder.TestContextForVCSim
		vcVM  *object.VirtualMachine
		vmCtx pkgctx.VirtualMachineContext
	)

	BeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(builder.VCSimTestConfig{})

		var err error
		vcVM, err = ctx.Finder.VirtualMachine(ctx, "DC0_C0_RP0_VM0")
		Expect(err).ToNot(HaveOccurred())

		vmCtx = pkgctx.VirtualMachineContext{
			Context: ctx,
			Logger:  suite.GetLogger().WithValues("vmName", vcVM.Name()),
			VM:      builder.DummyVirtualMachine(),
		}
	})

	JustBeforeEach(func() {
		// The provider DeleteVirtualMachine() will grab these properties.
		Expect(vcVM.Properties(vmCtx, vcVM.Reference(),
			virtualmachine.VMDeletePropertiesSelector, &vmCtx.MoVM)).To(Succeed())
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	It("Deletes VM that is off", func() {
		moID := vcVM.Reference().Value
		Expect(ctx.GetVMFromMoID(moID)).ToNot(BeNil())

		t, err := vcVM.PowerOff(ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(t.Wait(ctx)).To(Succeed())

		err = virtualmachine.DeleteVirtualMachine(vmCtx, vcVM)
		Expect(err).ToNot(HaveOccurred())

		Expect(ctx.GetVMFromMoID(moID)).To(BeNil())
	})

	It("Deletes VM that is on", func() {
		moID := vcVM.Reference().Value
		Expect(ctx.GetVMFromMoID(moID)).ToNot(BeNil())

		state, err := vcVM.PowerState(ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(state).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOn))

		err = virtualmachine.DeleteVirtualMachine(vmCtx, vcVM)
		Expect(err).ToNot(HaveOccurred())

		Expect(ctx.GetVMFromMoID(moID)).To(BeNil())
	})
}
