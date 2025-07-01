// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/object"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func guestInfoTests() {

	var (
		ctx  *builder.TestContextForVCSim
		vcVM *object.VirtualMachine
	)

	BeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(builder.VCSimTestConfig{})

		var err error
		vcVM, err = ctx.Finder.VirtualMachine(ctx, "DC0_C0_RP0_VM0")
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	It("returns expected GuestInfo", func() {
		moID := vcVM.Reference().Value
		Expect(ctx.GetVMFromMoID(moID)).ToNot(BeNil())

		config := vimtypes.VirtualMachineConfigSpec{}
		config.ExtraConfig = append(config.ExtraConfig,
			&vimtypes.OptionValue{Key: "some-key", Value: "ignore-me"},
			&vimtypes.OptionValue{Key: "guestinfo.foo", Value: "hello"},
		)
		task, err := vcVM.Reconfigure(ctx, config)
		Expect(err).ToNot(HaveOccurred())
		Expect(task.Wait(ctx)).To(Succeed())

		guestInfo, err := virtualmachine.GetExtraConfigGuestInfo(ctx, vcVM)
		Expect(err).ToNot(HaveOccurred())

		Expect(guestInfo).To(HaveLen(1))
		Expect(guestInfo).To(HaveKeyWithValue("guestinfo.foo", "hello"))
	})
}
