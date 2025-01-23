// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/util"
)

func newPCIPassthroughDevice(profile string) *vimtypes.VirtualPCIPassthrough {
	var dev vimtypes.VirtualPCIPassthrough
	if profile != "" {
		dev.Backing = &vimtypes.VirtualPCIPassthroughVmiopBackingInfo{
			Vgpu: profile,
		}
	} else {
		dev.Backing = &vimtypes.VirtualPCIPassthroughDynamicBackingInfo{}
	}
	return &dev
}

var _ = Describe("SelectDevices", func() {

	var (
		devIn       []vimtypes.BaseVirtualDevice
		devOut      []vimtypes.BaseVirtualDevice
		selectorFns []util.SelectDeviceFn[vimtypes.BaseVirtualDevice]
	)

	JustBeforeEach(func() {
		// Select the devices.
		devOut = util.SelectDevices(devIn, selectorFns...)
	})

	When("selecting Vmxnet3 NICs with UPTv2 enabled", func() {
		newUptv2EnabledNIC := func() *vimtypes.VirtualVmxnet3 {
			uptv2Enabled := true
			return &vimtypes.VirtualVmxnet3{Uptv2Enabled: &uptv2Enabled}
		}

		BeforeEach(func() {
			devIn = []vimtypes.BaseVirtualDevice{
				&vimtypes.VirtualPCIPassthrough{},
				newUptv2EnabledNIC(),
				&vimtypes.VirtualSriovEthernetCard{},
				&vimtypes.VirtualVmxnet3{},
				newUptv2EnabledNIC(),
			}
			selectorFns = []util.SelectDeviceFn[vimtypes.BaseVirtualDevice]{
				func(dev vimtypes.BaseVirtualDevice) bool {
					nic, ok := dev.(*vimtypes.VirtualVmxnet3)
					return ok && nic.Uptv2Enabled != nil && *nic.Uptv2Enabled
				},
			}
		})
		It("selects only the expected device(s)", func() {
			Expect(devOut).To(HaveLen(2))
			Expect(devOut[0]).To(BeEquivalentTo(newUptv2EnabledNIC()))
			Expect(devOut[1]).To(BeEquivalentTo(newUptv2EnabledNIC()))
		})
	})
})

var _ = Describe("SelectDevicesByType", func() {
	Context("selecting a VirtualPCIPassthrough", func() {
		It("will return only the selected device type", func() {
			devOut := util.SelectDevicesByType[*vimtypes.VirtualPCIPassthrough](
				[]vimtypes.BaseVirtualDevice{
					&vimtypes.VirtualVmxnet3{},
					&vimtypes.VirtualPCIPassthrough{},
					&vimtypes.VirtualSriovEthernetCard{},
				},
			)
			Expect(devOut).To(BeAssignableToTypeOf([]*vimtypes.VirtualPCIPassthrough{}))
			Expect(devOut).To(HaveLen(1))
			Expect(devOut[0]).To(BeEquivalentTo(&vimtypes.VirtualPCIPassthrough{}))
		})
	})
})

var _ = Describe("IsDeviceNvidiaVgpu", func() {
	Context("a VGPU", func() {
		It("will return true", func() {
			Expect(util.IsDeviceNvidiaVgpu(newPCIPassthroughDevice("profile1"))).To(BeTrue())
		})
	})
	Context("a dynamic direct path I/O device", func() {
		It("will return false", func() {
			Expect(util.IsDeviceNvidiaVgpu(newPCIPassthroughDevice(""))).To(BeFalse())
		})
	})
	Context("a virtual CD-ROM", func() {
		It("will return false", func() {
			Expect(util.IsDeviceNvidiaVgpu(&vimtypes.VirtualCdrom{})).To(BeFalse())
		})
	})
})

var _ = Describe("IsDeviceDynamicDirectPathIO", func() {
	Context("a VGPU", func() {
		It("will return false", func() {
			Expect(util.IsDeviceDynamicDirectPathIO(newPCIPassthroughDevice("profile1"))).To(BeFalse())
		})
	})
	Context("a dynamic direct path I/O device", func() {
		It("will return true", func() {
			Expect(util.IsDeviceDynamicDirectPathIO(newPCIPassthroughDevice(""))).To(BeTrue())
		})
	})
	Context("a virtual CD-ROM", func() {
		It("will return false", func() {
			Expect(util.IsDeviceDynamicDirectPathIO(&vimtypes.VirtualCdrom{})).To(BeFalse())
		})
	})
})

