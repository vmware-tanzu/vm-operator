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

var _ = Describe("OverrideResizeConfigSpec", func() {

	ctx := context.Background()
	truePtr, falsePtr := vimtypes.NewBool(true), vimtypes.NewBool(false)

	vmAdvSpec := func(advSpec vmopv1.VirtualMachineAdvancedSpec) vmopv1.VirtualMachine {
		vm := builder.DummyVirtualMachine()
		vm.Spec.Advanced = &advSpec
		return *vm
	}

	DescribeTable("Resize Overrides",
		func(vm vmopv1.VirtualMachine,
			cs, expectedCS vimtypes.VirtualMachineConfigSpec) {

			err := vmopv1util.OverrideResizeConfigSpec(ctx, vm, &cs)
			Expect(err).ToNot(HaveOccurred())
			Expect(reflect.DeepEqual(cs, expectedCS)).To(BeTrue(), cmp.Diff(cs, expectedCS))
		},

		Entry("Empty AdvancedSpec",
			vmAdvSpec(vmopv1.VirtualMachineAdvancedSpec{}),
			ConfigSpec{},
			ConfigSpec{}),

		Entry("CBT not set in VM Spec but in ConfigSpec",
			vmAdvSpec(vmopv1.VirtualMachineAdvancedSpec{}),
			ConfigSpec{ChangeTrackingEnabled: truePtr},
			ConfigSpec{ChangeTrackingEnabled: truePtr}),
		Entry("CBT set in VM Spec takes precedence over ConfigSpec",
			vmAdvSpec(vmopv1.VirtualMachineAdvancedSpec{ChangeBlockTracking: falsePtr}),
			ConfigSpec{ChangeTrackingEnabled: truePtr},
			ConfigSpec{ChangeTrackingEnabled: falsePtr}),
		Entry("CBT set in VM Spec but not in ConfigSpec",
			vmAdvSpec(vmopv1.VirtualMachineAdvancedSpec{ChangeBlockTracking: truePtr}),
			ConfigSpec{},
			ConfigSpec{ChangeTrackingEnabled: truePtr}),
	)
})
