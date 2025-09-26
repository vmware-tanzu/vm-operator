// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
)

var _ = Describe("GetVirtualDiskFileNameAndUUID", func() {
	var vd *vimtypes.VirtualDisk

	BeforeEach(func() {
		vd = &vimtypes.VirtualDisk{}
	})

	Context("VirtualDiskSeSparseBackingInfo", func() {
		It("should return fileName and diskUUID", func() {
			vd.Backing = &vimtypes.VirtualDiskSeSparseBackingInfo{
				VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
					FileName: "[datastore1] vm/disk.vmdk",
				},
				Uuid: "test-uuid-123",
			}

			fileName, diskUUID := vmopv1util.GetVirtualDiskFileNameAndUUID(vd)
			Expect(fileName).To(Equal("[datastore1] vm/disk.vmdk"))
			Expect(diskUUID).To(Equal("test-uuid-123"))
		})
	})

	Context("VirtualDiskSparseVer1BackingInfo", func() {
		It("should return fileName but no diskUUID", func() {
			vd.Backing = &vimtypes.VirtualDiskSparseVer1BackingInfo{
				VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
					FileName: "[datastore1] vm/sparse-v1.vmdk",
				},
			}

			fileName, diskUUID := vmopv1util.GetVirtualDiskFileNameAndUUID(vd)
			Expect(fileName).To(Equal("[datastore1] vm/sparse-v1.vmdk"))
			Expect(diskUUID).To(BeEmpty())
		})
	})

	Context("VirtualDiskSparseVer2BackingInfo", func() {
		It("should return fileName and diskUUID", func() {
			vd.Backing = &vimtypes.VirtualDiskSparseVer2BackingInfo{
				VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
					FileName: "[datastore1] vm/sparse-v2.vmdk",
				},
				Uuid: "sparse-uuid-456",
			}

			fileName, diskUUID := vmopv1util.GetVirtualDiskFileNameAndUUID(vd)
			Expect(fileName).To(Equal("[datastore1] vm/sparse-v2.vmdk"))
			Expect(diskUUID).To(Equal("sparse-uuid-456"))
		})
	})

	Context("VirtualDiskFlatVer1BackingInfo", func() {
		It("should return fileName but no diskUUID", func() {
			vd.Backing = &vimtypes.VirtualDiskFlatVer1BackingInfo{
				VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
					FileName: "[datastore1] vm/flat-v1.vmdk",
				},
			}

			fileName, diskUUID := vmopv1util.GetVirtualDiskFileNameAndUUID(vd)
			Expect(fileName).To(Equal("[datastore1] vm/flat-v1.vmdk"))
			Expect(diskUUID).To(BeEmpty())
		})
	})

	Context("VirtualDiskFlatVer2BackingInfo", func() {
		It("should return fileName and diskUUID", func() {
			vd.Backing = &vimtypes.VirtualDiskFlatVer2BackingInfo{
				VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
					FileName: "[datastore1] vm/flat-v2.vmdk",
				},
				Uuid: "flat-uuid-789",
			}

			fileName, diskUUID := vmopv1util.GetVirtualDiskFileNameAndUUID(vd)
			Expect(fileName).To(Equal("[datastore1] vm/flat-v2.vmdk"))
			Expect(diskUUID).To(Equal("flat-uuid-789"))
		})
	})

	Context("VirtualDiskLocalPMemBackingInfo", func() {
		It("should return fileName and diskUUID", func() {
			vd.Backing = &vimtypes.VirtualDiskLocalPMemBackingInfo{
				VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
					FileName: "[datastore1] vm/pmem.vmdk",
				},
				Uuid: "pmem-uuid-abc",
			}

			fileName, diskUUID := vmopv1util.GetVirtualDiskFileNameAndUUID(vd)
			Expect(fileName).To(Equal("[datastore1] vm/pmem.vmdk"))
			Expect(diskUUID).To(Equal("pmem-uuid-abc"))
		})
	})

	Context("VirtualDiskRawDiskMappingVer1BackingInfo", func() {
		It("should return fileName and diskUUID", func() {
			vd.Backing = &vimtypes.VirtualDiskRawDiskMappingVer1BackingInfo{
				VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
					FileName: "[datastore1] vm/raw-v1.vmdk",
				},
				Uuid: "raw-v1-uuid-def",
			}

			fileName, diskUUID := vmopv1util.GetVirtualDiskFileNameAndUUID(vd)
			Expect(fileName).To(Equal("[datastore1] vm/raw-v1.vmdk"))
			Expect(diskUUID).To(Equal("raw-v1-uuid-def"))
		})
	})

	Context("VirtualDiskRawDiskVer2BackingInfo", func() {
		It("should return descriptorFileName and diskUUID", func() {
			vd.Backing = &vimtypes.VirtualDiskRawDiskVer2BackingInfo{
				DescriptorFileName: "[datastore1] vm/raw-v2-descriptor.vmdk",
				Uuid:               "raw-v2-uuid-ghi",
			}

			fileName, diskUUID := vmopv1util.GetVirtualDiskFileNameAndUUID(vd)
			Expect(fileName).To(Equal("[datastore1] vm/raw-v2-descriptor.vmdk"))
			Expect(diskUUID).To(Equal("raw-v2-uuid-ghi"))
		})
	})

	Context("VirtualDiskPartitionedRawDiskVer2BackingInfo", func() {
		It("should return descriptorFileName and diskUUID", func() {
			vd.Backing = &vimtypes.VirtualDiskPartitionedRawDiskVer2BackingInfo{
				VirtualDiskRawDiskVer2BackingInfo: vimtypes.VirtualDiskRawDiskVer2BackingInfo{
					DescriptorFileName: "[datastore1] vm/partitioned-raw-v2.vmdk",
					Uuid:               "partitioned-uuid-jkl",
				},
			}

			fileName, diskUUID := vmopv1util.GetVirtualDiskFileNameAndUUID(vd)
			Expect(fileName).To(Equal("[datastore1] vm/partitioned-raw-v2.vmdk"))
			Expect(diskUUID).To(Equal("partitioned-uuid-jkl"))
		})
	})

	Context("VirtualDiskSeSparseBackingInfo", func() {
		It("should return fileName and diskUUID", func() {
			vd.Backing = &vimtypes.VirtualDiskSeSparseBackingInfo{
				VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
					FileName: "[datastore1] vm/sesparse-disk.vmdk",
				},
				Uuid: "sesparse-uuid-mno",
			}

			fileName, diskUUID := vmopv1util.GetVirtualDiskFileNameAndUUID(vd)
			Expect(fileName).To(Equal("[datastore1] vm/sesparse-disk.vmdk"))
			Expect(diskUUID).To(Equal("sesparse-uuid-mno"))
		})
	})

	Context("VirtualDiskFlatVer1BackingInfo", func() {
		It("should return fileName but no diskUUID", func() {
			vd.Backing = &vimtypes.VirtualDiskFlatVer1BackingInfo{
				VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
					FileName: "[datastore1] vm/flat-v1-disk.vmdk",
				},
			}

			fileName, diskUUID := vmopv1util.GetVirtualDiskFileNameAndUUID(vd)
			Expect(fileName).To(Equal("[datastore1] vm/flat-v1-disk.vmdk"))
			Expect(diskUUID).To(BeEmpty())
		})
	})

	Context("VirtualDiskSparseVer2BackingInfo", func() {
		It("should return fileName and diskUUID", func() {
			vd.Backing = &vimtypes.VirtualDiskSparseVer2BackingInfo{
				VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
					FileName: "[datastore1] vm/sparse-v2-disk.vmdk",
				},
				Uuid: "sparse-v2-uuid-pqr",
			}

			fileName, diskUUID := vmopv1util.GetVirtualDiskFileNameAndUUID(vd)
			Expect(fileName).To(Equal("[datastore1] vm/sparse-v2-disk.vmdk"))
			Expect(diskUUID).To(Equal("sparse-v2-uuid-pqr"))
		})
	})

	Context("VirtualDiskRawDiskMappingVer1BackingInfo", func() {
		It("should return fileName and diskUUID", func() {
			vd.Backing = &vimtypes.VirtualDiskRawDiskMappingVer1BackingInfo{
				VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
					FileName: "[datastore1] vm/raw-mapping-disk.vmdk",
				},
				Uuid: "raw-mapping-uuid-stu",
			}

			fileName, diskUUID := vmopv1util.GetVirtualDiskFileNameAndUUID(vd)
			Expect(fileName).To(Equal("[datastore1] vm/raw-mapping-disk.vmdk"))
			Expect(diskUUID).To(Equal("raw-mapping-uuid-stu"))
		})
	})

	Context("VirtualDiskLocalPMemBackingInfo", func() {
		It("should return fileName and diskUUID", func() {
			vd.Backing = &vimtypes.VirtualDiskLocalPMemBackingInfo{
				VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
					FileName: "[datastore1] vm/pmem-disk.vmdk",
				},
				Uuid: "pmem-uuid-vwx",
			}

			fileName, diskUUID := vmopv1util.GetVirtualDiskFileNameAndUUID(vd)
			Expect(fileName).To(Equal("[datastore1] vm/pmem-disk.vmdk"))
			Expect(diskUUID).To(Equal("pmem-uuid-vwx"))
		})
	})

	Context("Unsupported backing type", func() {
		It("should return empty strings", func() {
			// Use a different backing type that's not handled
			vd.Backing = &vimtypes.VirtualCdromIsoBackingInfo{
				VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
					FileName: "[datastore1] vm/iso.iso",
				},
			}

			fileName, diskUUID := vmopv1util.GetVirtualDiskFileNameAndUUID(vd)
			Expect(fileName).To(BeEmpty())
			Expect(diskUUID).To(BeEmpty())
		})
	})

	Context("Nil backing", func() {
		It("should return empty strings", func() {
			vd.Backing = nil

			fileName, diskUUID := vmopv1util.GetVirtualDiskFileNameAndUUID(vd)
			Expect(fileName).To(BeEmpty())
			Expect(diskUUID).To(BeEmpty())
		})
	})
})

