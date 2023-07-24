// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	vimTypes "github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/util"
)

func newPCIPassthroughDevice(profile string) *vimTypes.VirtualPCIPassthrough {
	var dev vimTypes.VirtualPCIPassthrough
	if profile != "" {
		dev.Backing = &vimTypes.VirtualPCIPassthroughVmiopBackingInfo{
			Vgpu: profile,
		}
	} else {
		dev.Backing = &vimTypes.VirtualPCIPassthroughDynamicBackingInfo{}
	}
	return &dev
}

var _ = Describe("SelectDevices", func() {

	var (
		devIn       []vimTypes.BaseVirtualDevice
		devOut      []vimTypes.BaseVirtualDevice
		selectorFns []util.SelectDeviceFn[vimTypes.BaseVirtualDevice]
	)

	JustBeforeEach(func() {
		// Select the devices.
		devOut = util.SelectDevices(devIn, selectorFns...)
	})

	When("selecting Vmxnet3 NICs with UPTv2 enabled", func() {
		newUptv2EnabledNIC := func() *vimTypes.VirtualVmxnet3 {
			uptv2Enabled := true
			return &vimTypes.VirtualVmxnet3{Uptv2Enabled: &uptv2Enabled}
		}

		BeforeEach(func() {
			devIn = []vimTypes.BaseVirtualDevice{
				&vimTypes.VirtualPCIPassthrough{},
				newUptv2EnabledNIC(),
				&vimTypes.VirtualSriovEthernetCard{},
				&vimTypes.VirtualVmxnet3{},
				newUptv2EnabledNIC(),
			}
			selectorFns = []util.SelectDeviceFn[vimTypes.BaseVirtualDevice]{
				func(dev vimTypes.BaseVirtualDevice) bool {
					nic, ok := dev.(*vimTypes.VirtualVmxnet3)
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
			devOut := util.SelectDevicesByType[*vimTypes.VirtualPCIPassthrough](
				[]vimTypes.BaseVirtualDevice{
					&vimTypes.VirtualVmxnet3{},
					&vimTypes.VirtualPCIPassthrough{},
					&vimTypes.VirtualSriovEthernetCard{},
				},
			)
			Expect(devOut).To(BeAssignableToTypeOf([]*vimTypes.VirtualPCIPassthrough{}))
			Expect(devOut).To(HaveLen(1))
			Expect(devOut[0]).To(BeEquivalentTo(&vimTypes.VirtualPCIPassthrough{}))
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
			Expect(util.IsDeviceNvidiaVgpu(&vimTypes.VirtualCdrom{})).To(BeFalse())
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
			Expect(util.IsDeviceDynamicDirectPathIO(&vimTypes.VirtualCdrom{})).To(BeFalse())
		})
	})
})

var _ = Describe("SelectDynamicDirectPathIO", func() {
	Context("selecting a dynamic direct path I/O device", func() {
		It("will return only the selected device type", func() {
			devOut := util.SelectDynamicDirectPathIO(
				[]vimTypes.BaseVirtualDevice{
					newPCIPassthroughDevice(""),
					&vimTypes.VirtualVmxnet3{},
					newPCIPassthroughDevice("profile1"),
					&vimTypes.VirtualSriovEthernetCard{},
					newPCIPassthroughDevice(""),
					newPCIPassthroughDevice("profile2"),
				},
			)
			Expect(devOut).To(BeAssignableToTypeOf([]*vimTypes.VirtualPCIPassthrough{}))
			Expect(devOut).To(HaveLen(2))
			Expect(devOut[0].Backing).To(BeAssignableToTypeOf(&vimTypes.VirtualPCIPassthroughDynamicBackingInfo{}))
			Expect(devOut[0]).To(BeEquivalentTo(newPCIPassthroughDevice("")))
			Expect(devOut[1].Backing).To(BeAssignableToTypeOf(&vimTypes.VirtualPCIPassthroughDynamicBackingInfo{}))
			Expect(devOut[1]).To(BeEquivalentTo(newPCIPassthroughDevice("")))
		})
	})
})

var _ = Describe("SelectNvidiaVgpu", func() {
	Context("selecting Nvidia vGPU devices", func() {
		It("will return only the selected device type", func() {
			devOut := util.SelectNvidiaVgpu(
				[]vimTypes.BaseVirtualDevice{
					newPCIPassthroughDevice(""),
					&vimTypes.VirtualVmxnet3{},
					newPCIPassthroughDevice("profile1"),
					&vimTypes.VirtualSriovEthernetCard{},
					newPCIPassthroughDevice(""),
					newPCIPassthroughDevice("profile2"),
				},
			)
			Expect(devOut).To(BeAssignableToTypeOf([]*vimTypes.VirtualPCIPassthrough{}))
			Expect(devOut).To(HaveLen(2))
			Expect(devOut[0].Backing).To(BeAssignableToTypeOf(&vimTypes.VirtualPCIPassthroughVmiopBackingInfo{}))
			Expect(devOut[0]).To(BeEquivalentTo(newPCIPassthroughDevice("profile1")))
			Expect(devOut[1].Backing).To(BeAssignableToTypeOf(&vimTypes.VirtualPCIPassthroughVmiopBackingInfo{}))
			Expect(devOut[1]).To(BeEquivalentTo(newPCIPassthroughDevice("profile2")))
		})
	})
})

var _ = Describe("SelectDevicesByTypes", func() {

	var (
		devIn  []vimTypes.BaseVirtualDevice
		devOut []vimTypes.BaseVirtualDevice
		devT2S []vimTypes.BaseVirtualDevice
	)

	BeforeEach(func() {
		devIn = []vimTypes.BaseVirtualDevice{
			&vimTypes.VirtualPCIPassthrough{},
			&vimTypes.VirtualSriovEthernetCard{},
			&vimTypes.VirtualVmxnet3{},
		}
	})

	JustBeforeEach(func() {
		devOut = util.SelectDevicesByTypes(devIn, devT2S...)
	})

	Context("selecting a VirtualPCIPassthrough", func() {
		BeforeEach(func() {
			devT2S = []vimTypes.BaseVirtualDevice{
				&vimTypes.VirtualPCIPassthrough{},
			}
		})
		It("will return only the selected device type(s)", func() {
			Expect(devOut).To(HaveLen(1))
			Expect(devOut[0]).To(BeEquivalentTo(&vimTypes.VirtualPCIPassthrough{}))
		})
	})

	Context("selecting a VirtualSriovEthernetCard and VirtualVmxnet3", func() {
		BeforeEach(func() {
			devT2S = []vimTypes.BaseVirtualDevice{
				&vimTypes.VirtualSriovEthernetCard{},
				&vimTypes.VirtualVmxnet3{},
			}
		})
		It("will return only the selected device type(s)", func() {
			Expect(devOut).To(HaveLen(2))
			Expect(devOut[0]).To(BeEquivalentTo(&vimTypes.VirtualSriovEthernetCard{}))
			Expect(devOut[1]).To(BeEquivalentTo(&vimTypes.VirtualVmxnet3{}))
		})
	})

	Context("selecting a type of device not in the ConfigSpec", func() {
		BeforeEach(func() {
			devT2S = []vimTypes.BaseVirtualDevice{
				&vimTypes.VirtualDisk{},
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
