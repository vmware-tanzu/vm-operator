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

	Context("ExtraConfig", func() {
		var (
			vm vmopv1.VirtualMachine
			ci ConfigInfo
			cs ConfigSpec
		)

		BeforeEach(func() {
			vm = *builder.DummyVirtualMachine()

			ci = ConfigInfo{}
			cs = ConfigSpec{}
		})

		JustBeforeEach(func() {
			err := vmopv1util.OverwriteResizeConfigSpec(ctx, vm, ci, &cs)
			Expect(err).ToNot(HaveOccurred())
		})

		Context("MMIO ExtraConfig", func() {
			const mmIOValue = "42"

			var (
				mmIOOptVal     = &vimtypes.OptionValue{Key: constants.PCIPassthruMMIOExtraConfigKey, Value: constants.ExtraConfigTrue}
				mmIOSizeOptVal = &vimtypes.OptionValue{Key: constants.PCIPassthruMMIOSizeExtraConfigKey, Value: mmIOValue}
			)

			BeforeEach(func() {
				vm.Annotations[constants.PCIPassthruMMIOOverrideAnnotation] = mmIOValue

				ci.Hardware.Device = append(ci.Hardware.Device, &vimtypes.VirtualPCIPassthrough{
					VirtualDevice: vimtypes.VirtualDevice{
						Backing: &vimtypes.VirtualPCIPassthroughVmiopBackingInfo{
							Vgpu: "my-vgpu",
						},
					},
				})
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

		Context("MMPowerOff ExtraConfig", func() {

			var (
				mmPowerOffOptVal = &vimtypes.OptionValue{Key: constants.MMPowerOffVMExtraConfigKey, Value: ""}
			)

			Context("VM does not have MMPowerOff EC", func() {
				It("no updates", func() {
					Expect(cs.ExtraConfig).To(BeEmpty())
				})
			})

			Context("VM does have MMPowerOff EC but to be delete value", func() {
				BeforeEach(func() {
					ci.ExtraConfig = append(ci.ExtraConfig, mmPowerOffOptVal)
				})

				It("no updates", func() {
					Expect(cs.ExtraConfig).To(BeEmpty())
				})
			})

			Context("VM has MMPowerOff EC", func() {
				BeforeEach(func() {
					ov := *mmPowerOffOptVal
					ov.Value = constants.ExtraConfigTrue
					ci.ExtraConfig = append(ci.ExtraConfig, &ov)
				})

				It("removes it", func() {
					Expect(cs.ExtraConfig).To(ConsistOf(mmPowerOffOptVal))
				})
			})
		})

		Context("V1Alpha1Compatible EC", func() {

			var (
				v1a1CompatReadyOptVal   = &vimtypes.OptionValue{Key: constants.VMOperatorV1Alpha1ExtraConfigKey, Value: constants.VMOperatorV1Alpha1ConfigReady}
				v1a1CompatEnabledOptVal = &vimtypes.OptionValue{Key: constants.VMOperatorV1Alpha1ExtraConfigKey, Value: constants.VMOperatorV1Alpha1ConfigEnabled}
			)

			BeforeEach(func() {
				vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
					LinuxPrep:  &vmopv1.VirtualMachineBootstrapLinuxPrepSpec{},
					VAppConfig: &vmopv1.VirtualMachineBootstrapVAppConfigSpec{},
				}
			})

			Context("v1a1 compat is not present", func() {
				It("no updates", func() {
					Expect(cs.ExtraConfig).To(BeEmpty())
				})
			})

			Context("v1a1 compat is ready", func() {
				BeforeEach(func() {
					ci.ExtraConfig = append(ci.ExtraConfig, v1a1CompatReadyOptVal)
				})

				It("updates to enabled", func() {
					Expect(cs.ExtraConfig).To(ConsistOf(v1a1CompatEnabledOptVal))
				})
			})

			Context("v1a1 compat is already enabled", func() {
				BeforeEach(func() {
					ci.ExtraConfig = append(ci.ExtraConfig, v1a1CompatEnabledOptVal)
				})

				It("no updates", func() {
					Expect(cs.ExtraConfig).To(BeEmpty())
				})
			})
		})
	})
})
