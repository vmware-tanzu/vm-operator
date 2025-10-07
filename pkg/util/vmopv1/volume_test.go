// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vimtypes "github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/api/v1alpha1"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
)

var _ = Describe("GetVirtualDiskFileNameAndUUID", func() {
	Context("when VirtualDisk is nil", func() {
		It("should return empty strings", func() {
			fileName, diskUUID := vmopv1util.GetVirtualDiskFileNameAndUUID(nil)
			Expect(fileName).To(BeEmpty())
			Expect(diskUUID).To(BeEmpty())
		})
	})

	Context("when VirtualDisk has different backing types", func() {
		It("should extract fileName and diskUUID from SeSparse backing", func() {
			vd := &vimtypes.VirtualDisk{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
						VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
							FileName: "[datastore1] vm/disk.vmdk",
						},
						Uuid: "test-uuid-123",
					},
				},
			}
			fileName, diskUUID := vmopv1util.GetVirtualDiskFileNameAndUUID(vd)
			Expect(fileName).To(Equal("[datastore1] vm/disk.vmdk"))
			Expect(diskUUID).To(Equal("test-uuid-123"))
		})

		It("should extract fileName from SparseVer1 backing", func() {
			vd := &vimtypes.VirtualDisk{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualDiskSparseVer1BackingInfo{
						VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
							FileName: "[datastore1] vm/sparse.vmdk",
						},
					},
				},
			}
			fileName, diskUUID := vmopv1util.GetVirtualDiskFileNameAndUUID(vd)
			Expect(fileName).To(Equal("[datastore1] vm/sparse.vmdk"))
			Expect(diskUUID).To(BeEmpty())
		})

		It("should extract fileName and diskUUID from SparseVer2 backing", func() {
			vd := &vimtypes.VirtualDisk{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualDiskSparseVer2BackingInfo{
						VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
							FileName: "[datastore1] vm/sparse2.vmdk",
						},
						Uuid: "sparse2-uuid",
					},
				},
			}
			fileName, diskUUID := vmopv1util.GetVirtualDiskFileNameAndUUID(vd)
			Expect(fileName).To(Equal("[datastore1] vm/sparse2.vmdk"))
			Expect(diskUUID).To(Equal("sparse2-uuid"))
		})

		It("should extract fileName from FlatVer1 backing", func() {
			vd := &vimtypes.VirtualDisk{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualDiskFlatVer1BackingInfo{
						VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
							FileName: "[datastore1] vm/flat.vmdk",
						},
					},
				},
			}
			fileName, diskUUID := vmopv1util.GetVirtualDiskFileNameAndUUID(vd)
			Expect(fileName).To(Equal("[datastore1] vm/flat.vmdk"))
			Expect(diskUUID).To(BeEmpty())
		})

		It("should extract fileName and diskUUID from FlatVer2 backing", func() {
			vd := &vimtypes.VirtualDisk{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
						VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
							FileName: "[datastore1] vm/flat2.vmdk",
						},
						Uuid: "flat2-uuid",
					},
				},
			}
			fileName, diskUUID := vmopv1util.GetVirtualDiskFileNameAndUUID(vd)
			Expect(fileName).To(Equal("[datastore1] vm/flat2.vmdk"))
			Expect(diskUUID).To(Equal("flat2-uuid"))
		})

		It("should extract fileName and diskUUID from LocalPMem backing", func() {
			vd := &vimtypes.VirtualDisk{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualDiskLocalPMemBackingInfo{
						VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
							FileName: "[datastore1] vm/pmem.vmdk",
						},
						Uuid: "pmem-uuid",
					},
				},
			}
			fileName, diskUUID := vmopv1util.GetVirtualDiskFileNameAndUUID(vd)
			Expect(fileName).To(Equal("[datastore1] vm/pmem.vmdk"))
			Expect(diskUUID).To(Equal("pmem-uuid"))
		})

		It("should extract fileName and diskUUID from RawDiskVer2 backing", func() {
			vd := &vimtypes.VirtualDisk{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualDiskRawDiskVer2BackingInfo{
						VirtualDeviceDeviceBackingInfo: vimtypes.VirtualDeviceDeviceBackingInfo{},
						DescriptorFileName:             "[datastore1] vm/raw.vmdk",
						Uuid:                           "raw-uuid",
					},
				},
			}
			fileName, diskUUID := vmopv1util.GetVirtualDiskFileNameAndUUID(vd)
			Expect(fileName).To(Equal("[datastore1] vm/raw.vmdk"))
			Expect(diskUUID).To(Equal("raw-uuid"))
		})

		It("should extract fileName and diskUUID from "+
			"PartitionedRawDiskVer2 backing", func() {
			vd := &vimtypes.VirtualDisk{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualDiskPartitionedRawDiskVer2BackingInfo{
						VirtualDiskRawDiskVer2BackingInfo: vimtypes.VirtualDiskRawDiskVer2BackingInfo{
							VirtualDeviceDeviceBackingInfo: vimtypes.VirtualDeviceDeviceBackingInfo{},
							DescriptorFileName:             "[datastore1] vm/partitioned.vmdk",
							Uuid:                           "partitioned-uuid",
						},
						Partition: []int32{1, 2},
					},
				},
			}
			fileName, diskUUID := vmopv1util.GetVirtualDiskFileNameAndUUID(vd)
			Expect(fileName).To(Equal("[datastore1] vm/partitioned.vmdk"))
			Expect(diskUUID).To(Equal("partitioned-uuid"))
		})

		It("should handle unknown backing type", func() {
			vd := &vimtypes.VirtualDisk{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualDiskRawDiskMappingVer1BackingInfo{
						VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
							FileName: "[datastore1] vm/mapping.vmdk",
						},
						Uuid: "mapping-uuid",
					},
				},
			}
			fileName, diskUUID := vmopv1util.GetVirtualDiskFileNameAndUUID(vd)
			Expect(fileName).To(Equal("[datastore1] vm/mapping.vmdk"))
			Expect(diskUUID).To(Equal("mapping-uuid"))
		})

		It("should handle nil backing", func() {
			vd := &vimtypes.VirtualDisk{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: nil,
				},
			}
			fileName, diskUUID := vmopv1util.GetVirtualDiskFileNameAndUUID(vd)
			Expect(fileName).To(BeEmpty())
			Expect(diskUUID).To(BeEmpty())
		})
	})
})

