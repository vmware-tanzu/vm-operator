// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package resize_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/google/go-cmp/cmp"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/pkg/util/resize"
)

var _ = Describe("Backings", func() {

	truePtr, falsePtr := ptr.To(true), ptr.To(false)

	DescribeTable("MatchVirtualDeviceDeviceBackingInfo",
		func(expected, current vimtypes.VirtualDeviceDeviceBackingInfo, match bool) {
			m := resize.MatchVirtualDeviceDeviceBackingInfo(&expected, &current)
			Expect(m).To(Equal(match), cmp.Diff(current, expected))
		},

		Entry("#1",
			vimtypes.VirtualDeviceDeviceBackingInfo{},
			vimtypes.VirtualDeviceDeviceBackingInfo{},
			true,
		),
		Entry("#2",
			vimtypes.VirtualDeviceDeviceBackingInfo{DeviceName: "foo", UseAutoDetect: truePtr},
			vimtypes.VirtualDeviceDeviceBackingInfo{DeviceName: "foo", UseAutoDetect: truePtr},
			true,
		),
		Entry("#3",
			vimtypes.VirtualDeviceDeviceBackingInfo{DeviceName: "foo"},
			vimtypes.VirtualDeviceDeviceBackingInfo{DeviceName: "bar"},
			false,
		),
		Entry("#4",
			vimtypes.VirtualDeviceDeviceBackingInfo{UseAutoDetect: truePtr},
			vimtypes.VirtualDeviceDeviceBackingInfo{UseAutoDetect: falsePtr},
			false,
		),
		Entry("#5",
			vimtypes.VirtualDeviceDeviceBackingInfo{UseAutoDetect: nil},
			vimtypes.VirtualDeviceDeviceBackingInfo{UseAutoDetect: falsePtr},
			true,
		),
		Entry("#6",
			vimtypes.VirtualDeviceDeviceBackingInfo{UseAutoDetect: nil},
			vimtypes.VirtualDeviceDeviceBackingInfo{UseAutoDetect: truePtr},
			false,
		),
	)

	DescribeTable("MatchVirtualDeviceFileBackingInfo",
		func(expected, current vimtypes.VirtualDeviceFileBackingInfo, match bool) {
			m := resize.MatchVirtualDeviceFileBackingInfo(&expected, &current)
			Expect(m).To(Equal(match), cmp.Diff(current, expected))
		},

		Entry("#1",
			vimtypes.VirtualDeviceFileBackingInfo{},
			vimtypes.VirtualDeviceFileBackingInfo{},
			true,
		),
		Entry("#2",
			vimtypes.VirtualDeviceFileBackingInfo{FileName: "foo"},
			vimtypes.VirtualDeviceFileBackingInfo{FileName: "foo"},
			true,
		),
		Entry("#3",
			vimtypes.VirtualDeviceFileBackingInfo{FileName: "foo"},
			vimtypes.VirtualDeviceFileBackingInfo{FileName: "bar"},
			false,
		),
	)

	DescribeTable("MatchMatchVirtualDevicePipeBackingInfo",
		func(expected, current vimtypes.VirtualDevicePipeBackingInfo, match bool) {
			m := resize.MatchVirtualDevicePipeBackingInfo(&expected, &current)
			Expect(m).To(Equal(match), cmp.Diff(current, expected))
		},

		Entry("#1",
			vimtypes.VirtualDevicePipeBackingInfo{},
			vimtypes.VirtualDevicePipeBackingInfo{},
			true,
		),
		Entry("#2",
			vimtypes.VirtualDevicePipeBackingInfo{PipeName: "foo"},
			vimtypes.VirtualDevicePipeBackingInfo{PipeName: "foo"},
			true,
		),
		Entry("#3",
			vimtypes.VirtualDevicePipeBackingInfo{PipeName: "foo"},
			vimtypes.VirtualDevicePipeBackingInfo{PipeName: "bar"},
			false,
		),
	)

	DescribeTable("MatchVirtualDeviceURIBackingInfo",
		func(expected, current vimtypes.VirtualDeviceURIBackingInfo, match bool) {
			m := resize.MatchVirtualDeviceURIBackingInfo(&expected, &current)
			Expect(m).To(Equal(match), cmp.Diff(current, expected))
		},

		Entry("#1",
			vimtypes.VirtualDeviceURIBackingInfo{},
			vimtypes.VirtualDeviceURIBackingInfo{},
			true,
		),
		Entry("#2",
			vimtypes.VirtualDeviceURIBackingInfo{ServiceURI: "foo", Direction: "in", ProxyURI: "bar"},
			vimtypes.VirtualDeviceURIBackingInfo{ServiceURI: "foo", Direction: "in", ProxyURI: "bar"},
			true,
		),
		Entry("#3",
			vimtypes.VirtualDeviceURIBackingInfo{ServiceURI: "foo", Direction: "in", ProxyURI: "bar"},
			vimtypes.VirtualDeviceURIBackingInfo{ServiceURI: "foX", Direction: "in", ProxyURI: "bar"},
			false,
		),
		Entry("#4",
			vimtypes.VirtualDeviceURIBackingInfo{ServiceURI: "foo", Direction: "in", ProxyURI: "bar"},
			vimtypes.VirtualDeviceURIBackingInfo{ServiceURI: "foo", Direction: "iX", ProxyURI: "bar"},
			false,
		),
		Entry("#5",
			vimtypes.VirtualDeviceURIBackingInfo{ServiceURI: "foo", Direction: "in", ProxyURI: "bar"},
			vimtypes.VirtualDeviceURIBackingInfo{ServiceURI: "foo", Direction: "in", ProxyURI: "baX"},
			false,
		),
	)

	DescribeTable("MatchVirtualParallelPortDeviceBackingInfo",
		func(expected *vimtypes.VirtualParallelPortDeviceBackingInfo, current vimtypes.BaseVirtualDeviceBackingInfo, match bool) {
			m := resize.MatchVirtualParallelPortDeviceBackingInfo(expected, current)
			Expect(m).To(Equal(match), cmp.Diff(current, expected))
		},

		Entry("#1",
			&vimtypes.VirtualParallelPortDeviceBackingInfo{},
			&vimtypes.VirtualNVDIMMBackingInfo{},
			false,
		),
		Entry("#2",
			&vimtypes.VirtualParallelPortDeviceBackingInfo{
				VirtualDeviceDeviceBackingInfo: vimtypes.VirtualDeviceDeviceBackingInfo{DeviceName: "foo"},
			},
			&vimtypes.VirtualParallelPortDeviceBackingInfo{
				VirtualDeviceDeviceBackingInfo: vimtypes.VirtualDeviceDeviceBackingInfo{DeviceName: "foo"},
			},
			true,
		),
		Entry("#3",
			&vimtypes.VirtualParallelPortDeviceBackingInfo{
				VirtualDeviceDeviceBackingInfo: vimtypes.VirtualDeviceDeviceBackingInfo{DeviceName: "foo"},
			},
			&vimtypes.VirtualParallelPortDeviceBackingInfo{
				VirtualDeviceDeviceBackingInfo: vimtypes.VirtualDeviceDeviceBackingInfo{DeviceName: "bar"},
			},
			false,
		),
	)

	DescribeTable("MatchVirtualParallelPortFileBackingInfo",
		func(expected *vimtypes.VirtualParallelPortFileBackingInfo, current vimtypes.BaseVirtualDeviceBackingInfo, match bool) {
			m := resize.MatchVirtualParallelPortFileBackingInfo(expected, current)
			Expect(m).To(Equal(match), cmp.Diff(current, expected))
		},

		Entry("#1",
			&vimtypes.VirtualParallelPortFileBackingInfo{},
			&vimtypes.VirtualNVDIMMBackingInfo{},
			false,
		),
		Entry("#2",
			&vimtypes.VirtualParallelPortFileBackingInfo{
				VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: "foo"},
			},
			&vimtypes.VirtualParallelPortFileBackingInfo{
				VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: "foo"},
			},
			true,
		),
		Entry("#3",
			&vimtypes.VirtualParallelPortFileBackingInfo{
				VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: "foo"},
			},
			&vimtypes.VirtualParallelPortFileBackingInfo{
				VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: "bar"},
			},
			false,
		),
	)

	DescribeTable("MatchVirtualPointingDeviceDeviceBackingInfo",
		func(expected *vimtypes.VirtualPointingDeviceDeviceBackingInfo, current vimtypes.BaseVirtualDeviceBackingInfo, match bool) {
			m := resize.MatchVirtualPointingDeviceDeviceBackingInfo(expected, current)
			Expect(m).To(Equal(match), cmp.Diff(current, expected))
		},

		Entry("#1",
			&vimtypes.VirtualPointingDeviceDeviceBackingInfo{},
			&vimtypes.VirtualNVDIMMBackingInfo{},
			false,
		),
		Entry("#2",
			&vimtypes.VirtualPointingDeviceDeviceBackingInfo{},
			&vimtypes.VirtualPointingDeviceDeviceBackingInfo{},
			true,
		),
		Entry("#3",
			&vimtypes.VirtualPointingDeviceDeviceBackingInfo{HostPointingDevice: "foo"},
			&vimtypes.VirtualPointingDeviceDeviceBackingInfo{HostPointingDevice: "bar"},
			false,
		),
		Entry("#4",
			&vimtypes.VirtualPointingDeviceDeviceBackingInfo{HostPointingDevice: "foo"},
			&vimtypes.VirtualPointingDeviceDeviceBackingInfo{HostPointingDevice: "foo"},
			true,
		),
		Entry("#5",
			&vimtypes.VirtualPointingDeviceDeviceBackingInfo{
				VirtualDeviceDeviceBackingInfo: vimtypes.VirtualDeviceDeviceBackingInfo{DeviceName: "foo"},
			},
			&vimtypes.VirtualPointingDeviceDeviceBackingInfo{
				VirtualDeviceDeviceBackingInfo: vimtypes.VirtualDeviceDeviceBackingInfo{DeviceName: "bar"},
			},
			false,
		),
	)

	DescribeTable("MatchVirtualPrecisionClockSystemClockBackingInfo",
		func(expected *vimtypes.VirtualPrecisionClockSystemClockBackingInfo, current vimtypes.BaseVirtualDeviceBackingInfo, match bool) {
			m := resize.MatchVirtualPrecisionClockSystemClockBackingInfo(expected, current)
			Expect(m).To(Equal(match), cmp.Diff(current, expected))
		},

		Entry("#1",
			&vimtypes.VirtualPrecisionClockSystemClockBackingInfo{},
			&vimtypes.VirtualNVDIMMBackingInfo{},
			false,
		),
		Entry("#2",
			&vimtypes.VirtualPrecisionClockSystemClockBackingInfo{},
			&vimtypes.VirtualPrecisionClockSystemClockBackingInfo{},
			true,
		),
		Entry("#3",
			&vimtypes.VirtualPrecisionClockSystemClockBackingInfo{Protocol: "foo"},
			&vimtypes.VirtualPrecisionClockSystemClockBackingInfo{Protocol: "bar"},
			false,
		),
		Entry("#4",
			&vimtypes.VirtualPrecisionClockSystemClockBackingInfo{Protocol: "foo"},
			&vimtypes.VirtualPrecisionClockSystemClockBackingInfo{Protocol: "foo"},
			true,
		),
	)

	DescribeTable("MatchVirtualPointingDeviceDeviceBackingInfo",
		func(expected *vimtypes.VirtualPointingDeviceDeviceBackingInfo, current vimtypes.BaseVirtualDeviceBackingInfo, match bool) {
			m := resize.MatchVirtualPointingDeviceDeviceBackingInfo(expected, current)
			Expect(m).To(Equal(match), cmp.Diff(current, expected))
		},

		Entry("#1",
			&vimtypes.VirtualPointingDeviceDeviceBackingInfo{},
			&vimtypes.VirtualNVDIMMBackingInfo{},
			false,
		),
		Entry("#2",
			&vimtypes.VirtualPointingDeviceDeviceBackingInfo{},
			&vimtypes.VirtualPointingDeviceDeviceBackingInfo{},
			true,
		),
		Entry("#3",
			&vimtypes.VirtualPointingDeviceDeviceBackingInfo{HostPointingDevice: "foo"},
			&vimtypes.VirtualPointingDeviceDeviceBackingInfo{HostPointingDevice: "foo"},
			true,
		),
		Entry("#4",
			&vimtypes.VirtualPointingDeviceDeviceBackingInfo{HostPointingDevice: "foo"},
			&vimtypes.VirtualPointingDeviceDeviceBackingInfo{HostPointingDevice: "bar"},
			false,
		),
		Entry("#5",
			&vimtypes.VirtualPointingDeviceDeviceBackingInfo{HostPointingDevice: ""},
			&vimtypes.VirtualPointingDeviceDeviceBackingInfo{HostPointingDevice: "autodetect"},
			true,
		),
		Entry("#6",
			&vimtypes.VirtualPointingDeviceDeviceBackingInfo{
				VirtualDeviceDeviceBackingInfo: vimtypes.VirtualDeviceDeviceBackingInfo{DeviceName: "bar"},
			},
			&vimtypes.VirtualPointingDeviceDeviceBackingInfo{
				VirtualDeviceDeviceBackingInfo: vimtypes.VirtualDeviceDeviceBackingInfo{DeviceName: "bar"},
			},
			true,
		),
		Entry("#7",
			&vimtypes.VirtualPointingDeviceDeviceBackingInfo{
				VirtualDeviceDeviceBackingInfo: vimtypes.VirtualDeviceDeviceBackingInfo{DeviceName: "foo"},
			},
			&vimtypes.VirtualPointingDeviceDeviceBackingInfo{
				VirtualDeviceDeviceBackingInfo: vimtypes.VirtualDeviceDeviceBackingInfo{DeviceName: "bar"},
			},
			false,
		),
	)

	DescribeTable("MatchVirtualPrecisionClockSystemClockBackingInfo",
		func(expected *vimtypes.VirtualPrecisionClockSystemClockBackingInfo, current vimtypes.BaseVirtualDeviceBackingInfo, match bool) {
			m := resize.MatchVirtualPrecisionClockSystemClockBackingInfo(expected, current)
			Expect(m).To(Equal(match), cmp.Diff(current, expected))
		},

		Entry("#1",
			&vimtypes.VirtualPrecisionClockSystemClockBackingInfo{},
			&vimtypes.VirtualNVDIMMBackingInfo{},
			false,
		),
		Entry("#2",
			&vimtypes.VirtualPrecisionClockSystemClockBackingInfo{Protocol: "foo"},
			&vimtypes.VirtualPrecisionClockSystemClockBackingInfo{Protocol: "foo"},
			true,
		),
		Entry("#3",
			&vimtypes.VirtualPrecisionClockSystemClockBackingInfo{Protocol: "foo"},
			&vimtypes.VirtualPrecisionClockSystemClockBackingInfo{Protocol: "bar"},
			false,
		),
	)

	DescribeTable("MatchVirtualSCSIPassthroughDeviceBackingInfo",
		func(expected *vimtypes.VirtualSCSIPassthroughDeviceBackingInfo, current vimtypes.BaseVirtualDeviceBackingInfo, match bool) {
			m := resize.MatchVirtualSCSIPassthroughDeviceBackingInfo(expected, current)
			Expect(m).To(Equal(match), cmp.Diff(current, expected))
		},

		Entry("#1",
			&vimtypes.VirtualSCSIPassthroughDeviceBackingInfo{},
			&vimtypes.VirtualNVDIMMBackingInfo{},
			false,
		),
		Entry("#2",
			&vimtypes.VirtualSCSIPassthroughDeviceBackingInfo{
				VirtualDeviceDeviceBackingInfo: vimtypes.VirtualDeviceDeviceBackingInfo{DeviceName: "foo"},
			},
			&vimtypes.VirtualSCSIPassthroughDeviceBackingInfo{
				VirtualDeviceDeviceBackingInfo: vimtypes.VirtualDeviceDeviceBackingInfo{DeviceName: "foo"},
			},
			true,
		),
		Entry("#3",
			&vimtypes.VirtualSCSIPassthroughDeviceBackingInfo{
				VirtualDeviceDeviceBackingInfo: vimtypes.VirtualDeviceDeviceBackingInfo{DeviceName: "foo"},
			},
			&vimtypes.VirtualSCSIPassthroughDeviceBackingInfo{
				VirtualDeviceDeviceBackingInfo: vimtypes.VirtualDeviceDeviceBackingInfo{DeviceName: "bar"},
			},
			false,
		),
	)

	DescribeTable("MatchVirtualSerialPortDeviceBackingInfo",
		func(expected *vimtypes.VirtualSerialPortDeviceBackingInfo, current vimtypes.BaseVirtualDeviceBackingInfo, match bool) {
			m := resize.MatchVirtualSerialPortDeviceBackingInfo(expected, current)
			Expect(m).To(Equal(match), cmp.Diff(current, expected))
		},

		Entry("#1",
			&vimtypes.VirtualSerialPortDeviceBackingInfo{},
			&vimtypes.VirtualNVDIMMBackingInfo{},
			false,
		),
		Entry("#2",
			&vimtypes.VirtualSerialPortDeviceBackingInfo{},
			&vimtypes.VirtualSerialPortDeviceBackingInfo{},
			true,
		),
		Entry("#3",
			&vimtypes.VirtualSerialPortDeviceBackingInfo{
				VirtualDeviceDeviceBackingInfo: vimtypes.VirtualDeviceDeviceBackingInfo{DeviceName: "foo"},
			},
			&vimtypes.VirtualSerialPortDeviceBackingInfo{
				VirtualDeviceDeviceBackingInfo: vimtypes.VirtualDeviceDeviceBackingInfo{DeviceName: "foo"},
			},
			true,
		),
		Entry("#4",
			&vimtypes.VirtualSerialPortDeviceBackingInfo{
				VirtualDeviceDeviceBackingInfo: vimtypes.VirtualDeviceDeviceBackingInfo{DeviceName: "foo"},
			},
			&vimtypes.VirtualSerialPortDeviceBackingInfo{
				VirtualDeviceDeviceBackingInfo: vimtypes.VirtualDeviceDeviceBackingInfo{DeviceName: "bar"},
			},
			false,
		),
	)

	DescribeTable("MatchVirtualSerialPortFileBackingInfo",
		func(expected *vimtypes.VirtualSerialPortFileBackingInfo, current vimtypes.BaseVirtualDeviceBackingInfo, match bool) {
			m := resize.MatchVirtualSerialPortFileBackingInfo(expected, current)
			Expect(m).To(Equal(match), cmp.Diff(current, expected))
		},

		Entry("#1",
			&vimtypes.VirtualSerialPortFileBackingInfo{},
			&vimtypes.VirtualNVDIMMBackingInfo{},
			false,
		),
		Entry("#2",
			&vimtypes.VirtualSerialPortFileBackingInfo{},
			&vimtypes.VirtualSerialPortFileBackingInfo{},
			true,
		),
		Entry("#3",
			&vimtypes.VirtualSerialPortFileBackingInfo{
				VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: "foo"},
			},
			&vimtypes.VirtualSerialPortFileBackingInfo{
				VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: "foo"},
			},
			true,
		),
		Entry("#4",
			&vimtypes.VirtualSerialPortFileBackingInfo{
				VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: "foo"},
			},
			&vimtypes.VirtualSerialPortFileBackingInfo{
				VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: "bar"},
			},
			false,
		),
	)

	DescribeTable("MatchVirtualSerialPortPipeBackingInfo",
		func(expected *vimtypes.VirtualSerialPortPipeBackingInfo, current vimtypes.BaseVirtualDeviceBackingInfo, match bool) {
			m := resize.MatchVirtualSerialPortPipeBackingInfo(expected, current)
			Expect(m).To(Equal(match), cmp.Diff(current, expected))
		},

		Entry("#1",
			&vimtypes.VirtualSerialPortPipeBackingInfo{},
			&vimtypes.VirtualNVDIMMBackingInfo{},
			false,
		),
		Entry("#2",
			&vimtypes.VirtualSerialPortPipeBackingInfo{},
			&vimtypes.VirtualSerialPortPipeBackingInfo{},
			true,
		),
		Entry("#3",
			&vimtypes.VirtualSerialPortPipeBackingInfo{Endpoint: "foo", NoRxLoss: truePtr},
			&vimtypes.VirtualSerialPortPipeBackingInfo{Endpoint: "foo", NoRxLoss: truePtr},
			true,
		),
		Entry("#4",
			&vimtypes.VirtualSerialPortPipeBackingInfo{Endpoint: "foo", NoRxLoss: truePtr},
			&vimtypes.VirtualSerialPortPipeBackingInfo{Endpoint: "bar", NoRxLoss: truePtr},
			false,
		),
		Entry("#5",
			&vimtypes.VirtualSerialPortPipeBackingInfo{Endpoint: "foo", NoRxLoss: truePtr},
			&vimtypes.VirtualSerialPortPipeBackingInfo{Endpoint: "foo", NoRxLoss: falsePtr},
			false,
		),
		Entry("#6",
			&vimtypes.VirtualSerialPortPipeBackingInfo{Endpoint: "foo", NoRxLoss: truePtr,
				VirtualDevicePipeBackingInfo: vimtypes.VirtualDevicePipeBackingInfo{PipeName: "foo"},
			},
			&vimtypes.VirtualSerialPortPipeBackingInfo{Endpoint: "foo", NoRxLoss: truePtr,
				VirtualDevicePipeBackingInfo: vimtypes.VirtualDevicePipeBackingInfo{PipeName: "bar"},
			},
			false,
		),
	)

	DescribeTable("MatchVirtualSerialPortThinPrintBackingInfo",
		func(expected *vimtypes.VirtualSerialPortThinPrintBackingInfo, current vimtypes.BaseVirtualDeviceBackingInfo, match bool) {
			m := resize.MatchVirtualSerialPortThinPrintBackingInfo(expected, current)
			Expect(m).To(Equal(match), cmp.Diff(current, expected))
		},

		Entry("#1",
			&vimtypes.VirtualSerialPortThinPrintBackingInfo{},
			&vimtypes.VirtualNVDIMMBackingInfo{},
			false,
		),
		Entry("#2",
			&vimtypes.VirtualSerialPortThinPrintBackingInfo{},
			&vimtypes.VirtualSerialPortThinPrintBackingInfo{},
			true,
		),
	)

	DescribeTable("MatchVirtualSerialPortURIBackingInfo",
		func(expected *vimtypes.VirtualSerialPortURIBackingInfo, current vimtypes.BaseVirtualDeviceBackingInfo, match bool) {
			m := resize.MatchVirtualSerialPortURIBackingInfo(expected, current)
			Expect(m).To(Equal(match), cmp.Diff(current, expected))
		},

		Entry("#1",
			&vimtypes.VirtualSerialPortURIBackingInfo{},
			&vimtypes.VirtualNVDIMMBackingInfo{},
			false,
		),
		Entry("#2",
			&vimtypes.VirtualSerialPortURIBackingInfo{
				VirtualDeviceURIBackingInfo: vimtypes.VirtualDeviceURIBackingInfo{ServiceURI: "foo"},
			},
			&vimtypes.VirtualSerialPortURIBackingInfo{
				VirtualDeviceURIBackingInfo: vimtypes.VirtualDeviceURIBackingInfo{ServiceURI: "foo"},
			},
			true,
		),
		Entry("#3",
			&vimtypes.VirtualSerialPortURIBackingInfo{
				VirtualDeviceURIBackingInfo: vimtypes.VirtualDeviceURIBackingInfo{ServiceURI: "foo"},
			},
			&vimtypes.VirtualSerialPortURIBackingInfo{
				VirtualDeviceURIBackingInfo: vimtypes.VirtualDeviceURIBackingInfo{ServiceURI: "bar"},
			},
			false,
		),
	)

	DescribeTable("MatchVirtualSoundCardDeviceBackingInfo",
		func(expected *vimtypes.VirtualSoundCardDeviceBackingInfo, current vimtypes.BaseVirtualDeviceBackingInfo, match bool) {
			m := resize.MatchVirtualSoundCardDeviceBackingInfo(expected, current)
			Expect(m).To(Equal(match), cmp.Diff(current, expected))
		},

		Entry("#1",
			&vimtypes.VirtualSoundCardDeviceBackingInfo{},
			&vimtypes.VirtualNVDIMMBackingInfo{},
			false,
		),
		Entry("#2",
			&vimtypes.VirtualSoundCardDeviceBackingInfo{},
			&vimtypes.VirtualSoundCardDeviceBackingInfo{},
			true,
		),
		Entry("#3",
			&vimtypes.VirtualSoundCardDeviceBackingInfo{
				VirtualDeviceDeviceBackingInfo: vimtypes.VirtualDeviceDeviceBackingInfo{DeviceName: "foo"},
			},
			&vimtypes.VirtualSoundCardDeviceBackingInfo{
				VirtualDeviceDeviceBackingInfo: vimtypes.VirtualDeviceDeviceBackingInfo{DeviceName: "foo"},
			},
			true,
		),
		Entry("#4",
			&vimtypes.VirtualSoundCardDeviceBackingInfo{
				VirtualDeviceDeviceBackingInfo: vimtypes.VirtualDeviceDeviceBackingInfo{DeviceName: "foo"},
			},
			&vimtypes.VirtualSoundCardDeviceBackingInfo{
				VirtualDeviceDeviceBackingInfo: vimtypes.VirtualDeviceDeviceBackingInfo{DeviceName: "bar"},
			},
			false,
		),
	)

	DescribeTable("MatchVirtualEthernetCardNetworkBackingInfo",
		func(expected *vimtypes.VirtualEthernetCardNetworkBackingInfo, current vimtypes.BaseVirtualDeviceBackingInfo, match bool) {
			m := resize.MatchVirtualEthernetCardNetworkBackingInfo(expected, current)
			Expect(m).To(Equal(match), cmp.Diff(current, expected))
		},

		Entry("#1",
			&vimtypes.VirtualEthernetCardNetworkBackingInfo{},
			&vimtypes.VirtualNVDIMMBackingInfo{},
			false,
		),
		Entry("#2",
			&vimtypes.VirtualEthernetCardNetworkBackingInfo{},
			&vimtypes.VirtualEthernetCardNetworkBackingInfo{},
			true,
		),
		Entry("#3",
			&vimtypes.VirtualEthernetCardNetworkBackingInfo{
				VirtualDeviceDeviceBackingInfo: vimtypes.VirtualDeviceDeviceBackingInfo{DeviceName: "bar"},
			},
			&vimtypes.VirtualEthernetCardNetworkBackingInfo{
				VirtualDeviceDeviceBackingInfo: vimtypes.VirtualDeviceDeviceBackingInfo{DeviceName: "bar"},
			},
			true,
		),
	)

	DescribeTable("MatchVirtualEthernetCardDistributedVirtualPortBackingInfo",
		func(expected *vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo, current vimtypes.BaseVirtualDeviceBackingInfo, match bool) {
			m := resize.MatchVirtualEthernetCardDistributedVirtualPortBackingInfo(expected, current)
			Expect(m).To(Equal(match), cmp.Diff(current, expected))
		},

		Entry("#1",
			&vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo{},
			&vimtypes.VirtualNVDIMMBackingInfo{},
			false,
		),
		Entry("#2",
			&vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo{},
			&vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo{},
			true,
		),
		Entry("#3",
			&vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo{
				Port: vimtypes.DistributedVirtualSwitchPortConnection{
					SwitchUuid:   "foo",
					PortgroupKey: "bar",
				},
			},
			&vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo{
				Port: vimtypes.DistributedVirtualSwitchPortConnection{
					SwitchUuid:   "foo",
					PortgroupKey: "bar",
				},
			},
			true,
		),
		Entry("#4",
			&vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo{
				Port: vimtypes.DistributedVirtualSwitchPortConnection{
					SwitchUuid:   "foo",
					PortgroupKey: "foo",
				},
			},
			&vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo{
				Port: vimtypes.DistributedVirtualSwitchPortConnection{
					SwitchUuid:   "bar",
					PortgroupKey: "bar",
				},
			},
			false,
		),
	)

	DescribeTable("MatchVirtualEthernetCardOpaqueNetworkBackingInfo",
		func(expected *vimtypes.VirtualEthernetCardOpaqueNetworkBackingInfo, current vimtypes.BaseVirtualDeviceBackingInfo, match bool) {
			m := resize.MatchVirtualEthernetCardOpaqueNetworkBackingInfo(expected, current)
			Expect(m).To(Equal(match), cmp.Diff(current, expected))
		},

		Entry("#1",
			&vimtypes.VirtualEthernetCardOpaqueNetworkBackingInfo{},
			&vimtypes.VirtualNVDIMMBackingInfo{},
			false,
		),
		Entry("#2",
			&vimtypes.VirtualEthernetCardOpaqueNetworkBackingInfo{
				OpaqueNetworkId:   "foo",
				OpaqueNetworkType: "bar",
			},
			&vimtypes.VirtualEthernetCardOpaqueNetworkBackingInfo{
				OpaqueNetworkId:   "foo",
				OpaqueNetworkType: "bar",
			},
			true,
		),
		Entry("#3",
			&vimtypes.VirtualEthernetCardOpaqueNetworkBackingInfo{
				OpaqueNetworkId:   "foo",
				OpaqueNetworkType: "foo",
			},
			&vimtypes.VirtualEthernetCardOpaqueNetworkBackingInfo{
				OpaqueNetworkId:   "bar",
				OpaqueNetworkType: "bar",
			},
			false,
		),
	)

})