var _ = Describe("SelectDynamicDirectPathIO", func() {
	Context("selecting a dynamic direct path I/O device", func() {
		It("will return only the selected device type", func() {
			devOut := util.SelectDynamicDirectPathIO(
				[]vimtypes.BaseVirtualDevice{
					newPCIPassthroughDevice(""),
					&vimtypes.VirtualVmxnet3{},
					newPCIPassthroughDevice("profile1"),
					&vimtypes.VirtualSriovEthernetCard{},
					newPCIPassthroughDevice(""),
					newPCIPassthroughDevice("profile2"),
				},
			)
			Expect(devOut).To(BeAssignableToTypeOf([]*vimtypes.VirtualPCIPassthrough{}))
			Expect(devOut).To(HaveLen(2))
			Expect(devOut[0].Backing).To(BeAssignableToTypeOf(&vimtypes.VirtualPCIPassthroughDynamicBackingInfo{}))
			Expect(devOut[0]).To(BeEquivalentTo(newPCIPassthroughDevice("")))
			Expect(devOut[1].Backing).To(BeAssignableToTypeOf(&vimtypes.VirtualPCIPassthroughDynamicBackingInfo{}))
			Expect(devOut[1]).To(BeEquivalentTo(newPCIPassthroughDevice("")))
		})
	})
})

var _ = Describe("HasVirtualPCIPassthroughDeviceChange", func() {

	var (
		devices []vimtypes.BaseVirtualDeviceConfigSpec
		has     bool
	)

	JustBeforeEach(func() {
		has = util.HasVirtualPCIPassthroughDeviceChange(devices)
	})

	AfterEach(func() {
		devices = nil
	})

	Context("empty list", func() {
		It("return false", func() {
			Expect(has).To(BeFalse())
		})
	})

	Context("non passthrough device", func() {
		BeforeEach(func() {
			devices = append(devices, &vimtypes.VirtualDeviceConfigSpec{
				Device: &vimtypes.VirtualVmxnet3{},
			})
		})

		It("returns false", func() {
			Expect(has).To(BeFalse())
		})
	})

	Context("vGPU device", func() {
		BeforeEach(func() {
			devices = append(devices,
				&vimtypes.VirtualDeviceConfigSpec{
					Device: &vimtypes.VirtualVmxnet3{},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Device: newPCIPassthroughDevice(""),
				},
			)
		})

		It("returns true", func() {
			Expect(has).To(BeTrue())
		})
	})

	Context("DDPIO device", func() {
		BeforeEach(func() {
			devices = append(devices,
				&vimtypes.VirtualDeviceConfigSpec{
					Device: &vimtypes.VirtualVmxnet3{},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Device: newPCIPassthroughDevice("profile1"),
				},
			)
		})

		It("returns true", func() {
			Expect(has).To(BeTrue())
		})
	})

})

var _ = Describe("SelectNvidiaVgpu", func() {
	Context("selecting Nvidia vGPU devices", func() {
		It("will return only the selected device type", func() {
			devOut := util.SelectNvidiaVgpu(
				[]vimtypes.BaseVirtualDevice{
					newPCIPassthroughDevice(""),
					&vimtypes.VirtualVmxnet3{},
					newPCIPassthroughDevice("profile1"),
					&vimtypes.VirtualSriovEthernetCard{},
					newPCIPassthroughDevice(""),
					newPCIPassthroughDevice("profile2"),
				},
			)
			Expect(devOut).To(BeAssignableToTypeOf([]*vimtypes.VirtualPCIPassthrough{}))
			Expect(devOut).To(HaveLen(2))
			Expect(devOut[0].Backing).To(BeAssignableToTypeOf(&vimtypes.VirtualPCIPassthroughVmiopBackingInfo{}))
			Expect(devOut[0]).To(BeEquivalentTo(newPCIPassthroughDevice("profile1")))
			Expect(devOut[1].Backing).To(BeAssignableToTypeOf(&vimtypes.VirtualPCIPassthroughVmiopBackingInfo{}))
			Expect(devOut[1]).To(BeEquivalentTo(newPCIPassthroughDevice("profile2")))
		})
	})
})

