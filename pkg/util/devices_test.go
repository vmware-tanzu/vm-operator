// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
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

var _ = Describe("GetVirtualDiskInfo", func() {
	Context("DiskMode extraction", func() {
		When("disk has FlatVer2 backing with persistent mode", func() {
			It("should extract info correctly", func() {
				disk := &vimtypes.VirtualDisk{
					VirtualDevice: vimtypes.VirtualDevice{
						Key:           2000,
						ControllerKey: 1000,
						UnitNumber:    ptr.To(int32(0)),
						Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
							VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
								FileName: "[datastore1] vm/disk.vmdk",
							},
							DiskMode: string(vimtypes.VirtualDiskModePersistent),
							Uuid:     "disk-uuid-123",
						},
						DeviceInfo: &vimtypes.Description{
							Label: "Hard disk 1",
						},
					},
					CapacityInBytes: 10737418240,
				}
				info := util.GetVirtualDiskInfo(disk)
				Expect(info.DiskMode).To(Equal(vimtypes.VirtualDiskModePersistent))
				Expect(info.UUID).To(Equal("disk-uuid-123"))
				Expect(info.Label).To(Equal("Hard disk 1"))
			})
		})

		When("disk has FlatVer2 backing with independent_persistent mode", func() {
			It("should extract info correctly", func() {
				disk := &vimtypes.VirtualDisk{
					VirtualDevice: vimtypes.VirtualDevice{
						Key:           2000,
						ControllerKey: 1000,
						UnitNumber:    ptr.To(int32(0)),
						Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
							VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
								FileName: "[datastore1] vm/disk.vmdk",
							},
							DiskMode: string(vimtypes.VirtualDiskModeIndependent_persistent),
							Uuid:     "disk-uuid-456",
						},
					},
					CapacityInBytes: 10737418240,
				}
				info := util.GetVirtualDiskInfo(disk)
				Expect(info.DiskMode).To(Equal(vimtypes.VirtualDiskModeIndependent_persistent))
			})
		})

		When("disk has SparseVer2 backing with nonpersistent mode", func() {
			It("should extract info correctly", func() {
				disk := &vimtypes.VirtualDisk{
					VirtualDevice: vimtypes.VirtualDevice{
						Key:           2000,
						ControllerKey: 1000,
						UnitNumber:    ptr.To(int32(0)),
						Backing: &vimtypes.VirtualDiskSparseVer2BackingInfo{
							VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
								FileName: "[datastore1] vm/disk.vmdk",
							},
							DiskMode: string(vimtypes.VirtualDiskModeNonpersistent),
							Uuid:     "disk-uuid-789",
						},
					},
					CapacityInBytes: 10737418240,
				}
				info := util.GetVirtualDiskInfo(disk)
				Expect(info.DiskMode).To(Equal(vimtypes.VirtualDiskModeNonpersistent))
			})
		})

		When("disk has SeSparse backing with undoable mode", func() {
			It("should extract info correctly", func() {
				disk := &vimtypes.VirtualDisk{
					VirtualDevice: vimtypes.VirtualDevice{
						Key:           2000,
						ControllerKey: 1000,
						UnitNumber:    ptr.To(int32(0)),
						Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
							VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
								FileName: "[datastore1] vm/disk.vmdk",
							},
							DiskMode: string(vimtypes.VirtualDiskModeUndoable),
							Uuid:     "disk-uuid-abc",
						},
					},
					CapacityInBytes: 10737418240,
				}
				info := util.GetVirtualDiskInfo(disk)
				Expect(info.DiskMode).To(Equal(vimtypes.VirtualDiskModeUndoable))
			})
		})

		When("disk has FlatVer2 backing with append mode", func() {
			It("should extract info correctly", func() {
				disk := &vimtypes.VirtualDisk{
					VirtualDevice: vimtypes.VirtualDevice{
						Key:           2000,
						ControllerKey: 1000,
						UnitNumber:    ptr.To(int32(0)),
						Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
							VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
								FileName: "[datastore1] vm/disk.vmdk",
							},
							DiskMode: string(vimtypes.VirtualDiskModeAppend),
							Uuid:     "disk-uuid-def",
						},
					},
					CapacityInBytes: 10737418240,
				}
				info := util.GetVirtualDiskInfo(disk)
				Expect(info.DiskMode).To(Equal(vimtypes.VirtualDiskModeAppend))
			})
		})

		When("disk has FlatVer2 backing with MultiWriter sharing", func() {
			It("should extract info correctly", func() {
				disk := &vimtypes.VirtualDisk{
					VirtualDevice: vimtypes.VirtualDevice{
						Key:           2000,
						ControllerKey: 1000,
						UnitNumber:    ptr.To(int32(0)),
						Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
							VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
								FileName: "[datastore1] vm/disk.vmdk",
							},
							DiskMode: string(vimtypes.VirtualDiskModeIndependent_persistent),
							Sharing:  string(vimtypes.VirtualDiskSharingSharingMultiWriter),
							Uuid:     "disk-uuid-ghi",
						},
					},
					CapacityInBytes: 10737418240,
				}
				info := util.GetVirtualDiskInfo(disk)
				Expect(info.DiskMode).To(Equal(vimtypes.VirtualDiskModeIndependent_persistent))
				Expect(info.Sharing).To(Equal(vimtypes.VirtualDiskSharingSharingMultiWriter))
			})
		})

		When("disk has RawDiskMappingVer1 backing with DiskMode", func() {
			It("should extract info correctly", func() {
				disk := &vimtypes.VirtualDisk{
					VirtualDevice: vimtypes.VirtualDevice{
						Key:           2000,
						ControllerKey: 1000,
						UnitNumber:    ptr.To(int32(0)),
						Backing: &vimtypes.VirtualDiskRawDiskMappingVer1BackingInfo{
							VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
								FileName: "[datastore1] vm/rdm.vmdk",
							},
							DiskMode: string(vimtypes.VirtualDiskModePersistent),
							Uuid:     "rdm-uuid-123",
						},
					},
					CapacityInBytes: 10737418240,
				}
				info := util.GetVirtualDiskInfo(disk)
				Expect(info.DiskMode).To(Equal(vimtypes.VirtualDiskModePersistent))
			})
		})

		When("disk has LocalPMem backing with DiskMode", func() {
			It("should extract info correctly", func() {
				disk := &vimtypes.VirtualDisk{
					VirtualDevice: vimtypes.VirtualDevice{
						Key:           2000,
						ControllerKey: 1000,
						UnitNumber:    ptr.To(int32(0)),
						Backing: &vimtypes.VirtualDiskLocalPMemBackingInfo{
							VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
								FileName: "[datastore1] vm/pmem.vmdk",
							},
							DiskMode: string(vimtypes.VirtualDiskModePersistent),
							Uuid:     "pmem-uuid-123",
						},
					},
					CapacityInBytes: 10737418240,
				}
				info := util.GetVirtualDiskInfo(disk)
				Expect(info.DiskMode).To(Equal(vimtypes.VirtualDiskModePersistent))
			})
		})
	})
})