var _ = Describe("GetVirtualCdromName", func() {
	var vcd *vimtypes.VirtualCdrom

	BeforeEach(func() {
		vcd = &vimtypes.VirtualCdrom{}
	})

	Context("VirtualCdromAtapiBackingInfo", func() {
		It("should return device name", func() {
			vcd.Backing = &vimtypes.VirtualCdromAtapiBackingInfo{
				VirtualDeviceDeviceBackingInfo: vimtypes.VirtualDeviceDeviceBackingInfo{
					DeviceName: "/dev/cdrom0",
				},
			}

			name := vmopv1util.GetVirtualCdromName(vcd)
			Expect(name).To(Equal("/dev/cdrom0"))
		})
	})

	Context("VirtualCdromPassthroughBackingInfo", func() {
		It("should return device name", func() {
			vcd.Backing = &vimtypes.VirtualCdromPassthroughBackingInfo{
				VirtualDeviceDeviceBackingInfo: vimtypes.VirtualDeviceDeviceBackingInfo{
					DeviceName: "/dev/sr0",
				},
			}

			name := vmopv1util.GetVirtualCdromName(vcd)
			Expect(name).To(Equal("/dev/sr0"))
		})
	})

	Context("VirtualCdromIsoBackingInfo", func() {
		It("should return filename without extension", func() {
			vcd.Backing = &vimtypes.VirtualCdromIsoBackingInfo{
				VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
					FileName: "[datastore1] vm/ubuntu-20.04.iso",
				},
			}

			name := vmopv1util.GetVirtualCdromName(vcd)
			Expect(name).To(Equal("ubuntu-20.04"))
		})

		It("should handle complex datastore paths", func() {
			vcd.Backing = &vimtypes.VirtualCdromIsoBackingInfo{
				VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
					FileName: "[datastore1] folder/subfolder/my-iso-file.iso",
				},
			}

			name := vmopv1util.GetVirtualCdromName(vcd)
			Expect(name).To(Equal("my-iso-file"))
		})

		It("should handle files without extension", func() {
			vcd.Backing = &vimtypes.VirtualCdromIsoBackingInfo{
				VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
					FileName: "[datastore1] vm/bootimage",
				},
			}

			name := vmopv1util.GetVirtualCdromName(vcd)
			Expect(name).To(Equal("bootimage"))
		})
	})

	Context("VirtualCdromRemoteAtapiBackingInfo", func() {
		It("should return device name", func() {
			vcd.Backing = &vimtypes.VirtualCdromRemoteAtapiBackingInfo{
				VirtualDeviceRemoteDeviceBackingInfo: vimtypes.VirtualDeviceRemoteDeviceBackingInfo{
					DeviceName: "remote-cdrom",
				},
			}

			name := vmopv1util.GetVirtualCdromName(vcd)
			Expect(name).To(Equal("remote-cdrom"))
		})
	})

	Context("VirtualCdromRemotePassthroughBackingInfo", func() {
		It("should return device name", func() {
			vcd.Backing = &vimtypes.VirtualCdromRemotePassthroughBackingInfo{
				VirtualDeviceRemoteDeviceBackingInfo: vimtypes.VirtualDeviceRemoteDeviceBackingInfo{
					DeviceName: "remote-passthrough-cdrom",
				},
			}

			name := vmopv1util.GetVirtualCdromName(vcd)
			Expect(name).To(Equal("remote-passthrough-cdrom"))
		})
	})

	Context("Unsupported backing type", func() {
		It("should return empty string", func() {
			// Use a different backing type that's not handled
			vcd.Backing = &vimtypes.VirtualDiskFlatVer2BackingInfo{
				VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
					FileName: "[datastore1] vm/disk.vmdk",
				},
			}

			name := vmopv1util.GetVirtualCdromName(vcd)
			Expect(name).To(BeEmpty())
		})
	})

	Context("Nil backing", func() {
		It("should return empty string", func() {
			vcd.Backing = nil

			name := vmopv1util.GetVirtualCdromName(vcd)
			Expect(name).To(BeEmpty())
		})
	})
})

