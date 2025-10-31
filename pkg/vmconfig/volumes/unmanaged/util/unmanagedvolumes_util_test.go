// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	unmanagedvolsutil "github.com/vmware-tanzu/vm-operator/pkg/vmconfig/volumes/unmanaged/util"
)

var _ = Describe("GetUnmanagedVolumeInfo", func() {
	var (
		vm   *vmopv1.VirtualMachine
		moVM mo.VirtualMachine
		info unmanagedvolsutil.UnmanagedVolumeInfo
	)

	BeforeEach(func() {
		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:   "my-namespace",
				Name:        "my-vm",
				Annotations: map[string]string{},
			},
			Spec: vmopv1.VirtualMachineSpec{
				StorageClass: "my-storage-class-1",
			},
			Status: vmopv1.VirtualMachineStatus{
				UniqueID: "vm-1",
			},
		}
		moVM = mo.VirtualMachine{
			Config:   &vimtypes.VirtualMachineConfigInfo{},
			LayoutEx: &vimtypes.VirtualMachineFileLayoutEx{},
		}

		Expect(vm).ToNot(BeNil())
		Expect(moVM).ToNot(BeNil())
	})

	JustBeforeEach(func() {
		info = unmanagedvolsutil.GetUnmanagedVolumeInfo(vm, moVM)
	})

	When("vm is nil", func() {
		BeforeEach(func() {
			vm = nil
		})
		It("should return an empty UnmanagedVolumeInfo", func() {
			Expect(info.Disks).To(BeEmpty())
			Expect(info.Controllers).To(BeEmpty())
			Expect(info.Volumes).To(BeEmpty())
		})
	})

	When("moVM config is nil", func() {
		BeforeEach(func() {
			moVM.Config = nil
		})
		It("should return an empty UnmanagedVolumeInfo", func() {
			Expect(info.Disks).To(BeEmpty())
			Expect(info.Controllers).To(BeEmpty())
			Expect(info.Volumes).To(BeEmpty())
		})
	})

	When("moVM has no devices", func() {
		BeforeEach(func() {
			moVM.Config.Hardware.Device = nil
		})
		It("should return an empty UnmanagedVolumeInfo", func() {
			Expect(info).To(BeZero())
			Expect(info.Disks).To(BeEmpty())
			Expect(info.Controllers).To(BeEmpty())
			Expect(info.Volumes).To(BeEmpty())
		})
	})

	When("moVM layoutEx is nil", func() {
		BeforeEach(func() {
			moVM.LayoutEx = nil
		})
		It("should return an empty UnmanagedVolumeInfo", func() {
			Expect(info.Disks).To(BeEmpty())
			Expect(info.Controllers).To(BeEmpty())
			Expect(info.Volumes).To(BeEmpty())
		})
	})

	When("moVM has an ide controller but no disks", func() {
		BeforeEach(func() {
			moVM.Config.Hardware.Device = []vimtypes.BaseVirtualDevice{
				&vimtypes.VirtualIDEController{
					VirtualController: vimtypes.VirtualController{
						VirtualDevice: vimtypes.VirtualDevice{
							Key: 100,
						},
						BusNumber: 0,
					},
				},
			}
		})
		It("should return the expected UnmanagedVolumeInfo", func() {
			Expect(info.Volumes).To(BeEmpty())

			Expect(info.Controllers).To(HaveLen(1))
			Expect(info.Controllers[100].Type).To(Equal(vmopv1.VirtualControllerTypeIDE))
			Expect(info.Controllers[100].Key).To(Equal(int32(100)))
			Expect(info.Controllers[100].Bus).To(Equal(int32(0)))

			Expect(info.Disks).To(BeEmpty())
		})
	})

	When("moVM has a SATA controller but no disks", func() {
		BeforeEach(func() {
			moVM.Config.Hardware.Device = []vimtypes.BaseVirtualDevice{
				&vimtypes.VirtualAHCIController{
					VirtualSATAController: vimtypes.VirtualSATAController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: 200,
							},
							BusNumber: 0,
						},
					},
				},
			}
		})
		It("should return the expected UnmanagedVolumeInfo", func() {
			Expect(info.Volumes).To(BeEmpty())

			Expect(info.Controllers).To(HaveLen(1))
			Expect(info.Controllers[200].Type).To(Equal(vmopv1.VirtualControllerTypeSATA))
			Expect(info.Controllers[200].Key).To(Equal(int32(200)))
			Expect(info.Controllers[200].Bus).To(Equal(int32(0)))

			Expect(info.Disks).To(BeEmpty())
		})
	})

	When("moVM has controllers and disks", func() {

		When("vm has no volumes", func() {
			JustBeforeEach(func() {
				Expect(info.Volumes).To(BeEmpty())
			})

			When("there is a disk attached to each possible controller type", func() {

				const diskCount = 7

				BeforeEach(func() {

					moVM.Config = &vimtypes.VirtualMachineConfigInfo{}

					for i := 0; i < diskCount; i++ {
						moVM.Config.Hardware.Device = append(
							moVM.Config.Hardware.Device,
							&vimtypes.VirtualDisk{
								VirtualDevice: vimtypes.VirtualDevice{
									Key:           300 + int32(i),
									ControllerKey: 100 + int32(i),
									UnitNumber:    ptr.To[int32](0),
									Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
										VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
											FileName: fmt.Sprintf("[LocalDS_0] vm1/disk%d.vmdk", i),
										},
										Uuid: fmt.Sprintf("disk-uuid-%d", i),
									},
								},
								CapacityInBytes: 1024 * 1024 * 1024,
							})
					}

					moVM.Config.Hardware.Device = append(
						moVM.Config.Hardware.Device,
						&vimtypes.ParaVirtualSCSIController{
							VirtualSCSIController: vimtypes.VirtualSCSIController{
								VirtualController: vimtypes.VirtualController{
									VirtualDevice: vimtypes.VirtualDevice{
										Key: 100,
									},
									BusNumber: 0,
								},
							},
						})
					moVM.Config.Hardware.Device = append(
						moVM.Config.Hardware.Device,
						&vimtypes.VirtualLsiLogicController{
							VirtualSCSIController: vimtypes.VirtualSCSIController{
								VirtualController: vimtypes.VirtualController{
									VirtualDevice: vimtypes.VirtualDevice{
										Key: 101,
									},
									BusNumber: 1,
								},
								SharedBus: vimtypes.VirtualSCSISharingVirtualSharing,
							},
						})
					moVM.Config.Hardware.Device = append(
						moVM.Config.Hardware.Device,
						&vimtypes.VirtualLsiLogicSASController{
							VirtualSCSIController: vimtypes.VirtualSCSIController{
								VirtualController: vimtypes.VirtualController{
									VirtualDevice: vimtypes.VirtualDevice{
										Key: 102,
									},
									BusNumber: 2,
								},
								SharedBus: vimtypes.VirtualSCSISharingPhysicalSharing,
							},
						})
					moVM.Config.Hardware.Device = append(
						moVM.Config.Hardware.Device,
						&vimtypes.VirtualBusLogicController{
							VirtualSCSIController: vimtypes.VirtualSCSIController{
								VirtualController: vimtypes.VirtualController{
									VirtualDevice: vimtypes.VirtualDevice{
										Key: 103,
									},
									BusNumber: 3,
								},
								SharedBus: vimtypes.VirtualSCSISharingNoSharing,
							},
						})

					moVM.Config.Hardware.Device = append(
						moVM.Config.Hardware.Device,
						&vimtypes.VirtualNVMEController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: 104,
								},
								BusNumber: 0,
							},
						})
					moVM.Config.Hardware.Device = append(
						moVM.Config.Hardware.Device,
						&vimtypes.VirtualNVMEController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: 105,
								},
								BusNumber: 1,
							},
							SharedBus: string(vimtypes.VirtualNVMEControllerSharingPhysicalSharing),
						})
					moVM.Config.Hardware.Device = append(
						moVM.Config.Hardware.Device,
						&vimtypes.VirtualNVMEController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: 106,
								},
								BusNumber: 2,
							},
							SharedBus: string(vimtypes.VirtualNVMEControllerSharingNoSharing),
						})
				})

				It("should return the expected UnmanagedVolumeInfo", func() {
					Expect(info.Volumes).To(BeEmpty())

					Expect(info.Controllers).To(HaveLen(diskCount))
					// Validate ParaVirtual SCSI Controller
					Expect(info.Controllers[100].Type).To(Equal(vmopv1.VirtualControllerTypeSCSI))
					Expect(info.Controllers[100].ScsiType).To(Equal(vmopv1.SCSIControllerTypeParaVirtualSCSI))
					Expect(info.Controllers[100].Bus).To(Equal(int32(0)))
					Expect(info.Controllers[100].SharingMode).To(BeEmpty()) // Default when SharedBus not set
					// Validate LsiLogic SCSI Controller
					Expect(info.Controllers[101].Type).To(Equal(vmopv1.VirtualControllerTypeSCSI))
					Expect(info.Controllers[101].ScsiType).To(Equal(vmopv1.SCSIControllerTypeLsiLogic))
					Expect(info.Controllers[101].Bus).To(Equal(int32(1)))
					Expect(info.Controllers[101].SharingMode).To(Equal(vmopv1.VirtualControllerSharingModeVirtual))
					// Validate LsiLogicSAS SCSI Controller
					Expect(info.Controllers[102].Type).To(Equal(vmopv1.VirtualControllerTypeSCSI))
					Expect(info.Controllers[102].ScsiType).To(Equal(vmopv1.SCSIControllerTypeLsiLogicSAS))
					Expect(info.Controllers[102].Bus).To(Equal(int32(2)))
					Expect(info.Controllers[102].SharingMode).To(Equal(vmopv1.VirtualControllerSharingModePhysical))
					// Validate BusLogic SCSI Controller
					Expect(info.Controllers[103].Type).To(Equal(vmopv1.VirtualControllerTypeSCSI))
					Expect(info.Controllers[103].ScsiType).To(Equal(vmopv1.SCSIControllerTypeBusLogic))
					Expect(info.Controllers[103].Bus).To(Equal(int32(3)))
					Expect(info.Controllers[103].SharingMode).To(Equal(vmopv1.VirtualControllerSharingModeNone))
					// Validate NVME Controllers
					Expect(info.Controllers[104].Type).To(Equal(vmopv1.VirtualControllerTypeNVME))
					Expect(info.Controllers[104].Bus).To(Equal(int32(0)))
					Expect(info.Controllers[104].SharingMode).To(BeEmpty()) // Default for NVME with no SharedBus
					Expect(info.Controllers[105].Type).To(Equal(vmopv1.VirtualControllerTypeNVME))
					Expect(info.Controllers[105].Bus).To(Equal(int32(1)))
					Expect(info.Controllers[105].SharingMode).To(Equal(vmopv1.VirtualControllerSharingModePhysical))
					Expect(info.Controllers[106].Type).To(Equal(vmopv1.VirtualControllerTypeNVME))
					Expect(info.Controllers[106].Bus).To(Equal(int32(2)))
					Expect(info.Controllers[106].SharingMode).To(Equal(vmopv1.VirtualControllerSharingModeNone))

					Expect(info.Disks).To(HaveLen(diskCount))
					for i := 0; i < diskCount; i++ {
						Expect(info.Disks[i].UUID).To(Equal(fmt.Sprintf("disk-uuid-%d", i)))
						Expect(info.Disks[i].FileName).To(Equal(fmt.Sprintf("[LocalDS_0] vm1/disk%d.vmdk", i)))
						Expect(info.Disks[i].ControllerKey).To(Equal(int32(100 + i)))
						Expect(info.Disks[i].DeviceKey).To(Equal(int32(300 + i)))
						Expect(info.Disks[i].CapacityInBytes).To(Equal(int64(1024 * 1024 * 1024)))
					}
				})
			})

			When("there is a disk attached to a paravirtual scsi controller", func() {

				BeforeEach(func() {
					moVM.Config = &vimtypes.VirtualMachineConfigInfo{
						Hardware: vimtypes.VirtualHardware{
							Device: []vimtypes.BaseVirtualDevice{
								&vimtypes.ParaVirtualSCSIController{
									VirtualSCSIController: vimtypes.VirtualSCSIController{
										VirtualController: vimtypes.VirtualController{
											VirtualDevice: vimtypes.VirtualDevice{
												Key: 100,
											},
											BusNumber: 0,
										},
									},
								},
							},
						},
					}
				})

				When("that is a child", func() {
					Context("of a snapshot", func() {
						BeforeEach(func() {
							disk := &vimtypes.VirtualDisk{
								VirtualDevice: vimtypes.VirtualDevice{
									Key:           300,
									ControllerKey: 100,
									UnitNumber:    ptr.To[int32](0),
									Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
										VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
											FileName:        "[LocalDS_0] vm1/disk-child.vmdk",
											BackingObjectId: "parent-disk-id",
										},
										Uuid: "disk-uuid-child",
										Parent: &vimtypes.VirtualDiskSeSparseBackingInfo{ // Indicates it has a parent
											VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
												FileName: "[LocalDS_0] vm1/disk-parent.vmdk",
											},
										},
									},
								},
								CapacityInBytes: 1024 * 1024 * 1024,
							}
							moVM.Config.Hardware.Device = append(moVM.Config.Hardware.Device, disk)

							// Add LayoutEx with snapshot info
							moVM.LayoutEx = &vimtypes.VirtualMachineFileLayoutEx{
								Snapshot: []vimtypes.VirtualMachineFileLayoutExSnapshotLayout{
									{
										Disk: []vimtypes.VirtualMachineFileLayoutExDiskLayout{
											{Key: 300},
										},
									},
								},
							}
						})
						It("should include the disk", func() {
							Expect(info.Disks).To(HaveLen(1))
							Expect(info.Disks[0].UUID).To(Equal("disk-uuid-child"))
						})
					})

					Context("not of a snapshot", func() {
						BeforeEach(func() {
							disk := &vimtypes.VirtualDisk{
								VirtualDevice: vimtypes.VirtualDevice{
									Key:           300,
									ControllerKey: 100,
									UnitNumber:    ptr.To[int32](0),
									Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
										VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
											FileName:        "[LocalDS_0] vm1/disk-child.vmdk",
											BackingObjectId: "parent-disk-id",
										},
										Uuid: "disk-uuid-child",
										Parent: &vimtypes.VirtualDiskSeSparseBackingInfo{ // Indicates it has a parent
											VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
												FileName: "[LocalDS_0] vm1/disk-parent.vmdk",
											},
										},
									},
								},
								CapacityInBytes: 1024 * 1024 * 1024,
							}
							moVM.Config.Hardware.Device = append(moVM.Config.Hardware.Device, disk)
						})
						It("should exclude the disk", func() {
							Expect(info.Disks).To(BeEmpty())
						})
					})
				})

				When("that is an FCD with non-empty ID", func() {
					BeforeEach(func() {
						disk := &vimtypes.VirtualDisk{
							VirtualDevice: vimtypes.VirtualDevice{
								Key:           300,
								ControllerKey: 100,
								UnitNumber:    ptr.To[int32](0),
								Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
									VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
										FileName: "[LocalDS_0] vm1/disk.vmdk",
									},
									Uuid: "disk-uuid-fcd",
								},
							},
							CapacityInBytes: 1024 * 1024 * 1024,
							VDiskId:         &vimtypes.ID{Id: "fcd-12345"}, // FCD disk
						}
						moVM.Config.Hardware.Device = append(moVM.Config.Hardware.Device, disk)
					})
					It("should exclude FCD disks", func() {
						Expect(info.Disks).To(BeEmpty())
					})
				})

				When("that is an FCD with empty ID", func() {
					BeforeEach(func() {
						disk := &vimtypes.VirtualDisk{
							VirtualDevice: vimtypes.VirtualDevice{
								Key:           300,
								ControllerKey: 100,
								UnitNumber:    ptr.To[int32](0),
								Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
									VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
										FileName: "[LocalDS_0] vm1/disk.vmdk",
									},
									Uuid: "disk-uuid-regular-2",
								},
							},
							CapacityInBytes: 1024 * 1024 * 1024,
							VDiskId:         &vimtypes.ID{Id: ""}, // Empty FCD ID
						}
						moVM.Config.Hardware.Device = append(moVM.Config.Hardware.Device, disk)
					})
					It("should include disk with empty FCD ID", func() {
						Expect(info.Disks).To(HaveLen(1))
						Expect(info.Disks[0].UUID).To(Equal("disk-uuid-regular-2"))
					})
				})

				When("that has no UUID", func() {
					BeforeEach(func() {
						disk := &vimtypes.VirtualDisk{
							VirtualDevice: vimtypes.VirtualDevice{
								Key:           300,
								ControllerKey: 100,
								UnitNumber:    ptr.To[int32](0),
								Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
									VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
										FileName: "[LocalDS_0] vm1/disk-no-uuid.vmdk",
									},
									Uuid: "", // No UUID
								},
							},
							CapacityInBytes: 1024 * 1024 * 1024,
						}
						moVM.Config.Hardware.Device = append(moVM.Config.Hardware.Device, disk)
					})
					It("should exclude disks without UUID", func() {
						Expect(info.Disks).To(BeEmpty())
					})
				})

				When("that has no filename", func() {
					BeforeEach(func() {
						disk := &vimtypes.VirtualDisk{
							VirtualDevice: vimtypes.VirtualDevice{
								Key:           300,
								ControllerKey: 100,
								UnitNumber:    ptr.To[int32](0),
								Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
									VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
										FileName: "", // No filename
									},
									Uuid: "disk-uuid-no-file",
								},
							},
							CapacityInBytes: 1024 * 1024 * 1024,
						}
						moVM.Config.Hardware.Device = append(moVM.Config.Hardware.Device, disk)
					})
					It("should exclude disks without filename", func() {
						Expect(info.Disks).To(BeEmpty())
					})
				})

				When("that is a non-FCD", func() {
					BeforeEach(func() {
						disk := &vimtypes.VirtualDisk{
							VirtualDevice: vimtypes.VirtualDevice{
								Key:           300,
								ControllerKey: 100,
								UnitNumber:    ptr.To[int32](0),
								Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
									VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
										FileName: "[LocalDS_0] vm1/disk.vmdk",
									},
									Uuid: "disk-uuid-regular",
								},
							},
							CapacityInBytes: 1024 * 1024 * 1024,
						}
						moVM.Config.Hardware.Device = append(moVM.Config.Hardware.Device, disk)
					})
					It("should include regular disks", func() {
						Expect(info.Disks).To(HaveLen(1))
						Expect(info.Disks[0].UUID).To(Equal("disk-uuid-regular"))
						Expect(info.Disks[0].FileName).To(Equal("[LocalDS_0] vm1/disk.vmdk"))
					})
				})
			})
		})

		When("vm has volumes", func() {
			JustBeforeEach(func() {
				Expect(info.Volumes).ToNot(BeEmpty())
			})

			When("vm has a volume", func() {
				BeforeEach(func() {
					vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
						{
							Name: "vol1",
							VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
								PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
									UnmanagedVolumeClaim: &vmopv1.UnmanagedVolumeClaimVolumeSource{
										UUID: "disk-uuid-1",
										Type: vmopv1.UnmanagedVolumeClaimVolumeTypeFromVM,
									},
								},
							},
						},
					}

					moVM.Config = &vimtypes.VirtualMachineConfigInfo{
						Hardware: vimtypes.VirtualHardware{
							Device: []vimtypes.BaseVirtualDevice{
								&vimtypes.ParaVirtualSCSIController{
									VirtualSCSIController: vimtypes.VirtualSCSIController{
										VirtualController: vimtypes.VirtualController{
											VirtualDevice: vimtypes.VirtualDevice{
												Key: 100,
											},
											BusNumber: 0,
										},
									},
								},
								&vimtypes.VirtualDisk{
									VirtualDevice: vimtypes.VirtualDevice{
										Key:           300,
										ControllerKey: 100,
										UnitNumber:    ptr.To[int32](0),
										Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
											VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
												FileName: "[LocalDS_0] vm1/disk.vmdk",
											},
											Uuid: "disk-uuid-1",
										},
									},
									CapacityInBytes: 1024 * 1024 * 1024,
								},
							},
						},
					}
				})

				It("should return the expected UnmanagedVolumeInfo", func() {
					Expect(info.Volumes).To(HaveLen(1))
					Expect(info.Volumes["disk-uuid-1"].Name).To(Equal("vol1"))

					Expect(info.Controllers).To(HaveLen(1))
					Expect(info.Controllers[100].Type).To(Equal(vmopv1.VirtualControllerTypeSCSI))
					Expect(info.Controllers[100].ScsiType).To(Equal(vmopv1.SCSIControllerTypeParaVirtualSCSI))

					Expect(info.Disks).To(HaveLen(1))
					Expect(info.Disks[0].UUID).To(Equal("disk-uuid-1"))
					Expect(info.Disks[0].Type).To(Equal(vmopv1.UnmanagedVolumeClaimVolumeTypeFromVM))
				})
			})

			When("vm has two volumes", func() {
				When("one disk is attached to a paravirtual scsi controller and the other disk to an nvme controller", func() {
					BeforeEach(func() {
						vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
							{
								Name: "vol1",
								VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
									PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
										UnmanagedVolumeClaim: &vmopv1.UnmanagedVolumeClaimVolumeSource{
											UUID: "disk-uuid-1",
											Type: vmopv1.UnmanagedVolumeClaimVolumeTypeFromVM,
										},
									},
								},
							},
							{
								Name: "vol2",
								VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
									PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
										UnmanagedVolumeClaim: &vmopv1.UnmanagedVolumeClaimVolumeSource{
											UUID: "disk-uuid-2",
											Type: vmopv1.UnmanagedVolumeClaimVolumeTypeFromImage,
										},
									},
								},
							},
						}

						moVM.Config = &vimtypes.VirtualMachineConfigInfo{
							Hardware: vimtypes.VirtualHardware{
								Device: []vimtypes.BaseVirtualDevice{
									&vimtypes.ParaVirtualSCSIController{
										VirtualSCSIController: vimtypes.VirtualSCSIController{
											VirtualController: vimtypes.VirtualController{
												VirtualDevice: vimtypes.VirtualDevice{
													Key: 100,
												},
												BusNumber: 0,
											},
										},
									},
									&vimtypes.VirtualNVMEController{
										VirtualController: vimtypes.VirtualController{
											VirtualDevice: vimtypes.VirtualDevice{
												Key: 200,
											},
											BusNumber: 0,
										},
									},
									&vimtypes.VirtualDisk{
										VirtualDevice: vimtypes.VirtualDevice{
											Key:           300,
											ControllerKey: 100,
											UnitNumber:    ptr.To[int32](0),
											Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
												VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
													FileName: "[LocalDS_0] vm1/disk1.vmdk",
												},
												Uuid: "disk-uuid-1",
											},
										},
										CapacityInBytes: 1024 * 1024 * 1024,
									},
									&vimtypes.VirtualDisk{
										VirtualDevice: vimtypes.VirtualDevice{
											Key:           400,
											ControllerKey: 200,
											UnitNumber:    ptr.To[int32](0),
											Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
												VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
													FileName: "[LocalDS_0] vm1/disk2.vmdk",
												},
												Uuid: "disk-uuid-2",
											},
										},
										CapacityInBytes: 2 * 1024 * 1024 * 1024,
									},
								},
							},
						}
					})

					It("should return the expected UnmanagedVolumeInfo", func() {
						Expect(info.Volumes).To(HaveLen(2))
						Expect(info.Volumes["disk-uuid-1"].Name).To(Equal("vol1"))
						Expect(info.Volumes["disk-uuid-2"].Name).To(Equal("vol2"))

						Expect(info.Controllers).To(HaveLen(2))
						Expect(info.Controllers[100].Type).To(Equal(vmopv1.VirtualControllerTypeSCSI))
						Expect(info.Controllers[200].Type).To(Equal(vmopv1.VirtualControllerTypeNVME))

						Expect(info.Disks).To(HaveLen(2))
						Expect(info.Disks[0].UUID).To(Equal("disk-uuid-1"))
						Expect(info.Disks[0].Type).To(Equal(vmopv1.UnmanagedVolumeClaimVolumeTypeFromVM))
						Expect(info.Disks[1].UUID).To(Equal("disk-uuid-2"))
						Expect(info.Disks[1].Type).To(Equal(vmopv1.UnmanagedVolumeClaimVolumeTypeFromImage))
					})
				})
			})

			When("vm has three volumes", func() {
				When("two disks are attached to the same sata controller and the other disk to an ide controller", func() {
					BeforeEach(func() {
						vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
							{
								Name: "vol1",
								VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
									PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
										UnmanagedVolumeClaim: &vmopv1.UnmanagedVolumeClaimVolumeSource{
											UUID: "disk-uuid-1",
											Type: vmopv1.UnmanagedVolumeClaimVolumeTypeFromVM,
										},
									},
								},
							},
							{
								Name: "vol2",
								VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
									PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
										UnmanagedVolumeClaim: &vmopv1.UnmanagedVolumeClaimVolumeSource{
											UUID: "disk-uuid-2",
											Type: vmopv1.UnmanagedVolumeClaimVolumeTypeFromImage,
										},
									},
								},
							},
							{
								Name: "vol3",
								VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
									PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
										UnmanagedVolumeClaim: &vmopv1.UnmanagedVolumeClaimVolumeSource{
											UUID: "disk-uuid-3",
											Type: vmopv1.UnmanagedVolumeClaimVolumeTypeFromVM,
										},
									},
								},
							},
						}

						moVM.Config = &vimtypes.VirtualMachineConfigInfo{
							Hardware: vimtypes.VirtualHardware{
								Device: []vimtypes.BaseVirtualDevice{
									&vimtypes.VirtualAHCIController{
										VirtualSATAController: vimtypes.VirtualSATAController{
											VirtualController: vimtypes.VirtualController{
												VirtualDevice: vimtypes.VirtualDevice{
													Key: 100,
												},
												BusNumber: 0,
											},
										},
									},
									&vimtypes.VirtualIDEController{
										VirtualController: vimtypes.VirtualController{
											VirtualDevice: vimtypes.VirtualDevice{
												Key: 200,
											},
											BusNumber: 0,
										},
									},
									&vimtypes.VirtualDisk{
										VirtualDevice: vimtypes.VirtualDevice{
											Key:           300,
											ControllerKey: 100,
											UnitNumber:    ptr.To[int32](0),
											Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
												VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
													FileName: "[LocalDS_0] vm1/disk1.vmdk",
												},
												Uuid: "disk-uuid-1",
											},
										},
										CapacityInBytes: 1024 * 1024 * 1024,
									},
									&vimtypes.VirtualDisk{
										VirtualDevice: vimtypes.VirtualDevice{
											Key:           400,
											ControllerKey: 100,
											UnitNumber:    ptr.To[int32](1),
											Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
												VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
													FileName: "[LocalDS_0] vm1/disk2.vmdk",
												},
												Uuid: "disk-uuid-2",
											},
										},
										CapacityInBytes: 2 * 1024 * 1024 * 1024,
									},
									&vimtypes.VirtualDisk{
										VirtualDevice: vimtypes.VirtualDevice{
											Key:           500,
											ControllerKey: 200,
											UnitNumber:    ptr.To[int32](0),
											Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
												VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
													FileName: "[LocalDS_0] vm1/disk3.vmdk",
												},
												Uuid: "disk-uuid-3",
											},
										},
										CapacityInBytes: 3 * 1024 * 1024 * 1024,
									},
								},
							},
						}
					})

					It("should return the expected UnmanagedVolumeInfo", func() {
						Expect(info.Volumes).To(HaveLen(3))
						Expect(info.Volumes["disk-uuid-1"].Name).To(Equal("vol1"))
						Expect(info.Volumes["disk-uuid-2"].Name).To(Equal("vol2"))
						Expect(info.Volumes["disk-uuid-3"].Name).To(Equal("vol3"))

						Expect(info.Controllers).To(HaveLen(2))
						Expect(info.Controllers[100].Type).To(Equal(vmopv1.VirtualControllerTypeSATA))
						Expect(info.Controllers[200].Type).To(Equal(vmopv1.VirtualControllerTypeIDE))

						Expect(info.Disks).To(HaveLen(3))
						Expect(info.Disks[0].UUID).To(Equal("disk-uuid-1"))
						Expect(info.Disks[0].Type).To(Equal(vmopv1.UnmanagedVolumeClaimVolumeTypeFromVM))
						Expect(info.Disks[1].UUID).To(Equal("disk-uuid-2"))
						Expect(info.Disks[1].Type).To(Equal(vmopv1.UnmanagedVolumeClaimVolumeTypeFromImage))
						Expect(info.Disks[2].UUID).To(Equal("disk-uuid-3"))
						Expect(info.Disks[2].Type).To(Equal(vmopv1.UnmanagedVolumeClaimVolumeTypeFromVM))
					})
				})
			})
		})

	})

})