var _ = Describe("GetVolumeDiskModeFromDiskMode", func() {
	DescribeTable("disk mode conversion",
		func(diskMode vimtypes.VirtualDiskMode, expectedDiskMode vmopv1.VolumeDiskMode, expectError bool) {
			volumeDiskMode, err := util.GetVolumeDiskModeFromDiskMode(diskMode)
			if expectError {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unsupported disk mode"))
			} else {
				Expect(err).ToNot(HaveOccurred())
				Expect(volumeDiskMode).To(Equal(expectedDiskMode))
			}
		},
		Entry("persistent mode",
			vimtypes.VirtualDiskModePersistent,
			vmopv1.VolumeDiskModePersistent,
			false,
		),
		Entry("independent_persistent mode",
			vimtypes.VirtualDiskModeIndependent_persistent,
			vmopv1.VolumeDiskModeIndependentPersistent,
			false,
		),
		Entry("nonpersistent mode",
			vimtypes.VirtualDiskModeNonpersistent,
			vmopv1.VolumeDiskModeNonPersistent,
			false,
		),
		Entry("independent_nonpersistent mode",
			vimtypes.VirtualDiskModeIndependent_nonpersistent,
			vmopv1.VolumeDiskModeIndependentNonPersistent,
			false,
		),
		Entry("empty disk mode defaults to Persistent",
			vimtypes.VirtualDiskMode(""),
			vmopv1.VolumeDiskModePersistent,
			false,
		),
		Entry("undoable mode is unsupported",
			vimtypes.VirtualDiskModeUndoable,
			vmopv1.VolumeDiskMode(""),
			true,
		),
		Entry("append mode is unsupported",
			vimtypes.VirtualDiskModeAppend,
			vmopv1.VolumeDiskMode(""),
			true,
		),
		Entry("unknown mode is unsupported",
			vimtypes.VirtualDiskMode("unknown_mode"),
			vmopv1.VolumeDiskMode(""),
			true,
		),
	)
})

var _ = Describe("GetVolumeSharingModeFromDiskSharing", func() {
	DescribeTable("sharing mode conversion",
		func(diskSharing vimtypes.VirtualDiskSharing, expectedSharingMode vmopv1.VolumeSharingMode, expectError bool) {
			volumeSharingMode, err := util.GetVolumeSharingModeFromDiskSharing(diskSharing)
			if expectError {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unsupported sharing mode"))
			} else {
				Expect(err).ToNot(HaveOccurred())
				Expect(volumeSharingMode).To(Equal(expectedSharingMode))
			}
		},
		Entry("sharingNone mode",
			vimtypes.VirtualDiskSharingSharingNone,
			vmopv1.VolumeSharingModeNone,
			false,
		),
		Entry("sharingMultiWriter mode",
			vimtypes.VirtualDiskSharingSharingMultiWriter,
			vmopv1.VolumeSharingModeMultiWriter,
			false,
		),
		Entry("empty sharing mode defaults to None",
			vimtypes.VirtualDiskSharing(""),
			vmopv1.VolumeSharingModeNone,
			false,
		),
		Entry("unknown sharing mode is unsupported",
			vimtypes.VirtualDiskSharing("unknown_sharing"),
			vmopv1.VolumeSharingMode(""),
			true,
		),
	)
})