var _ = Describe("GetVirtualCdromName", func() {
	Context("when VirtualCdrom is nil", func() {
		It("should return empty string", func() {
			name := vmopv1util.GetVirtualCdromName(nil)
			Expect(name).To(BeEmpty())
		})
	})

	Context("when VirtualCdrom has different backing types", func() {
		It("should return device name for Atapi backing", func() {
			cdrom := &vimtypes.VirtualCdrom{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualCdromAtapiBackingInfo{
						VirtualDeviceDeviceBackingInfo: vimtypes.VirtualDeviceDeviceBackingInfo{
							DeviceName: "ide-cdrom-0",
						},
					},
				},
			}
			name := vmopv1util.GetVirtualCdromName(cdrom)
			Expect(name).To(Equal("ide-cdrom-0"))
		})

		It("should return device name for Passthrough backing", func() {
			cdrom := &vimtypes.VirtualCdrom{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualCdromPassthroughBackingInfo{
						VirtualDeviceDeviceBackingInfo: vimtypes.VirtualDeviceDeviceBackingInfo{
							DeviceName: "ide-cdrom-1",
						},
					},
				},
			}
			name := vmopv1util.GetVirtualCdromName(cdrom)
			Expect(name).To(Equal("ide-cdrom-1"))
		})

		It("should return processed filename for ISO backing", func() {
			cdrom := &vimtypes.VirtualCdrom{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualCdromIsoBackingInfo{
						VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
							FileName: "[datastore1] iso/ubuntu.iso",
						},
					},
				},
			}
			name := vmopv1util.GetVirtualCdromName(cdrom)
			Expect(name).To(Equal("ubuntu"))
		})

		It("should return device name for RemoteAtapi backing", func() {
			cdrom := &vimtypes.VirtualCdrom{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualCdromRemoteAtapiBackingInfo{
						VirtualDeviceRemoteDeviceBackingInfo: vimtypes.VirtualDeviceRemoteDeviceBackingInfo{
							DeviceName: "remote-cdrom-0",
						},
					},
				},
			}
			name := vmopv1util.GetVirtualCdromName(cdrom)
			Expect(name).To(Equal("remote-cdrom-0"))
		})

		It("should return device name for RemotePassthrough backing", func() {
			cdrom := &vimtypes.VirtualCdrom{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualCdromRemotePassthroughBackingInfo{
						VirtualDeviceRemoteDeviceBackingInfo: vimtypes.VirtualDeviceRemoteDeviceBackingInfo{
							DeviceName: "remote-cdrom-1",
						},
					},
				},
			}
			name := vmopv1util.GetVirtualCdromName(cdrom)
			Expect(name).To(Equal("remote-cdrom-1"))
		})

		It("should handle nil backing", func() {
			cdrom := &vimtypes.VirtualCdrom{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: nil,
				},
			}
			name := vmopv1util.GetVirtualCdromName(cdrom)
			Expect(name).To(BeEmpty())
		})
	})
})

var _ = Describe("GetVirtualDiskNameAndUUID", func() {
	It("should return processed name and UUID", func() {
		vd := &vimtypes.VirtualDisk{
			VirtualDevice: vimtypes.VirtualDevice{
				Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
					VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
						FileName: "[datastore1] vm/my-disk.vmdk",
					},
					Uuid: "test-uuid-456",
				},
			},
		}
		name, uuid := vmopv1util.GetVirtualDiskNameAndUUID(vd)
		Expect(name).To(Equal("my-disk"))
		Expect(uuid).To(Equal("test-uuid-456"))
	})
})

var _ = Describe("fileNameToName", func() {
	Context("with valid datastore paths", func() {
		It("should extract name from standard datastore path", func() {
			// This tests the fileNameToName function indirectly through
			// GetVirtualDiskNameAndUUID
			vd := &vimtypes.VirtualDisk{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
						VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
							FileName: "[datastore1] vm/test-disk.vmdk",
						},
						Uuid: "test-uuid",
					},
				},
			}
			name, _ := vmopv1util.GetVirtualDiskNameAndUUID(vd)
			Expect(name).To(Equal("test-disk"))
		})

		It("should handle path with multiple directories", func() {
			vd := &vimtypes.VirtualDisk{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
						VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
							FileName: "[datastore1] vm/folder/subfolder/disk.vmdk",
						},
						Uuid: "test-uuid",
					},
				},
			}
			name, _ := vmopv1util.GetVirtualDiskNameAndUUID(vd)
			Expect(name).To(Equal("disk"))
		})

		It("should handle file without extension", func() {
			vd := &vimtypes.VirtualDisk{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
						VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
							FileName: "[datastore1] vm/disk",
						},
						Uuid: "test-uuid",
					},
				},
			}
			name, _ := vmopv1util.GetVirtualDiskNameAndUUID(vd)
			Expect(name).To(Equal("disk"))
		})

		It("should handle file with multiple extensions", func() {
			vd := &vimtypes.VirtualDisk{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
						VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
							FileName: "[datastore1] vm/disk.backup.vmdk",
						},
						Uuid: "test-uuid",
					},
				},
			}
			name, _ := vmopv1util.GetVirtualDiskNameAndUUID(vd)
			Expect(name).To(Equal("disk.backup"))
		})
	})

	Context("with invalid datastore paths", func() {
		It("should return empty string for invalid path", func() {
			vd := &vimtypes.VirtualDisk{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
						VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
							FileName: "invalid-path-without-brackets",
						},
						Uuid: "test-uuid",
					},
				},
			}
			name, _ := vmopv1util.GetVirtualDiskNameAndUUID(vd)
			Expect(name).To(BeEmpty())
		})

		It("should return empty string for empty path", func() {
			vd := &vimtypes.VirtualDisk{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
						VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
							FileName: "",
						},
						Uuid: "test-uuid",
					},
				},
			}
			name, _ := vmopv1util.GetVirtualDiskNameAndUUID(vd)
			Expect(name).To(BeEmpty())
		})
	})
})

