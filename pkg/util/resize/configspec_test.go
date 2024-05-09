// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package resize_test

import (
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

	DescribeTable("Simple fields",
		func(
			ci vimtypes.VirtualMachineConfigInfo,
			cs, expectedCS vimtypes.VirtualMachineConfigSpec) {

			actualCS, err := resize.CreateResizeConfigSpec(ci, cs)
			Expect(err).ToNot(HaveOccurred())
			Expect(reflect.DeepEqual(actualCS, expectedCS)).To(BeTrue(), cmp.Diff(actualCS, expectedCS))
		},

		Entry("NumCPUs needs updating",
			ConfigInfo{Hardware: vimtypes.VirtualHardware{NumCPU: 2}},
			ConfigSpec{NumCPUs: 4},
			ConfigSpec{NumCPUs: 4}),
		Entry("NumCpus does not need updating",
			ConfigInfo{Hardware: vimtypes.VirtualHardware{NumCPU: 4}},
			ConfigSpec{NumCPUs: 4},
			ConfigSpec{}),

		Entry("MemoryMB needs updating",
			ConfigInfo{Hardware: vimtypes.VirtualHardware{MemoryMB: 512}},
			ConfigSpec{NumCPUs: 1024},
			ConfigSpec{NumCPUs: 1024}),
		Entry("MemoryMB does not need updating",
			ConfigInfo{Hardware: vimtypes.VirtualHardware{MemoryMB: 1024}},
			ConfigSpec{MemoryMB: 1024},
			ConfigSpec{}),
	)
})
