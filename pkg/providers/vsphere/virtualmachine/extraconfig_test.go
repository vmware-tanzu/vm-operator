// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	"github.com/vmware/govmomi/object"

	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func extraConfigTests() {

	var (
		ctx      *builder.TestContextForVCSim
		vcVM     *object.VirtualMachine
		vmConfig *vimtypes.VirtualMachineConfigSpec
	)

	BeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(builder.VCSimTestConfig{})
		vmConfig = &vimtypes.VirtualMachineConfigSpec{}

		var err error
		vcVM, err = ctx.Finder.VirtualMachine(ctx, "DC0_C0_RP0_VM0")
		Expect(err).ToNot(HaveOccurred())
	})

	JustBeforeEach(func() {
		task, err := vcVM.Reconfigure(ctx, *vmConfig)
		Expect(err).ToNot(HaveOccurred())
		Expect(task.Wait(ctx)).To(Succeed())
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	unchangedTestFn := func(reset bool) {
		before, err := virtualmachine.GetExtraConfigFromObject(ctx, vcVM)
		Expect(err).ToNot(HaveOccurred())
		after, err := virtualmachine.GetFilteredExtraConfigFromObject(ctx, vcVM, reset)
		Expect(err).ToNot(HaveOccurred())
		Expect(after.Diff(before...)).To(BeNil())
	}

	When("extraConfig has a key that is not in the denyList", func() {
		BeforeEach(func() {
			vmConfig.ExtraConfig = append(vmConfig.ExtraConfig,
				&vimtypes.OptionValue{Key: "k1", Value: "v1"},
				&vimtypes.OptionValue{Key: "k2", Value: "v2"},
			)
		})

		It("should be unchanged", func() {
			unchangedTestFn(false)
			unchangedTestFn(true)
		})
	})

	When("extraConfig has a key that is in the allowKeys", func() {
		BeforeEach(func() {
			vmConfig.ExtraConfig = append(vmConfig.ExtraConfig,
				&vimtypes.OptionValue{Key: "vmservice.virtualmachine.pvc.disk.data", Value: "v1"},
				&vimtypes.OptionValue{Key: "vmservice.vmi.labels", Value: "v2"},
			)
		})

		It("should be unchanged", func() {
			unchangedTestFn(false)
			unchangedTestFn(true)
		})
	})

	When("extraConfig has a key that is the same as the key prefix", func() {
		BeforeEach(func() {
			vmConfig.ExtraConfig = append(vmConfig.ExtraConfig,
				&vimtypes.OptionValue{Key: "guestinfo", Value: "v1"},
				&vimtypes.OptionValue{Key: "vmservice", Value: "v2"},
			)
		})

		It("should be unchanged", func() {
			unchangedTestFn(false)
			unchangedTestFn(true)
		})
	})

	When("extraConfig has a key that is in the deny key prefixes", func() {
		var (
			toBeRemoved = pkgutil.OptionValues{
				&vimtypes.OptionValue{Key: strings.ToUpper("guestinfo.v1"), Value: "v1"},
				&vimtypes.OptionValue{Key: "vmservice.v2", Value: "v2"},
			}
		)
		BeforeEach(func() {
			vmConfig.ExtraConfig = append(vmConfig.ExtraConfig, toBeRemoved...)
		})

		testFn := func(reset bool) {
			before, err := virtualmachine.GetExtraConfigFromObject(ctx, vcVM)
			Expect(err).ToNot(HaveOccurred())
			after, err := virtualmachine.GetFilteredExtraConfigFromObject(ctx, vcVM, reset)
			Expect(err).ToNot(HaveOccurred())
			if reset {
				for k := range toBeRemoved.StringMap() {
					newV, ok := after.GetString(k)
					Expect(ok).To(BeTrue())
					Expect(newV).To(BeEmpty())
				}
			} else {
				Expect(len(after.Diff(before...))).To(Equal(len(toBeRemoved)))
			}
		}

		It("should be removed", func() {
			testFn(false)
			testFn(true)
		})
	})

	When("extraConfig has a key with non string value", func() {
		BeforeEach(func() {
			vmConfig.ExtraConfig = append(vmConfig.ExtraConfig,
				&vimtypes.OptionValue{Key: "guestinfo.v1", Value: 1},
			)
		})

		testFn := func(reset bool) {
			out, err := virtualmachine.GetFilteredExtraConfigFromObject(ctx, vcVM, reset)
			if reset {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to filter extraConfig"))
				Expect(err.Error()).To(ContainSubstring("expected value's datatype string"))
				Expect(out).To(BeNil())
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
		}

		It("should return error if trying to unset it to empty string", func() {
			testFn(false)
			testFn(true)
		})
	})
}
