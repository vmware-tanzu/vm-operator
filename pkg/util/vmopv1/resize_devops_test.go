// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmopv1_test

import (
	"context"
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/google/go-cmp/cmp"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

type ConfigSpec = vimtypes.VirtualMachineConfigSpec
type ConfigInfo = vimtypes.VirtualMachineConfigInfo

var _ = Describe("OverwriteResizeConfigSpec", func() {

	ctx := context.Background()
	truePtr, falsePtr := vimtypes.NewBool(true), vimtypes.NewBool(false)

	vmAdvSpec := func(advSpec vmopv1.VirtualMachineAdvancedSpec) vmopv1.VirtualMachine {
		vm := builder.DummyVirtualMachine()
		vm.Spec.Advanced = &advSpec
		return *vm
	}

	DescribeTable("Resize Overrides",
		func(vm vmopv1.VirtualMachine,
			ci ConfigInfo,
			cs, expectedCS ConfigSpec) {

			err := vmopv1util.OverwriteResizeConfigSpec(ctx, vm, ci, &cs)
			Expect(err).ToNot(HaveOccurred())
			Expect(reflect.DeepEqual(cs, expectedCS)).To(BeTrue(), cmp.Diff(cs, expectedCS))
		},

		Entry("Empty AdvancedSpec",
			vmAdvSpec(vmopv1.VirtualMachineAdvancedSpec{}),
			ConfigInfo{},
			ConfigSpec{},
			ConfigSpec{}),

		Entry("CBT not set in VM Spec but in ConfigSpec",
			vmAdvSpec(vmopv1.VirtualMachineAdvancedSpec{}),
			ConfigInfo{},
			ConfigSpec{ChangeTrackingEnabled: truePtr},
			ConfigSpec{ChangeTrackingEnabled: truePtr}),
		Entry("CBT set in VM Spec takes precedence over ConfigSpec",
			vmAdvSpec(vmopv1.VirtualMachineAdvancedSpec{ChangeBlockTracking: falsePtr}),
			ConfigInfo{},
			ConfigSpec{ChangeTrackingEnabled: truePtr},
			ConfigSpec{ChangeTrackingEnabled: falsePtr}),
		Entry("CBT set in VM Spec but not in ConfigSpec",
			vmAdvSpec(vmopv1.VirtualMachineAdvancedSpec{ChangeBlockTracking: truePtr}),
			ConfigInfo{},
			ConfigSpec{},
			ConfigSpec{ChangeTrackingEnabled: truePtr}),
		Entry("CBT set in VM Spec with same value in ConfigInfo",
			vmAdvSpec(vmopv1.VirtualMachineAdvancedSpec{ChangeBlockTracking: truePtr}),
			ConfigInfo{ChangeTrackingEnabled: truePtr},
			ConfigSpec{},
			ConfigSpec{}),
		Entry("CBT set in ConfigSpec with same value in ConfigInfo",
			vmAdvSpec(vmopv1.VirtualMachineAdvancedSpec{}),
			ConfigInfo{ChangeTrackingEnabled: truePtr},
			ConfigSpec{ChangeTrackingEnabled: truePtr},
			ConfigSpec{}),
		Entry("CBT set in VM Spec with same value in ConfigInfo but different value in ConfigSpec",
			vmAdvSpec(vmopv1.VirtualMachineAdvancedSpec{ChangeBlockTracking: truePtr}),
			ConfigInfo{ChangeTrackingEnabled: truePtr},
			ConfigSpec{ChangeTrackingEnabled: falsePtr},
			ConfigSpec{}),
	)

	Context("MMIO ExtraConfig", func() {
		const mmIOValue = "42"

		var (
			vm vmopv1.VirtualMachine
			ci ConfigInfo
			cs ConfigSpec

			mmIOOptVal     = &vimtypes.OptionValue{Key: constants.PCIPassthruMMIOExtraConfigKey, Value: constants.ExtraConfigTrue}
			mmIOSizeOptVal = &vimtypes.OptionValue{Key: constants.PCIPassthruMMIOSizeExtraConfigKey, Value: mmIOValue}
		)

		BeforeEach(func() {
			vm = *builder.DummyVirtualMachine()
			vm.Annotations[constants.PCIPassthruMMIOOverrideAnnotation] = mmIOValue

			ci = ConfigInfo{}
			cs = ConfigSpec{}

			ci.Hardware.Device = append(ci.Hardware.Device, &vimtypes.VirtualPCIPassthrough{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualPCIPassthroughVmiopBackingInfo{
						Vgpu: "my-vgpu",
					},
				},
			})
		})

		JustBeforeEach(func() {
			err := vmopv1util.OverwriteResizeConfigSpec(ctx, vm, ci, &cs)
			Expect(err).ToNot(HaveOccurred())
		})

		Context("VM already has expected MMIO EC values", func() {
			BeforeEach(func() {
				ci.ExtraConfig = append(ci.ExtraConfig, mmIOOptVal, mmIOSizeOptVal)
			})

			It("no updates", func() {
				Expect(cs.ExtraConfig).To(BeEmpty())
			})
		})

		Context("VM ConfigSpec already has expected MMIO EC values", func() {
			BeforeEach(func() {
				cs.ExtraConfig = append(ci.ExtraConfig, mmIOOptVal, mmIOSizeOptVal)
			})

			It("same changes", func() {
				Expect(cs.ExtraConfig).To(ConsistOf(mmIOOptVal, mmIOSizeOptVal))
			})
		})

		Context("VM has none of expected MMIO EC values", func() {
			It("adds updates", func() {
				Expect(cs.ExtraConfig).To(ConsistOf(mmIOOptVal, mmIOSizeOptVal))
			})
		})

		Context("VM has different MMIO EC value than expected", func() {
			BeforeEach(func() {
				ov := *mmIOOptVal
				ov.Value = constants.ExtraConfigFalse
				ci.ExtraConfig = append(ci.ExtraConfig, &ov, mmIOSizeOptVal)
			})

			It("updates it", func() {
				Expect(cs.ExtraConfig).To(ConsistOf(mmIOOptVal))
			})
		})

		Context("VM and ConfigSpec already have expected MMIO EC values", func() {
			BeforeEach(func() {
				ci.ExtraConfig = append(ci.ExtraConfig, mmIOOptVal, mmIOSizeOptVal)
				cs.ExtraConfig = append(cs.ExtraConfig, mmIOOptVal, mmIOSizeOptVal)
			})

			It("removes updates", func() {
				Expect(cs.ExtraConfig).To(BeEmpty())
			})
		})
	})
})
