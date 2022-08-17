// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	vimTypes "github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/util"
)

var _ = Describe("DevicesFromConfigSpec", func() {
	var (
		devOut     []vimTypes.BaseVirtualDevice
		configSpec vimTypes.VirtualMachineConfigSpec
	)

	BeforeEach(func() {
		configSpec = vimTypes.VirtualMachineConfigSpec{
			DeviceChange: []vimTypes.BaseVirtualDeviceConfigSpec{
				&vimTypes.VirtualDeviceConfigSpec{
					Device: &vimTypes.VirtualPCIPassthrough{},
				},
				&vimTypes.VirtualDeviceConfigSpec{
					Device: &vimTypes.VirtualSriovEthernetCard{},
				},
				&vimTypes.VirtualDeviceConfigSpec{
					Device: &vimTypes.VirtualVmxnet3{},
				},
			},
		}
	})

	JustBeforeEach(func() {
		devOut = util.DevicesFromConfigSpec(configSpec)
	})

	When("a ConfigSpec has a nil DeviceChange property", func() {
		BeforeEach(func() {
			configSpec.DeviceChange = nil
		})
		It("will not panic", func() {
			Expect(devOut).To(HaveLen(0))
		})
	})

	When("a ConfigSpec has an empty DeviceChange property", func() {
		BeforeEach(func() {
			configSpec.DeviceChange = []vimTypes.BaseVirtualDeviceConfigSpec{}
		})
		It("will not panic", func() {
			Expect(devOut).To(HaveLen(0))
		})
	})

	When("a ConfigSpec has a VirtualPCIPassthrough device, SR-IOV NIC, and Vmxnet3 NIC", func() {
		It("returns the devices in insertion order", func() {
			Expect(devOut).To(HaveLen(3))
			Expect(devOut[0]).To(BeEquivalentTo(&vimTypes.VirtualPCIPassthrough{}))
			Expect(devOut[1]).To(BeEquivalentTo(&vimTypes.VirtualSriovEthernetCard{}))
			Expect(devOut[2]).To(BeEquivalentTo(&vimTypes.VirtualVmxnet3{}))
		})
	})

	When("a ConfigSpec has one or more DeviceChanges with a nil Device", func() {
		BeforeEach(func() {
			configSpec = vimTypes.VirtualMachineConfigSpec{
				DeviceChange: []vimTypes.BaseVirtualDeviceConfigSpec{
					&vimTypes.VirtualDeviceConfigSpec{},
					&vimTypes.VirtualDeviceConfigSpec{
						Device: &vimTypes.VirtualPCIPassthrough{},
					},
					&vimTypes.VirtualDeviceConfigSpec{},
					&vimTypes.VirtualDeviceConfigSpec{
						Device: &vimTypes.VirtualSriovEthernetCard{},
					},
					&vimTypes.VirtualDeviceConfigSpec{
						Device: &vimTypes.VirtualVmxnet3{},
					},
					&vimTypes.VirtualDeviceConfigSpec{},
					&vimTypes.VirtualDeviceConfigSpec{},
				},
			}
		})

		It("will still return only the expected device(s)", func() {
			Expect(devOut).To(HaveLen(3))
			Expect(devOut[0]).To(BeEquivalentTo(&vimTypes.VirtualPCIPassthrough{}))
			Expect(devOut[1]).To(BeEquivalentTo(&vimTypes.VirtualSriovEthernetCard{}))
			Expect(devOut[2]).To(BeEquivalentTo(&vimTypes.VirtualVmxnet3{}))
		})
	})
})