// MockController implements ControllerWithDevices interface for testing.
type MockController struct {
	BusNumber int32
	Devices   []vmopv1.VirtualDeviceStatus
}

func (m MockController) GetDevices() []vmopv1.VirtualDeviceStatus {
	return m.Devices
}

func (m MockController) GetBusNumber() int32 {
	return m.BusNumber
}

var _ = Describe("SortControllerWithDevices", func() {
	Context("with controller map", func() {
		It("should sort controllers by bus number", func() {

			controllerMap := map[int32]*MockController{
				2: {
					BusNumber: 2,
					Devices: []vmopv1.VirtualDeviceStatus{
						{Type: vmopv1.VirtualDeviceTypeDisk, UnitNumber: 1},
						{Type: vmopv1.VirtualDeviceTypeCDROM, UnitNumber: 0},
					},
				},
				0: {
					BusNumber: 0,
					Devices: []vmopv1.VirtualDeviceStatus{
						{Type: vmopv1.VirtualDeviceTypeDisk, UnitNumber: 0},
					},
				},
				1: {
					BusNumber: 1,
					Devices: []vmopv1.VirtualDeviceStatus{
						{Type: vmopv1.VirtualDeviceTypeCDROM, UnitNumber: 0},
						{Type: vmopv1.VirtualDeviceTypeDisk, UnitNumber: 1},
					},
				},
			}

			result := vmopv1util.SortControllerWithDevices(controllerMap)
			Expect(result).To(HaveLen(3))
			Expect(result[0].GetBusNumber()).To(Equal(int32(0)))
			Expect(result[1].GetBusNumber()).To(Equal(int32(1)))
			Expect(result[2].GetBusNumber()).To(Equal(int32(2)))
		})

		It("should sort devices within each controller", func() {

			controllerMap := map[int32]*MockController{
				0: {
					BusNumber: 0,
					Devices: []vmopv1.VirtualDeviceStatus{
						{Type: vmopv1.VirtualDeviceTypeDisk, UnitNumber: 2},
						{Type: vmopv1.VirtualDeviceTypeCDROM, UnitNumber: 0},
						{Type: vmopv1.VirtualDeviceTypeDisk, UnitNumber: 1},
					},
				},
			}

			result := vmopv1util.SortControllerWithDevices(controllerMap)
			Expect(result).To(HaveLen(1))

			// Check that devices are sorted by type first, then unit number
			devices := result[0].GetDevices()
			Expect(devices).To(HaveLen(3))
			Expect(devices[0].Type).To(Equal(vmopv1.VirtualDeviceTypeCDROM))
			Expect(devices[0].UnitNumber).To(Equal(int32(0)))
			Expect(devices[1].Type).To(Equal(vmopv1.VirtualDeviceTypeDisk))
			Expect(devices[1].UnitNumber).To(Equal(int32(1)))
			Expect(devices[2].Type).To(Equal(vmopv1.VirtualDeviceTypeDisk))
			Expect(devices[2].UnitNumber).To(Equal(int32(2)))
		})

		It("should handle empty controller map", func() {

			controllerMap := map[int32]*MockController{}
			result := vmopv1util.SortControllerWithDevices(controllerMap)
			Expect(result).To(BeEmpty())
		})
	})
})

