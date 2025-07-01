// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
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

	Specify("VM is paused by admin", func() {
		var moVM mo.VirtualMachine
		Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &moVM)).To(Succeed())

		sctx := ctx.SimulatorContext()
		sctx.WithLock(
			vcVM.Reference(),
			func() {
				vm := sctx.Map.Get(vcVM.Reference()).(*simulator.VirtualMachine)
				vm.Config = &vimtypes.VirtualMachineConfigInfo{
					ExtraConfig: []vimtypes.BaseOptionValue{
						&vimtypes.OptionValue{
							Key:   vmopv1.PauseVMExtraConfigKey,
							Value: "True",
						},
					},
				}
			})

		err := virtualmachine.DeleteVirtualMachine(vmCtx, vcVM)

		Expect(err).To(HaveOccurred())
		var noRequeueErr pkgerr.NoRequeueError
		Expect(errors.As(err, &noRequeueErr)).To(BeTrue())
		Expect(noRequeueErr.Message).To(Equal(constants.VMPausedByAdminError))
		Expect(ctx.GetVMFromMoID(moVM.Reference().Value)).ToNot(BeNil())
	})

	DescribeTable("VM is not connected",
		func(state vimtypes.VirtualMachineConnectionState) {
			var moVM mo.VirtualMachine
			Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &moVM)).To(Succeed())

			sctx := ctx.SimulatorContext()
			sctx.WithLock(
				vcVM.Reference(),
				func() {
					vm := sctx.Map.Get(vcVM.Reference()).(*simulator.VirtualMachine)
					vm.Summary.Runtime.ConnectionState = state
				})

			err := virtualmachine.DeleteVirtualMachine(vmCtx, vcVM)

			if state == "" {
				Expect(err).ToNot(HaveOccurred())
				Expect(ctx.GetVMFromMoID(moVM.Reference().Value)).To(BeNil())
			} else {
				Expect(err).To(HaveOccurred())
				var noRequeueErr pkgerr.NoRequeueError
				Expect(errors.As(err, &noRequeueErr)).To(BeTrue())
				Expect(noRequeueErr.Message).To(Equal(
					fmt.Sprintf("unsupported connection state: %s", state)))
				Expect(ctx.GetVMFromMoID(moVM.Reference().Value)).ToNot(BeNil())
			}
		},
		Entry("empty", vimtypes.VirtualMachineConnectionState("")),
		Entry("disconnected", vimtypes.VirtualMachineConnectionStateDisconnected),
		Entry("inaccessible", vimtypes.VirtualMachineConnectionStateInaccessible),
		Entry("invalid", vimtypes.VirtualMachineConnectionStateInvalid),
		Entry("orphaned", vimtypes.VirtualMachineConnectionStateOrphaned),
	)
}
