// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	"encoding/base64"
	"reflect"

	"k8s.io/utils/pointer"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	vimTypes "github.com/vmware/govmomi/vim25/types"
	"github.com/vmware/govmomi/vim25/xml"

	"github.com/vmware-tanzu/vm-operator/pkg/util"
)

var _ = Describe("DevicesFromConfigSpec", func() {
	var (
		devOut     []vimTypes.BaseVirtualDevice
		configSpec *vimTypes.VirtualMachineConfigSpec
	)

	BeforeEach(func() {
		configSpec = &vimTypes.VirtualMachineConfigSpec{
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
			configSpec = &vimTypes.VirtualMachineConfigSpec{
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

var _ = Describe("ConfigSpec Util", func() {
	Context("MarshalConfigSpecToXML", func() {
		It("marshals and unmarshal to the same spec", func() {
			inputSpec := &vimTypes.VirtualMachineConfigSpec{Name: "dummy-VM"}
			bytes, err := util.MarshalConfigSpecToXML(inputSpec)
			Expect(err).ShouldNot(HaveOccurred())
			var outputSpec vimTypes.VirtualMachineConfigSpec
			err = xml.Unmarshal(bytes, &outputSpec)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(reflect.DeepEqual(inputSpec, &outputSpec)).To(BeTrue())
		})

		It("marshals spec correctly to expected base64 encoded XML", func() {
			inputSpec := &vimTypes.VirtualMachineConfigSpec{Name: "dummy-VM"}
			bytes, err := util.MarshalConfigSpecToXML(inputSpec)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(base64.StdEncoding.EncodeToString(bytes)).To(Equal("PG9iaiB4bWxuczp2aW0yNT0idXJuOnZpbTI1I" +
				"iB4bWxuczp4c2k9Imh0dHA6Ly93d3cudzMub3JnLzIwMDEvWE1MU2NoZW1hLWluc3RhbmNlIiB4c2k6dHlwZT0idmltMjU6Vmlyd" +
				"HVhbE1hY2hpbmVDb25maWdTcGVjIj48bmFtZT5kdW1teS1WTTwvbmFtZT48L29iaj4="))
		})
	})
})

var _ = Describe("RemoveDevicesFromConfigSpec", func() {
	var (
		configSpec *vimTypes.VirtualMachineConfigSpec
		fn         func(vimTypes.BaseVirtualDevice) bool
	)

	BeforeEach(func() {
		configSpec = &vimTypes.VirtualMachineConfigSpec{
			Name:         "dummy-VM",
			DeviceChange: []vimTypes.BaseVirtualDeviceConfigSpec{},
		}
	})

	When("provided a config spec with a disk and func to remove VirtualDisk type", func() {
		BeforeEach(func() {
			configSpec.DeviceChange = []vimTypes.BaseVirtualDeviceConfigSpec{
				&vimTypes.VirtualDeviceConfigSpec{
					Operation: vimTypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimTypes.VirtualDisk{
						CapacityInBytes: 1024,
						VirtualDevice: vimTypes.VirtualDevice{
							Key: -42,
							Backing: &vimTypes.VirtualDiskFlatVer2BackingInfo{
								ThinProvisioned: pointer.Bool(true),
							},
						},
					},
				},
			}

			fn = func(dev vimTypes.BaseVirtualDevice) bool {
				switch dev.(type) {
				case *vimTypes.VirtualDisk:
					return true
				default:
					return false
				}
			}
		})

		It("config spec deviceChanges empty", func() {
			util.RemoveDevicesFromConfigSpec(configSpec, fn)
			Expect(configSpec.DeviceChange).To(BeEmpty())
		})
	})
})

var _ = Describe("ProcessVMClassConfigSpecExclusions", func() {
	var (
		configSpec *vimTypes.VirtualMachineConfigSpec
	)

	BeforeEach(func() {
		configSpec = &vimTypes.VirtualMachineConfigSpec{
			Name:       "dummy-VM",
			Annotation: "test-annotation",
			Files:      &vimTypes.VirtualMachineFileInfo{},
			VmProfile: []vimTypes.BaseVirtualMachineProfileSpec{
				&vimTypes.VirtualMachineDefinedProfileSpec{
					ProfileId: "dummy-id",
				},
			},
			DeviceChange: []vimTypes.BaseVirtualDeviceConfigSpec{
				&vimTypes.VirtualDeviceConfigSpec{
					Operation: vimTypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimTypes.VirtualSATAController{
						VirtualController: vimTypes.VirtualController{},
					},
				},
				&vimTypes.VirtualDeviceConfigSpec{
					Operation: vimTypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimTypes.VirtualIDEController{
						VirtualController: vimTypes.VirtualController{},
					},
				},
				&vimTypes.VirtualDeviceConfigSpec{
					Operation: vimTypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimTypes.VirtualSCSIController{
						VirtualController: vimTypes.VirtualController{},
					},
				},
				&vimTypes.VirtualDeviceConfigSpec{
					Operation: vimTypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimTypes.VirtualNVMEController{
						VirtualController: vimTypes.VirtualController{},
					},
				},
				&vimTypes.VirtualDeviceConfigSpec{
					Operation: vimTypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimTypes.VirtualE1000{
						VirtualEthernetCard: vimTypes.VirtualEthernetCard{
							VirtualDevice: vimTypes.VirtualDevice{
								Key: 4000,
							},
						},
					},
				},
				&vimTypes.VirtualDeviceConfigSpec{
					Operation: vimTypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimTypes.VirtualDisk{
						CapacityInBytes: 1024 * 1024,
						VirtualDevice: vimTypes.VirtualDevice{
							Key: -42,
							Backing: &vimTypes.VirtualDiskFlatVer2BackingInfo{
								ThinProvisioned: pointer.Bool(true),
							},
						},
					},
				},
			},
		}
	})

	When("provided a config spec with a disk, disk controllers, files, vmprofile", func() {
		It("config spec has all exclusions removed", func() {
			util.ProcessVMClassConfigSpecExclusions(configSpec)
			Expect(configSpec.Name).To(Equal("dummy-VM"))
			Expect(configSpec.Annotation).ToNot(BeEmpty())
			Expect(configSpec.Annotation).To(Equal("test-annotation"))
			Expect(configSpec.Files).To(BeNil())
			Expect(configSpec.VmProfile).To(BeEmpty())
			Expect(configSpec.DeviceChange).To(HaveLen(1))
			dSpec := configSpec.DeviceChange[0].GetVirtualDeviceConfigSpec()
			_, ok := dSpec.Device.(*vimTypes.VirtualE1000)
			Expect(ok).To(BeTrue())
		})
	})
})