var _ = Describe("GetVolumeValidationInfo", func() {
	var (
		hwStatus *vmopv1.VirtualMachineHardwareStatus
	)

	BeforeEach(func() {
		hwStatus = &vmopv1.VirtualMachineHardwareStatus{
			IDEControllers: []vmopv1.IDEControllerStatus{
				{
					BusNumber: 0,
					DeviceKey: 200,
					Devices: []vmopv1.VirtualDeviceStatus{
						{DiskUUID: "disk-uuid-1", UnitNumber: 0},
						{DiskUUID: "unmanaged-uuid-1", UnitNumber: 1},
					},
				},
			},
		}
	})

	Context("with valid hardware status", func() {
		It("should return correct controller and device info", func() {
			// Convert maps to slices for the new function signature
			existingAttachmentStatus := []cnsv1alpha1.VolumeStatus{
				{
					Name: "volume-1",
					PersistentVolumeClaim: cnsv1alpha1.PersistentVolumeClaimStatus{
						ClaimName: "pvc-1",
						Attached:  true,
						Diskuuid:  "disk-uuid-1",
					},
				},
			}
			volStatus := []vmopv1.VirtualMachineVolumeStatus{
				{
					Type:     vmopv1.VolumeTypeClassic,
					Attached: true,
					DiskUUID: "unmanaged-uuid-1",
				},
			}
			ctrlInfoMap, managedDeviceInfoMap, unmanagedDiskDeviceInfoSet, err :=
				vmopv1util.GetVolumeValidationInfo(
					existingAttachmentStatus, volStatus, hwStatus)

			Expect(err).ToNot(HaveOccurred())
			Expect(ctrlInfoMap).To(HaveLen(1))
			Expect(managedDeviceInfoMap).To(HaveLen(1))
			Expect(unmanagedDiskDeviceInfoSet).To(HaveLen(1))

			// Check controller info
			ctrlKey := vmopv1util.CtrlKey{
				CtrlType:  vmopv1.VirtualControllerTypeIDE,
				BusNumber: 0,
			}
			Expect(ctrlInfoMap[ctrlKey].ControllerKey).To(Equal(int32(200)))

			// Check managed device info
			volumeKey := vmopv1util.VolumeKey{VolumeName: "volume-1", PVCName: "pvc-1"}
			Expect(managedDeviceInfoMap[volumeKey].UnitNumber).To(Equal(int32(0)))
		})
	})

	Context("when hardware is out of sync", func() {
		It("should return error", func() {
			// Use a disk UUID that doesn't exist in hardware
			existingAttachmentStatus := []cnsv1alpha1.VolumeStatus{
				{
					Name: "volume-1",
					PersistentVolumeClaim: cnsv1alpha1.PersistentVolumeClaimStatus{
						ClaimName: "pvc-1",
						Attached:  true,
						Diskuuid:  "non-existent-uuid",
					},
				},
			}
			volStatus := []vmopv1.VirtualMachineVolumeStatus{}

			_, _, _, err := vmopv1util.GetVolumeValidationInfo(
				existingAttachmentStatus, volStatus, hwStatus)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("vm.status.hardware is out of sync"))
		})
	})

	Context("with multiple controller types", func() {
		It("should process all controller types", func() {
			hwStatus = &vmopv1.VirtualMachineHardwareStatus{
				IDEControllers: []vmopv1.IDEControllerStatus{
					{
						BusNumber: 0,
						DeviceKey: 200,
						Devices: []vmopv1.VirtualDeviceStatus{
							{DiskUUID: "ide-disk-1", UnitNumber: 0},
						},
					},
				},
				NVMEControllers: []vmopv1.NVMEControllerStatus{
					{
						BusNumber:   1,
						DeviceKey:   201,
						SharingMode: vmopv1.VirtualControllerSharingModePhysical,
						Devices: []vmopv1.VirtualDeviceStatus{
							{DiskUUID: "nvme-disk-1", UnitNumber: 0},
						},
					},
				},
				SATAControllers: []vmopv1.SATAControllerStatus{
					{
						BusNumber: 2,
						DeviceKey: 202,
						Devices: []vmopv1.VirtualDeviceStatus{
							{DiskUUID: "sata-disk-1", UnitNumber: 0},
						},
					},
				},
				SCSIControllers: []vmopv1.SCSIControllerStatus{
					{
						BusNumber:   3,
						DeviceKey:   203,
						SharingMode: vmopv1.VirtualControllerSharingModePhysical,
						Type:        vmopv1.SCSIControllerTypeLsiLogic,
						Devices: []vmopv1.VirtualDeviceStatus{
							{DiskUUID: "scsi-disk-1", UnitNumber: 0},
						},
					},
				},
			}

			existingAttachmentStatus := []cnsv1alpha1.VolumeStatus{
				{
					Name: "volume-1",
					PersistentVolumeClaim: cnsv1alpha1.PersistentVolumeClaimStatus{
						ClaimName: "pvc-1",
						Attached:  true,
						Diskuuid:  "ide-disk-1",
					},
				},
			}
			volStatus := []vmopv1.VirtualMachineVolumeStatus{
				{
					Type:     vmopv1.VolumeTypeClassic,
					Attached: true,
					DiskUUID: "nvme-disk-1",
				},
				{
					Type:     vmopv1.VolumeTypeClassic,
					Attached: true,
					DiskUUID: "sata-disk-1",
				},
				{
					Type:     vmopv1.VolumeTypeClassic,
					Attached: true,
					DiskUUID: "scsi-disk-1",
				},
			}

			ctrlInfoMap, managedDeviceInfoMap, unmanagedDiskDeviceInfoSet, err := vmopv1util.GetVolumeValidationInfo(
				existingAttachmentStatus, volStatus, hwStatus)

			Expect(err).ToNot(HaveOccurred())
			Expect(ctrlInfoMap).To(HaveLen(4))
			Expect(managedDeviceInfoMap).To(HaveLen(1))
			Expect(unmanagedDiskDeviceInfoSet).To(HaveLen(3))

			// Check that all controller types are processed
			Expect(ctrlInfoMap[vmopv1util.CtrlKey{
				CtrlType: vmopv1.VirtualControllerTypeIDE, BusNumber: 0}].ControllerKey).To(Equal(int32(200)))
			Expect(ctrlInfoMap[vmopv1util.CtrlKey{
				CtrlType: vmopv1.VirtualControllerTypeNVME, BusNumber: 1}].ControllerKey).To(Equal(int32(201)))
			Expect(ctrlInfoMap[vmopv1util.CtrlKey{
				CtrlType: vmopv1.VirtualControllerTypeSATA, BusNumber: 2}].ControllerKey).To(Equal(int32(202)))
			Expect(ctrlInfoMap[vmopv1util.CtrlKey{
				CtrlType: vmopv1.VirtualControllerTypeSCSI, BusNumber: 3}].ControllerKey).To(Equal(int32(203)))
		})
	})

	Context("with devices that have empty DiskUUID", func() {
		It("should skip devices with empty DiskUUID", func() {
			hwStatus = &vmopv1.VirtualMachineHardwareStatus{
				IDEControllers: []vmopv1.IDEControllerStatus{
					{
						BusNumber: 0,
						DeviceKey: 200,
						Devices: []vmopv1.VirtualDeviceStatus{
							{DiskUUID: "valid-disk", UnitNumber: 0},
							{DiskUUID: "", UnitNumber: 1}, // Empty DiskUUID
						},
					},
				},
			}

			existingAttachmentStatus := []cnsv1alpha1.VolumeStatus{
				{
					Name: "volume-1",
					PersistentVolumeClaim: cnsv1alpha1.PersistentVolumeClaimStatus{
						ClaimName: "pvc-1",
						Attached:  true,
						Diskuuid:  "valid-disk",
					},
				},
			}
			volStatus := []vmopv1.VirtualMachineVolumeStatus{}

			ctrlInfoMap, managedDeviceInfoMap, unmanagedDiskDeviceInfoSet, err := vmopv1util.GetVolumeValidationInfo(
				existingAttachmentStatus, volStatus, hwStatus)

			Expect(err).ToNot(HaveOccurred())
			Expect(ctrlInfoMap).To(HaveLen(1))
			Expect(managedDeviceInfoMap).To(HaveLen(1))
			Expect(unmanagedDiskDeviceInfoSet).To(BeEmpty())
		})
	})
})

