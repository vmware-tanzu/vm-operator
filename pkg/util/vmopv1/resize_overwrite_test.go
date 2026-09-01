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

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("OverwriteAlwaysResizeConfigSpec", func() {

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

	DescribeTable("Always Resize Overrides",
		func(vm vmopv1.VirtualMachine,
			ci ConfigInfo,
			cs, expectedCS ConfigSpec) {

			err := vmopv1util.OverwriteAlwaysResizeConfigSpec(ctx, vm, ci, &cs)
			Expect(err).ToNot(HaveOccurred())
			Expect(reflect.DeepEqual(cs, expectedCS)).To(BeTrue(), cmp.Diff(cs, expectedCS))
		},

		Entry("Empty VM",
			vmopv1.VirtualMachine{},
			configInfoWithManagedByAndNamespaceName(),
			ConfigSpec{},
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
			err := vmopv1util.OverwriteAlwaysResizeConfigSpec(ctx, vm, ci, &cs)
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

	})
})
