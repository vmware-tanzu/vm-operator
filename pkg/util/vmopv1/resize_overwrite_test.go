// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1_test

import (
	"context"
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/google/go-cmp/cmp"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("OverwriteResizeConfigSpec", func() {

	type ConfigSpec = vimtypes.VirtualMachineConfigSpec
	type ConfigInfo = vimtypes.VirtualMachineConfigInfo

	configInfoManagedBy := func(ci ConfigInfo, args ...string) ConfigInfo {
		var (
			exKey  = vmopv1.ManagedByExtensionKey
			exType = vmopv1.ManagedByExtensionType
		)

		if len(args) > 0 {
			exKey = args[0]
		}
		if len(args) > 1 {
			exType = args[1]
		}

		ci.ManagedBy = &vimtypes.ManagedByInfo{
			ExtensionKey: exKey,
			Type:         exType,
		}

		return ci
	}

	//nolint:unparam
	configSpecManagedBy := func(cs ConfigSpec, args ...string) ConfigSpec {
		var (
			exKey  = vmopv1.ManagedByExtensionKey
			exType = vmopv1.ManagedByExtensionType
		)

		if len(args) > 0 {
			exKey = args[0]
		}
		if len(args) > 1 {
			exType = args[1]
		}

		cs.ManagedBy = &vimtypes.ManagedByInfo{
			ExtensionKey: exKey,
			Type:         exType,
		}

		return cs
	}

	configInfoNamespaceName := func(ci ConfigInfo) ConfigInfo {
		ci.ExtraConfig = pkgutil.OptionValues(ci.ExtraConfig).Merge(
			&vimtypes.OptionValue{
				Key:   constants.ExtraConfigVMServiceNamespacedName,
				Value: "",
			},
		)
		return ci
	}

	configInfoWithNamespaceName := func() ConfigInfo {
		return configInfoNamespaceName(ConfigInfo{})
	}
	configInfoWithManagedByAndNamespaceName := func() ConfigInfo {
		return configInfoManagedBy(configInfoWithNamespaceName())
	}

	ctx := context.Background()
	truePtr, falsePtr := vimtypes.NewBool(true), vimtypes.NewBool(false)

	vmAdvSpec := func(advSpec vmopv1.VirtualMachineAdvancedSpec) vmopv1.VirtualMachine {
		vm := builder.DummyVirtualMachine()
		vm.Spec.Advanced = &advSpec
		// Empty guestID to avoid it being set in the returned ConfigSpec.
		vm.Spec.GuestID = ""
		return *vm
	}

	vmGuestID := func(guestID string) vmopv1.VirtualMachine {
		vm := builder.DummyVirtualMachine()
		vm.Spec.GuestID = guestID
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

		Entry("Empty VM",
			vmopv1.VirtualMachine{},
			configInfoWithManagedByAndNamespaceName(),
			ConfigSpec{},
			ConfigSpec{}),
		Entry("CBT not set in VM Spec but in ConfigSpec",
			vmAdvSpec(vmopv1.VirtualMachineAdvancedSpec{}),
			configInfoWithManagedByAndNamespaceName(),
			ConfigSpec{ChangeTrackingEnabled: truePtr},
			ConfigSpec{ChangeTrackingEnabled: truePtr}),
		Entry("CBT set in VM Spec takes precedence over ConfigSpec",
			vmAdvSpec(vmopv1.VirtualMachineAdvancedSpec{ChangeBlockTracking: falsePtr}),
			configInfoWithManagedByAndNamespaceName(),
			ConfigSpec{ChangeTrackingEnabled: truePtr},
			ConfigSpec{ChangeTrackingEnabled: falsePtr}),
		Entry("CBT set in VM Spec but not in ConfigSpec",
			vmAdvSpec(vmopv1.VirtualMachineAdvancedSpec{ChangeBlockTracking: truePtr}),
			configInfoWithManagedByAndNamespaceName(),
			ConfigSpec{},
			ConfigSpec{ChangeTrackingEnabled: truePtr}),
		Entry("CBT set in VM Spec with same value in ConfigInfo",
			vmAdvSpec(vmopv1.VirtualMachineAdvancedSpec{ChangeBlockTracking: truePtr}),
			configInfoManagedBy(configInfoNamespaceName(ConfigInfo{ChangeTrackingEnabled: truePtr})),
			ConfigSpec{},
			ConfigSpec{}),
		Entry("CBT set in ConfigSpec with same value in ConfigInfo",
			vmAdvSpec(vmopv1.VirtualMachineAdvancedSpec{}),
			configInfoManagedBy(configInfoNamespaceName(ConfigInfo{ChangeTrackingEnabled: truePtr})),
			ConfigSpec{ChangeTrackingEnabled: truePtr},
			ConfigSpec{}),
		Entry("CBT set in VM Spec with same value in ConfigInfo but different value in ConfigSpec",
			vmAdvSpec(vmopv1.VirtualMachineAdvancedSpec{ChangeBlockTracking: truePtr}),
			configInfoManagedBy(configInfoNamespaceName(ConfigInfo{ChangeTrackingEnabled: truePtr})),
			ConfigSpec{ChangeTrackingEnabled: falsePtr},
			ConfigSpec{}),

		Entry("Guest not set in VM Spec but in ConfigSpec is ignored",
			vmGuestID(""),
			configInfoWithManagedByAndNamespaceName(),
			ConfigSpec{GuestId: "foo"},
			ConfigSpec{}),
		Entry("GuestID set in VM Spec takes precedence over ConfigSpec",
			vmGuestID("foo"),
			configInfoWithManagedByAndNamespaceName(),
			ConfigSpec{GuestId: "bar"},
			ConfigSpec{GuestId: "foo"}),
		Entry("GuestID set in VM Spec but not in ConfigSpec",
			vmGuestID("foo"),
			configInfoWithManagedByAndNamespaceName(),
			ConfigSpec{},
			ConfigSpec{GuestId: "foo"}),
		Entry("GuestID set in VM Spec with same value in ConfigInfo",
			vmGuestID("foo"),
			configInfoManagedBy(configInfoNamespaceName(ConfigInfo{GuestId: "foo"})),
			ConfigSpec{},
			ConfigSpec{}),
		Entry("GuestID set in ConfigSpec with same value in ConfigInfo",
			vmGuestID(""),
			configInfoManagedBy(configInfoNamespaceName(ConfigInfo{GuestId: "foo"})),
			ConfigSpec{GuestId: "foo"},
			ConfigSpec{}),
		Entry("GuestID set in VM Spec with same value in ConfigInfo but different value in ConfigSpec",
			vmGuestID("foo"),
			configInfoManagedBy(configInfoNamespaceName(ConfigInfo{GuestId: "foo"})),
			ConfigSpec{GuestId: "bar"},
			ConfigSpec{}),

		Entry("ManagedBy not set in ConfigInfo or ConfigSpec",
			vmopv1.VirtualMachine{},
			configInfoWithNamespaceName(),
			ConfigSpec{},
			configSpecManagedBy(ConfigSpec{})),
		Entry("ManagedBy set in ConfigInfo and not in ConfigSpec",
			vmopv1.VirtualMachine{},
			configInfoWithManagedByAndNamespaceName(),
			ConfigSpec{},
			ConfigSpec{}),
		Entry("ManagedBy set in ConfigInfo with same value in ConfigSpec",
			vmopv1.VirtualMachine{},
			configInfoWithManagedByAndNamespaceName(),
			configSpecManagedBy(ConfigSpec{}),
			ConfigSpec{}),
		Entry("ManagedBy set in ConfigInfo with wrong value in ConfigSpec",
			vmopv1.VirtualMachine{},
			configInfoWithManagedByAndNamespaceName(),
			configSpecManagedBy(ConfigSpec{}, "fake", "fake"),
			ConfigSpec{}),
		Entry("ManagedBy not set in ConfigInfo with wrong value in ConfigSpec",
			vmopv1.VirtualMachine{},
			configInfoWithNamespaceName(),
			configSpecManagedBy(ConfigSpec{}, "fake", "fake"),
			configSpecManagedBy(ConfigSpec{})),
	)

	Context("ExtraConfig", func() {
		var (
			vm                vmopv1.VirtualMachine
			ci                ConfigInfo
			cs                ConfigSpec
			namespacedNameVal *vimtypes.OptionValue
		)

		BeforeEach(func() {
			vm = *builder.DummyVirtualMachine()
			namespacedNameVal = &vimtypes.OptionValue{
				Key:   constants.ExtraConfigVMServiceNamespacedName,
				Value: "",
			}
			ci = configInfoWithNamespaceName()
			cs = ConfigSpec{}
		})

		JustBeforeEach(func() {
			err := vmopv1util.OverwriteResizeConfigSpec(ctx, vm, ci, &cs)
			Expect(err).ToNot(HaveOccurred())
		})

		Context("Namespace and name", func() {
			BeforeEach(func() {
				ci.ExtraConfig = nil
			})

			When("VM already has expected EC values", func() {
				BeforeEach(func() {
					ci.ExtraConfig = append(ci.ExtraConfig, namespacedNameVal)
				})
				It("no updates", func() {
					Expect(cs.ExtraConfig).To(BeEmpty())
				})
			})
			When("VM ConfigSpec already has expected EC values", func() {
				BeforeEach(func() {
					cs.ExtraConfig = append(cs.ExtraConfig, namespacedNameVal)
				})
				It("same changes", func() {
					Expect(cs.ExtraConfig).To(ConsistOf(namespacedNameVal))
				})
			})
			When("VM has none of expected EC values", func() {
				It("adds it", func() {
					Expect(cs.ExtraConfig).To(ConsistOf(namespacedNameVal))
				})
			})
			When("VM has different than expected EC values", func() {
				BeforeEach(func() {
					ov := *namespacedNameVal
					ov.Value = "fake/fake"
					ci.ExtraConfig = append(ci.ExtraConfig, &ov)
				})
				It("updates it", func() {
					Expect(cs.ExtraConfig).To(ConsistOf(namespacedNameVal))
				})
			})
			Context("VM and ConfigSpec already have expected values", func() {
				BeforeEach(func() {
					ci.ExtraConfig = append(ci.ExtraConfig, namespacedNameVal)
					cs.ExtraConfig = append(cs.ExtraConfig, namespacedNameVal)
				})

				It("removes updates", func() {
					Expect(cs.ExtraConfig).To(BeEmpty())
				})
			})
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