var _ = Describe("SelectDevicesByTypes", func() {

	var (
		devIn  []vimtypes.BaseVirtualDevice
		devOut []vimtypes.BaseVirtualDevice
		devT2S []vimtypes.BaseVirtualDevice
	)

	BeforeEach(func() {
		devIn = []vimtypes.BaseVirtualDevice{
			&vimtypes.VirtualPCIPassthrough{},
			&vimtypes.VirtualSriovEthernetCard{},
			&vimtypes.VirtualVmxnet3{},
		}
	})

	JustBeforeEach(func() {
		devOut = util.SelectDevicesByTypes(devIn, devT2S...)
	})

	Context("selecting a VirtualPCIPassthrough", func() {
		BeforeEach(func() {
			devT2S = []vimtypes.BaseVirtualDevice{
				&vimtypes.VirtualPCIPassthrough{},
			}
		})
		It("will return only the selected device type(s)", func() {
			Expect(devOut).To(HaveLen(1))
			Expect(devOut[0]).To(BeEquivalentTo(&vimtypes.VirtualPCIPassthrough{}))
		})
	})

	Context("selecting a VirtualSriovEthernetCard and VirtualVmxnet3", func() {
		BeforeEach(func() {
			devT2S = []vimtypes.BaseVirtualDevice{
				&vimtypes.VirtualSriovEthernetCard{},
				&vimtypes.VirtualVmxnet3{},
			}
		})
		It("will return only the selected device type(s)", func() {
			Expect(devOut).To(HaveLen(2))
			Expect(devOut[0]).To(BeEquivalentTo(&vimtypes.VirtualSriovEthernetCard{}))
			Expect(devOut[1]).To(BeEquivalentTo(&vimtypes.VirtualVmxnet3{}))
		})
	})

	Context("selecting a type of device not in the ConfigSpec", func() {
		BeforeEach(func() {
			devT2S = []vimtypes.BaseVirtualDevice{
				&vimtypes.VirtualDisk{},
			}
		})
		It("will not return any devices", func() {
			Expect(devOut).To(HaveLen(0))
		})
	})

	Context("selecting no device types", func() {
		It("will not return any devices", func() {
			Expect(devOut).To(HaveLen(0))
		})
	})
})

var _ = Describe("GetPreferredDiskFormat", func() {

	DescribeTable("[]string",
		func(in []string, exp vimtypes.DatastoreSectorFormat) {
			Expect(util.GetPreferredDiskFormat(in...)).To(Equal(exp))
		},
		Entry(
			"no available formats",
			[]string{},
			vimtypes.DatastoreSectorFormat(""),
		),
		Entry(
			"4kn is available",
			[]string{
				string(vimtypes.DatastoreSectorFormatEmulated_512),
				string(vimtypes.DatastoreSectorFormatNative_512),
				string(vimtypes.DatastoreSectorFormatNative_4k),
			},
			vimtypes.DatastoreSectorFormatNative_4k,
		),
		Entry(
			"native 512 is available",
			[]string{
				string(vimtypes.DatastoreSectorFormatEmulated_512),
				string(vimtypes.DatastoreSectorFormatNative_512),
			},
			vimtypes.DatastoreSectorFormatNative_512,
		),
		Entry(
			"neither 4kn nor 512 are available",
			[]string{
				string(vimtypes.DatastoreSectorFormatEmulated_512),
			},
			vimtypes.DatastoreSectorFormatEmulated_512,
		),
	)

	DescribeTable("[]vimtypes.DatastoreSectorFormat",
		func(in []vimtypes.DatastoreSectorFormat, exp vimtypes.DatastoreSectorFormat) {
			Expect(util.GetPreferredDiskFormat(in...)).To(Equal(exp))
		},
		Entry(
			"no available formats",
			[]vimtypes.DatastoreSectorFormat{},
			vimtypes.DatastoreSectorFormat(""),
		),
		Entry(
			"4kn is available",
			[]vimtypes.DatastoreSectorFormat{
				vimtypes.DatastoreSectorFormatEmulated_512,
				vimtypes.DatastoreSectorFormatNative_512,
				vimtypes.DatastoreSectorFormatNative_4k,
			},
			vimtypes.DatastoreSectorFormatNative_4k,
		),
		Entry(
			"native 512 is available",
			[]vimtypes.DatastoreSectorFormat{
				vimtypes.DatastoreSectorFormatEmulated_512,
				vimtypes.DatastoreSectorFormatNative_512,
			},
			vimtypes.DatastoreSectorFormatNative_512,
		),
		Entry(
			"neither 4kn nor 512 are available",
			[]vimtypes.DatastoreSectorFormat{
				vimtypes.DatastoreSectorFormatEmulated_512,
			},
			vimtypes.DatastoreSectorFormatEmulated_512,
		),
	)
})