var _ = Describe("ValidateVMSpecVolumesApplicationType", func() {
	var (
		vmSpecVolumes []vmopv1.VirtualMachineVolume
		ctrlInfoMap   map[vmopv1util.CtrlKey]vmopv1util.CtrlInfo
	)

	BeforeEach(func() {
		vmSpecVolumes = []vmopv1.VirtualMachineVolume{
			{
				Name: "volume-1",
				VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
					PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
						PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: "pvc-1",
						},
						ControllerType:      vmopv1.VirtualControllerTypeSCSI,
						ControllerBusNumber: func() *int32 { v := int32(0); return &v }(),
						ApplicationType:     vmopv1.VolumeApplicationTypeOracleRAC,
						DiskMode:            vmopv1.VolumeDiskModeIndependentPersistent,
						SharingMode:         vmopv1.VolumeSharingModeMultiWriter,
					},
				},
			},
		}

		ctrlInfoMap = map[vmopv1util.CtrlKey]vmopv1util.CtrlInfo{
			{CtrlType: vmopv1.VirtualControllerTypeSCSI, BusNumber: 0}: {
				ControllerKey: 200,
				SharingMode:   vmopv1.VirtualControllerSharingModeNone,
			},
		}
	})

	Context("with valid OracleRAC configuration", func() {
		It("should return no error", func() {
			err := vmopv1util.ValidateVMSpecVolumesApplicationType(
				vmSpecVolumes, ctrlInfoMap)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("with non-SCSI controller for OracleRAC", func() {
		It("should return error", func() {
			vmSpecVolumes[0].VirtualMachineVolumeSource.PersistentVolumeClaim.ControllerType =
				vmopv1.VirtualControllerTypeIDE

			err := vmopv1util.ValidateVMSpecVolumesApplicationType(
				vmSpecVolumes, ctrlInfoMap)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("controller must be SCSI"))
		})
	})

	Context("with wrong disk mode for OracleRAC", func() {
		It("should return error", func() {
			vmSpecVolumes[0].VirtualMachineVolumeSource.PersistentVolumeClaim.DiskMode =
				vmopv1.VolumeDiskModePersistent

			err := vmopv1util.ValidateVMSpecVolumesApplicationType(
				vmSpecVolumes, ctrlInfoMap)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(
				"diskMode must be IndependentPersistent"))
		})
	})

	Context("with wrong PVC sharing mode for OracleRAC", func() {
		It("should return error", func() {
			vmSpecVolumes[0].VirtualMachineVolumeSource.PersistentVolumeClaim.SharingMode = vmopv1.VolumeSharingModeNone

			err := vmopv1util.ValidateVMSpecVolumesApplicationType(
				vmSpecVolumes, ctrlInfoMap)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(
				"PVC sharingMode must be MultiWriter"))
		})
	})

	Context("with wrong controller sharing mode for OracleRAC", func() {
		It("should return error", func() {
			ctrlInfo := ctrlInfoMap[vmopv1util.CtrlKey{
				CtrlType: vmopv1.VirtualControllerTypeSCSI, BusNumber: 0}]
			ctrlInfo.SharingMode = vmopv1.VirtualControllerSharingModePhysical
			ctrlInfoMap[vmopv1util.CtrlKey{
				CtrlType: vmopv1.VirtualControllerTypeSCSI, BusNumber: 0}] = ctrlInfo

			err := vmopv1util.ValidateVMSpecVolumesApplicationType(
				vmSpecVolumes, ctrlInfoMap)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(
				"controller sharingMode must be None"))
		})
	})

	Context("with MicrosoftWSFC configuration", func() {
		BeforeEach(func() {
			vmSpecVolumes[0].VirtualMachineVolumeSource.PersistentVolumeClaim.ApplicationType =
				vmopv1.VolumeApplicationTypeMicrosoftWSFC
			vmSpecVolumes[0].VirtualMachineVolumeSource.PersistentVolumeClaim.SharingMode =
				vmopv1.VolumeSharingModeNone
			ctrlInfo := ctrlInfoMap[vmopv1util.CtrlKey{
				CtrlType: vmopv1.VirtualControllerTypeSCSI, BusNumber: 0}]
			ctrlInfo.SharingMode = vmopv1.VirtualControllerSharingModePhysical
			ctrlInfoMap[vmopv1util.CtrlKey{
				CtrlType: vmopv1.VirtualControllerTypeSCSI, BusNumber: 0}] = ctrlInfo
		})

		It("should return no error with valid configuration", func() {
			err := vmopv1util.ValidateVMSpecVolumesApplicationType(
				vmSpecVolumes, ctrlInfoMap)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return error with wrong PVC sharing mode", func() {
			vmSpecVolumes[0].VirtualMachineVolumeSource.PersistentVolumeClaim.SharingMode =
				vmopv1.VolumeSharingModeMultiWriter

			err := vmopv1util.ValidateVMSpecVolumesApplicationType(
				vmSpecVolumes, ctrlInfoMap)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(
				"PVC sharingMode must be None"))
		})

		It("should return error with wrong controller sharing mode", func() {
			ctrlInfo := ctrlInfoMap[vmopv1util.CtrlKey{
				CtrlType: vmopv1.VirtualControllerTypeSCSI, BusNumber: 0}]
			ctrlInfo.SharingMode = vmopv1.VirtualControllerSharingModeNone
			ctrlInfoMap[vmopv1util.CtrlKey{
				CtrlType: vmopv1.VirtualControllerTypeSCSI, BusNumber: 0}] = ctrlInfo

			err := vmopv1util.ValidateVMSpecVolumesApplicationType(
				vmSpecVolumes, ctrlInfoMap)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(
				"controller sharingMode must be Physical"))
		})
	})

	Context("when application type is not specified", func() {
		It("should skip validation", func() {
			vmSpecVolumes[0].VirtualMachineVolumeSource.PersistentVolumeClaim.ApplicationType =
				""

			err := vmopv1util.ValidateVMSpecVolumesApplicationType(
				vmSpecVolumes, ctrlInfoMap)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("when controller information is not provided", func() {
		It("should return error for non-SCSI controller", func() {
			vmSpecVolumes[0].VirtualMachineVolumeSource.PersistentVolumeClaim.ControllerType =
				""

			err := vmopv1util.ValidateVMSpecVolumesApplicationType(
				vmSpecVolumes, ctrlInfoMap)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("controller must be SCSI"))
		})

		It("should return error for wrong disk mode", func() {
			vmSpecVolumes[0].VirtualMachineVolumeSource.PersistentVolumeClaim.ControllerType =
				vmopv1.VirtualControllerTypeSCSI
			vmSpecVolumes[0].VirtualMachineVolumeSource.PersistentVolumeClaim.DiskMode =
				vmopv1.VolumeDiskModePersistent

			err := vmopv1util.ValidateVMSpecVolumesApplicationType(
				vmSpecVolumes, ctrlInfoMap)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(
				"diskMode must be IndependentPersistent"))
		})
	})

	Context("with mixed volumes", func() {
		It("should validate only volumes with application type", func() {
			vmSpecVolumes = append(vmSpecVolumes, vmopv1.VirtualMachineVolume{
				Name: "volume-2",
				VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
					PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
						PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: "pvc-2",
						},
						// No application type - should be skipped
					},
				},
			})

			err := vmopv1util.ValidateVMSpecVolumesApplicationType(
				vmSpecVolumes, ctrlInfoMap)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})

var _ = Describe("validateUnitNumberRange", func() {
	Context("with negative unit numbers", func() {
		It("should return error for negative unit number", func() {
			err := vmopv1util.ValidateVMSpecVolumesUnitNumber(
				[]vmopv1.VirtualMachineVolume{
					{
						Name: "volume-1",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "pvc-1",
								},
								ControllerType:      vmopv1.VirtualControllerTypeIDE,
								ControllerBusNumber: func() *int32 { v := int32(0); return &v }(),
								UnitNumber:          func() *int32 { v := int32(-1); return &v }(),
							},
						},
					},
				},
				map[vmopv1util.VolumeKey]vmopv1util.DeviceInfo{},
				sets.New[vmopv1util.DeviceInfo](),
				map[vmopv1util.CtrlKey]vmopv1util.CtrlInfo{
					{CtrlType: vmopv1.VirtualControllerTypeIDE, BusNumber: 0}: {
						ControllerKey: 200,
					},
				})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unit number cannot be negative"))
		})
	})

	Context("with NVME controller", func() {
		It("should return error for unit number too high", func() {
			err := vmopv1util.ValidateVMSpecVolumesUnitNumber(
				[]vmopv1.VirtualMachineVolume{
					{
						Name: "volume-1",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "pvc-1",
								},
								ControllerType:      vmopv1.VirtualControllerTypeNVME,
								ControllerBusNumber: func() *int32 { v := int32(0); return &v }(),
								UnitNumber:          func() *int32 { v := int32(64); return &v }(),
							},
						},
					},
				},
				map[vmopv1util.VolumeKey]vmopv1util.DeviceInfo{},
				sets.New[vmopv1util.DeviceInfo](),
				map[vmopv1util.CtrlKey]vmopv1util.CtrlInfo{
					{CtrlType: vmopv1.VirtualControllerTypeNVME, BusNumber: 0}: {
						ControllerKey: 200,
					},
				})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(
				"NVME controller unit number cannot be greater than 63"))
		})

		It("should accept valid NVME unit number", func() {
			err := vmopv1util.ValidateVMSpecVolumesUnitNumber(
				[]vmopv1.VirtualMachineVolume{
					{
						Name: "volume-1",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "pvc-1",
								},
								ControllerType:      vmopv1.VirtualControllerTypeNVME,
								ControllerBusNumber: func() *int32 { v := int32(0); return &v }(),
								UnitNumber:          func() *int32 { v := int32(32); return &v }(),
							},
						},
					},
				},
				map[vmopv1util.VolumeKey]vmopv1util.DeviceInfo{},
				sets.New[vmopv1util.DeviceInfo](),
				map[vmopv1util.CtrlKey]vmopv1util.CtrlInfo{
					{CtrlType: vmopv1.VirtualControllerTypeNVME, BusNumber: 0}: {
						ControllerKey: 200,
					},
				})
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("with SATA controller", func() {
		It("should return error for unit number too high", func() {
			err := vmopv1util.ValidateVMSpecVolumesUnitNumber(
				[]vmopv1.VirtualMachineVolume{
					{
						Name: "volume-1",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "pvc-1",
								},
								ControllerType:      vmopv1.VirtualControllerTypeSATA,
								ControllerBusNumber: func() *int32 { v := int32(0); return &v }(),
								UnitNumber:          func() *int32 { v := int32(30); return &v }(),
							},
						},
					},
				},
				map[vmopv1util.VolumeKey]vmopv1util.DeviceInfo{},
				sets.New[vmopv1util.DeviceInfo](),
				map[vmopv1util.CtrlKey]vmopv1util.CtrlInfo{
					{CtrlType: vmopv1.VirtualControllerTypeSATA, BusNumber: 0}: {
						ControllerKey: 200,
					},
				})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("SATA controller unit number cannot be greater than 29"))
		})

		It("should accept valid SATA unit number", func() {
			err := vmopv1util.ValidateVMSpecVolumesUnitNumber(
				[]vmopv1.VirtualMachineVolume{
					{
						Name: "volume-1",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "pvc-1",
								},
								ControllerType:      vmopv1.VirtualControllerTypeSATA,
								ControllerBusNumber: func() *int32 { v := int32(0); return &v }(),
								UnitNumber:          func() *int32 { v := int32(15); return &v }(),
							},
						},
					},
				},
				map[vmopv1util.VolumeKey]vmopv1util.DeviceInfo{},
				sets.New[vmopv1util.DeviceInfo](),
				map[vmopv1util.CtrlKey]vmopv1util.CtrlInfo{
					{CtrlType: vmopv1.VirtualControllerTypeSATA, BusNumber: 0}: {
						ControllerKey: 200,
					},
				})
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("with SCSI ParaVirtual controller", func() {
		It("should return error for unit number too high", func() {
			err := vmopv1util.ValidateVMSpecVolumesUnitNumber(
				[]vmopv1.VirtualMachineVolume{
					{
						Name: "volume-1",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "pvc-1",
								},
								ControllerType:      vmopv1.VirtualControllerTypeSCSI,
								ControllerBusNumber: func() *int32 { v := int32(0); return &v }(),
								UnitNumber:          func() *int32 { v := int32(64); return &v }(),
							},
						},
					},
				},
				map[vmopv1util.VolumeKey]vmopv1util.DeviceInfo{},
				sets.New[vmopv1util.DeviceInfo](),
				map[vmopv1util.CtrlKey]vmopv1util.CtrlInfo{
					{CtrlType: vmopv1.VirtualControllerTypeSCSI, BusNumber: 0}: {
						ControllerKey: 200,
						SCSIType:      vmopv1.SCSIControllerTypeParaVirtualSCSI,
					},
				})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("SCSI ParaVirtual controller unit number cannot be greater than 63"))
		})

		It("should accept valid ParaVirtual unit number", func() {
			err := vmopv1util.ValidateVMSpecVolumesUnitNumber(
				[]vmopv1.VirtualMachineVolume{
					{
						Name: "volume-1",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "pvc-1",
								},
								ControllerType:      vmopv1.VirtualControllerTypeSCSI,
								ControllerBusNumber: func() *int32 { v := int32(0); return &v }(),
								UnitNumber:          func() *int32 { v := int32(32); return &v }(),
							},
						},
					},
				},
				map[vmopv1util.VolumeKey]vmopv1util.DeviceInfo{},
				sets.New[vmopv1util.DeviceInfo](),
				map[vmopv1util.CtrlKey]vmopv1util.CtrlInfo{
					{CtrlType: vmopv1.VirtualControllerTypeSCSI, BusNumber: 0}: {
						ControllerKey: 200,
						SCSIType:      vmopv1.SCSIControllerTypeParaVirtualSCSI,
					},
				})
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("with SCSI LsiLogic controller", func() {
		It("should return error for unit number too high", func() {
			err := vmopv1util.ValidateVMSpecVolumesUnitNumber(
				[]vmopv1.VirtualMachineVolume{
					{
						Name: "volume-1",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "pvc-1",
								},
								ControllerType:      vmopv1.VirtualControllerTypeSCSI,
								ControllerBusNumber: func() *int32 { v := int32(0); return &v }(),
								UnitNumber:          func() *int32 { v := int32(16); return &v }(),
							},
						},
					},
				},
				map[vmopv1util.VolumeKey]vmopv1util.DeviceInfo{},
				sets.New[vmopv1util.DeviceInfo](),
				map[vmopv1util.CtrlKey]vmopv1util.CtrlInfo{
					{CtrlType: vmopv1.VirtualControllerTypeSCSI, BusNumber: 0}: {
						ControllerKey: 200,
						SCSIType:      vmopv1.SCSIControllerTypeLsiLogic,
					},
				})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("SCSI LsiLogic controller unit number cannot be greater than 15"))
		})

		It("should accept valid LsiLogic unit number", func() {
			err := vmopv1util.ValidateVMSpecVolumesUnitNumber(
				[]vmopv1.VirtualMachineVolume{
					{
						Name: "volume-1",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "pvc-1",
								},
								ControllerType:      vmopv1.VirtualControllerTypeSCSI,
								ControllerBusNumber: func() *int32 { v := int32(0); return &v }(),
								UnitNumber:          func() *int32 { v := int32(8); return &v }(),
							},
						},
					},
				},
				map[vmopv1util.VolumeKey]vmopv1util.DeviceInfo{},
				sets.New[vmopv1util.DeviceInfo](),
				map[vmopv1util.CtrlKey]vmopv1util.CtrlInfo{
					{CtrlType: vmopv1.VirtualControllerTypeSCSI, BusNumber: 0}: {
						ControllerKey: 200,
						SCSIType:      vmopv1.SCSIControllerTypeLsiLogic,
					},
				})
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("with unknown controller type", func() {
		It("should accept any unit number", func() {
			err := vmopv1util.ValidateVMSpecVolumesUnitNumber(
				[]vmopv1.VirtualMachineVolume{
					{
						Name: "volume-1",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "pvc-1",
								},
								ControllerType:      vmopv1.VirtualControllerType("Unknown"),
								ControllerBusNumber: func() *int32 { v := int32(0); return &v }(),
								UnitNumber:          func() *int32 { v := int32(100); return &v }(),
							},
						},
					},
				},
				map[vmopv1util.VolumeKey]vmopv1util.DeviceInfo{},
				sets.New[vmopv1util.DeviceInfo](),
				map[vmopv1util.CtrlKey]vmopv1util.CtrlInfo{
					{CtrlType: vmopv1.VirtualControllerType("Unknown"), BusNumber: 0}: {
						ControllerKey: 200,
					},
				})
			Expect(err).ToNot(HaveOccurred())
		})
	})

})

