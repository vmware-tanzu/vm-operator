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
})
