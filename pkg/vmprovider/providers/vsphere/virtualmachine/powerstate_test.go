// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func powerStateTests() {

	var (
		ctx   *builder.TestContextForVCSim
		vcVM  *object.VirtualMachine
		vmCtx context.VirtualMachineContext
	)

	BeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(builder.VCSimTestConfig{})

		var err error
		vcVM, err = ctx.Finder.VirtualMachine(ctx, "DC0_C0_RP0_VM0")
		Expect(err).ToNot(HaveOccurred())

		vmCtx = context.VirtualMachineContext{
			Context: ctx,
			Logger:  suite.GetLogger().WithValues("vmName", vcVM.Name()),
			VM:      builder.DummyVirtualMachine(),
		}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	It("Turns VM off", func() {
		err := virtualmachine.ChangePowerState(vmCtx, vcVM, types.VirtualMachinePowerStatePoweredOff)
		Expect(err).ToNot(HaveOccurred())

		state, err := vcVM.PowerState(ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(state).To(Equal(types.VirtualMachinePowerStatePoweredOff))
	})

	It("Turns VM on", func() {
		t, err := vcVM.PowerOff(ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(t.Wait(ctx)).To(Succeed())

		err = virtualmachine.ChangePowerState(vmCtx, vcVM, types.VirtualMachinePowerStatePoweredOn)
		Expect(err).ToNot(HaveOccurred())

		state, err := vcVM.PowerState(ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(state).To(Equal(types.VirtualMachinePowerStatePoweredOn))
	})

	It("Returns success when VM is already in desired state", func() {
		state, err := vcVM.PowerState(ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(state).To(Equal(types.VirtualMachinePowerStatePoweredOn))

		err = virtualmachine.ChangePowerState(vmCtx, vcVM, types.VirtualMachinePowerStatePoweredOn)
		Expect(err).ToNot(HaveOccurred())
	})
}