var _ = Describe("ValidateVMSpecVolumesUnitNumber", func() {
	var (
		vmSpecVolumes              []vmopv1.VirtualMachineVolume
		managedDeviceInfoMap       map[vmopv1util.VolumeKey]vmopv1util.DeviceInfo
		unmanagedDiskDeviceInfoSet sets.Set[vmopv1util.DeviceInfo]
		ctrlInfoMap                map[vmopv1util.CtrlKey]vmopv1util.CtrlInfo
	)

	BeforeEach(func() {
		vmSpecVolumes = []vmopv1.VirtualMachineVolume{
			{
				Name: "volume-1",
				VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
					PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
						PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: "pvc-1",
						},
						ControllerType:      vmopv1.VirtualControllerTypeIDE,
						ControllerBusNumber: func() *int32 { v := int32(0); return &v }(),
						UnitNumber:          func() *int32 { v := int32(0); return &v }(),
					},
				},
			},
		}

		managedDeviceInfoMap = map[vmopv1util.VolumeKey]vmopv1util.DeviceInfo{
			{VolumeName: "volume-1", PVCName: "pvc-1"}: {
				CtrlKey:    vmopv1util.CtrlKey{CtrlType: vmopv1.VirtualControllerTypeIDE, BusNumber: 0},
				UnitNumber: 0,
			},
		}

		unmanagedDiskDeviceInfoSet = sets.New[vmopv1util.DeviceInfo]()
		ctrlInfoMap = map[vmopv1util.CtrlKey]vmopv1util.CtrlInfo{
			{CtrlType: vmopv1.VirtualControllerTypeIDE, BusNumber: 0}: {
				ControllerKey: 200,
			},
		}
	})

	Context("with valid unit numbers", func() {
		It("should return no error", func() {
			err := vmopv1util.ValidateVMSpecVolumesUnitNumber(
				vmSpecVolumes, managedDeviceInfoMap, unmanagedDiskDeviceInfoSet, ctrlInfoMap)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("with negative unit number", func() {
		It("should return error", func() {
			vmSpecVolumes[0].VirtualMachineVolumeSource.PersistentVolumeClaim.UnitNumber = func() *int32 { v := int32(-1); return &v }()

			err := vmopv1util.ValidateVMSpecVolumesUnitNumber(
				vmSpecVolumes, managedDeviceInfoMap, unmanagedDiskDeviceInfoSet, ctrlInfoMap)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unit number cannot be negative"))
		})
	})

	Context("with unit number too high for IDE controller", func() {
		It("should return error", func() {
			vmSpecVolumes[0].VirtualMachineVolumeSource.PersistentVolumeClaim.UnitNumber = func() *int32 { v := int32(2); return &v }()

			err := vmopv1util.ValidateVMSpecVolumesUnitNumber(
				vmSpecVolumes, managedDeviceInfoMap, unmanagedDiskDeviceInfoSet, ctrlInfoMap)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("IDE controller unit number cannot be greater than 1"))
		})
	})

	Context("with SCSI reserved unit number", func() {
		BeforeEach(func() {
			vmSpecVolumes[0].VirtualMachineVolumeSource.PersistentVolumeClaim.ControllerType = vmopv1.VirtualControllerTypeSCSI
			vmSpecVolumes[0].VirtualMachineVolumeSource.PersistentVolumeClaim.UnitNumber = func() *int32 { v := int32(7); return &v }()
			ctrlInfoMap[vmopv1util.CtrlKey{CtrlType: vmopv1.VirtualControllerTypeSCSI, BusNumber: 0}] = vmopv1util.CtrlInfo{
				ControllerKey: 200,
				SCSIType:      vmopv1.SCSIControllerTypeLsiLogic,
			}
		})

		It("should return error", func() {
			err := vmopv1util.ValidateVMSpecVolumesUnitNumber(
				vmSpecVolumes, managedDeviceInfoMap, unmanagedDiskDeviceInfoSet, ctrlInfoMap)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("SCSI controller unit number 7 is reserved"))
		})
	})

	Context("with duplicated unit number", func() {
		It("should return error", func() {
			// Add an existing device with the same unit number
			unmanagedDiskDeviceInfoSet.Insert(vmopv1util.DeviceInfo{
				CtrlKey:    vmopv1util.CtrlKey{CtrlType: vmopv1.VirtualControllerTypeIDE, BusNumber: 0},
				UnitNumber: 0,
			})

			err := vmopv1util.ValidateVMSpecVolumesUnitNumber(
				vmSpecVolumes, managedDeviceInfoMap, unmanagedDiskDeviceInfoSet, ctrlInfoMap)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("volume \"volume-1\" contains a duplicated unit number"))
		})
	})

	Context("when controller is not found", func() {
		It("should return error", func() {
			vmSpecVolumes[0].VirtualMachineVolumeSource.PersistentVolumeClaim.ControllerType = vmopv1.VirtualControllerTypeSCSI
			vmSpecVolumes[0].VirtualMachineVolumeSource.PersistentVolumeClaim.ControllerBusNumber = func() *int32 { v := int32(1); return &v }()

			err := vmopv1util.ValidateVMSpecVolumesUnitNumber(
				vmSpecVolumes, managedDeviceInfoMap, unmanagedDiskDeviceInfoSet, ctrlInfoMap)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unable to find a controller"))
		})
	})

	Context("when no controller info is provided", func() {
		It("should use existing device info", func() {
			vmSpecVolumes[0].VirtualMachineVolumeSource.PersistentVolumeClaim.ControllerType = ""
			vmSpecVolumes[0].VirtualMachineVolumeSource.PersistentVolumeClaim.ControllerBusNumber = nil
			vmSpecVolumes[0].VirtualMachineVolumeSource.PersistentVolumeClaim.UnitNumber = nil

			err := vmopv1util.ValidateVMSpecVolumesUnitNumber(
				vmSpecVolumes, managedDeviceInfoMap, unmanagedDiskDeviceInfoSet, ctrlInfoMap)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