var _ = Describe("GetVirtualDiskName", func() {
	var vd *vimtypes.VirtualDisk

	BeforeEach(func() {
		vd = &vimtypes.VirtualDisk{}
	})

	Context("Valid disk backing", func() {
		It("should return disk name without extension", func() {
			vd.Backing = &vimtypes.VirtualDiskFlatVer2BackingInfo{
				VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
					FileName: "[datastore1] vm/my-data-disk.vmdk",
				},
				Uuid: "test-uuid",
			}

			name := vmopv1util.GetVirtualDiskName(vd)
			Expect(name).To(Equal("my-data-disk"))
		})

		It("should handle complex paths", func() {
			vd.Backing = &vimtypes.VirtualDiskFlatVer2BackingInfo{
				VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
					FileName: "[shared-storage] vms/prod/database/db-disk-001.vmdk",
				},
				Uuid: "complex-path-uuid",
			}

			name := vmopv1util.GetVirtualDiskName(vd)
			Expect(name).To(Equal("db-disk-001"))
		})

		It("should handle files without extension", func() {
			vd.Backing = &vimtypes.VirtualDiskFlatVer2BackingInfo{
				VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
					FileName: "[datastore1] vm/rawdisk",
				},
				Uuid: "no-ext-uuid",
			}

			name := vmopv1util.GetVirtualDiskName(vd)
			Expect(name).To(Equal("rawdisk"))
		})
	})

	Context("Sparse disk backing", func() {
		It("should work with sparse disks", func() {
			vd.Backing = &vimtypes.VirtualDiskSeSparseBackingInfo{
				VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
					FileName: "[datastore1] vm/sparse-disk.vmdk",
				},
				Uuid: "sparse-uuid",
			}

			name := vmopv1util.GetVirtualDiskName(vd)
			Expect(name).To(Equal("sparse-disk"))
		})
	})

	Context("Raw disk backing", func() {
		It("should work with raw disk v2 (uses DescriptorFileName)", func() {
			vd.Backing = &vimtypes.VirtualDiskRawDiskVer2BackingInfo{
				DescriptorFileName: "[datastore1] vm/raw-descriptor.vmdk",
				Uuid:               "raw-uuid",
			}

			name := vmopv1util.GetVirtualDiskName(vd)
			Expect(name).To(Equal("raw-descriptor"))
		})
	})

	Context("Unsupported backing type", func() {
		It("should return empty string", func() {
			vd.Backing = &vimtypes.VirtualCdromIsoBackingInfo{
				VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
					FileName: "[datastore1] vm/iso.iso",
				},
			}

			name := vmopv1util.GetVirtualDiskName(vd)
			Expect(name).To(BeEmpty())
		})
	})

	Context("Invalid datastore path", func() {
		It("should return empty string for malformed paths", func() {
			vd.Backing = &vimtypes.VirtualDiskFlatVer2BackingInfo{
				VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
					FileName: "invalid-datastore-path",
				},
				Uuid: "invalid-path-uuid",
			}

			name := vmopv1util.GetVirtualDiskName(vd)
			Expect(name).To(BeEmpty())
		})
	})

	Context("Nil backing", func() {
		It("should return empty string", func() {
			vd.Backing = nil

			name := vmopv1util.GetVirtualDiskName(vd)
			Expect(name).To(BeEmpty())
		})
	})
})
