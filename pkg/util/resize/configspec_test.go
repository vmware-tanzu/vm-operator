// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package resize_test

import (
	"context"
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/google/go-cmp/cmp"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/util/resize"
)

type ConfigSpec = vimtypes.VirtualMachineConfigSpec
type ConfigInfo = vimtypes.VirtualMachineConfigInfo

var _ = Describe("CreateResizeConfigSpec", func() {

	ctx := context.Background()
	truePtr, falsePtr := vimtypes.NewBool(true), vimtypes.NewBool(false)

	DescribeTable("ConfigInfo.Hardware",
		func(
			ci vimtypes.VirtualMachineConfigInfo,
			cs, expectedCS vimtypes.VirtualMachineConfigSpec) {

			actualCS, err := resize.CreateResizeConfigSpec(ctx, ci, cs)
			Expect(err).ToNot(HaveOccurred())
			Expect(reflect.DeepEqual(actualCS, expectedCS)).To(BeTrue(), cmp.Diff(actualCS, expectedCS))
		},

		Entry("Empty Hardware needs no updating",
			ConfigInfo{Hardware: vimtypes.VirtualHardware{}},
			ConfigSpec{},
			ConfigSpec{}),

		Entry("NumCPUs needs updating",
			ConfigInfo{Hardware: vimtypes.VirtualHardware{NumCPU: 2}},
			ConfigSpec{NumCPUs: 4},
			ConfigSpec{NumCPUs: 4}),
		Entry("NumCpus does not need updating",
			ConfigInfo{Hardware: vimtypes.VirtualHardware{NumCPU: 4}},
			ConfigSpec{NumCPUs: 4},
			ConfigSpec{}),

		Entry("NumCoresPerSocket needs updating",
			ConfigInfo{Hardware: vimtypes.VirtualHardware{NumCoresPerSocket: 2}},
			ConfigSpec{NumCoresPerSocket: 4},
			ConfigSpec{NumCoresPerSocket: 4}),
		Entry("NumCoresPerSocket does not need updating",
			ConfigInfo{Hardware: vimtypes.VirtualHardware{NumCoresPerSocket: 4}},
			ConfigSpec{NumCoresPerSocket: 4},
			ConfigSpec{}),

		Entry("MemoryMB needs updating",
			ConfigInfo{Hardware: vimtypes.VirtualHardware{MemoryMB: 512}},
			ConfigSpec{NumCPUs: 1024},
			ConfigSpec{NumCPUs: 1024}),
		Entry("MemoryMB does not need updating",
			ConfigInfo{Hardware: vimtypes.VirtualHardware{MemoryMB: 1024}},
			ConfigSpec{MemoryMB: 1024},
			ConfigSpec{}),

		Entry("VirtualICH7MPresent needs updating",
			ConfigInfo{Hardware: vimtypes.VirtualHardware{VirtualICH7MPresent: truePtr}},
			ConfigSpec{VirtualICH7MPresent: falsePtr},
			ConfigSpec{VirtualICH7MPresent: falsePtr}),
		Entry("VirtualICH7MPresent needs updating",
			ConfigInfo{Hardware: vimtypes.VirtualHardware{VirtualICH7MPresent: nil}},
			ConfigSpec{VirtualICH7MPresent: falsePtr},
			ConfigSpec{VirtualICH7MPresent: falsePtr}),
		Entry("VirtualICH7MPresent does not need updating",
			ConfigInfo{Hardware: vimtypes.VirtualHardware{VirtualICH7MPresent: truePtr}},
			ConfigSpec{VirtualICH7MPresent: truePtr},
			ConfigSpec{}),

		Entry("VirtualSMCPresent needs updating",
			ConfigInfo{Hardware: vimtypes.VirtualHardware{VirtualSMCPresent: truePtr}},
			ConfigSpec{VirtualSMCPresent: falsePtr},
			ConfigSpec{VirtualSMCPresent: falsePtr}),
		Entry("VirtualSMCPresent needs updating",
			ConfigInfo{Hardware: vimtypes.VirtualHardware{VirtualSMCPresent: nil}},
			ConfigSpec{VirtualSMCPresent: falsePtr},
			ConfigSpec{VirtualSMCPresent: falsePtr}),
		Entry("VirtualSMCPresent does not need updating",
			ConfigInfo{Hardware: vimtypes.VirtualHardware{VirtualSMCPresent: truePtr}},
			ConfigSpec{VirtualSMCPresent: truePtr},
			ConfigSpec{}),

		Entry("MotherboardLayout needs updating",
			ConfigInfo{Hardware: vimtypes.VirtualHardware{MotherboardLayout: "foo"}},
			ConfigSpec{MotherboardLayout: "i440bxHostBridge"},
			ConfigSpec{MotherboardLayout: "i440bxHostBridge"}),
		Entry("MotherboardLayout does not needs updating",
			ConfigInfo{Hardware: vimtypes.VirtualHardware{MotherboardLayout: "i440bxHostBridge"}},
			ConfigSpec{MotherboardLayout: "i440bxHostBridge"},
			ConfigSpec{}),

		Entry("SimultaneousThreads needs updating",
			ConfigInfo{Hardware: vimtypes.VirtualHardware{SimultaneousThreads: 8}},
			ConfigSpec{SimultaneousThreads: 16},
			ConfigSpec{SimultaneousThreads: 16}),
		Entry("SimultaneousThreads does not need updating",
			ConfigInfo{Hardware: vimtypes.VirtualHardware{SimultaneousThreads: 8}},
			ConfigSpec{SimultaneousThreads: 8},
			ConfigSpec{}),
	)
})
