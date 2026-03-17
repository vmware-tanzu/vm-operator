// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package volumes_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	pkgvol "github.com/vmware-tanzu/vm-operator/pkg/util/volumes"
)

var _ = Context("Context", func() {
	It("should not panic", func() {
		_, ok := pkgvol.FromContext(context.Background())
		Expect(ok).To(BeFalse())

		ctx := pkgvol.WithContext(
			context.Background(), pkgvol.VolumeInfo{})
		v, ok := pkgvol.FromContext(ctx)
		Expect(ok).To(BeTrue())
		Expect(v).To(Equal(pkgvol.VolumeInfo{}))
	})
})

var _ = Describe("GetVolumeInfoFrom", func() {
	var (
		vm      *vmopv1.VirtualMachine
		devices []vimtypes.BaseVirtualDevice
	)

	BeforeEach(func() {
		vm = &vmopv1.VirtualMachine{}
		devices = nil
	})

	When("virtual disk as nil unit number", func() {
		BeforeEach(func() {
			devices = []vimtypes.BaseVirtualDevice{
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
						Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
							VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
								FileName: "[LocalDS_0] vm1/disk1.vmdk",
							},
							Uuid: "disk-uuid-1",
						},
						UnitNumber: ptr.To[int32](0),
					},
					CapacityInBytes: 1024 * 1024 * 1024,
				},
				&vimtypes.VirtualDisk{
					VirtualDevice: vimtypes.VirtualDevice{
						Key:           300,
						ControllerKey: 101,
						Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
							VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
								FileName: "[LocalDS_0] vm1/disk2.vmdk",
							},
							Uuid: "disk-uuid-2",
						},
					},
					CapacityInBytes: 1024 * 1024 * 1024,
				},
			}
		})

		It("should panic", func() {
			fn := func() {
				_ = pkgvol.GetVolumeInfo(vm, devices, nil)
			}
			Expect(fn).To(PanicWith(
				"disk at device index 2 missing unit number"))
		})
	})
})

var _ = Describe("GetVolumeInfoFromVM", func() {
	var (
		vm   *vmopv1.VirtualMachine
		moVM mo.VirtualMachine
		info pkgvol.VolumeInfo
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
		info = pkgvol.GetVolumeInfoFromVM(vm, moVM)
	})

	When("vm is nil", func() {
		BeforeEach(func() {
			vm = nil
		})
		It("should return an empty VolumeInfo", func() {
			Expect(info.Disks).To(BeEmpty())
			Expect(info.Controllers).To(BeEmpty())
			Expect(info.Volumes).To(BeEmpty())
		})
	})

	When("moVM config is nil", func() {
		BeforeEach(func() {
			moVM.Config = nil
		})
		It("should return an empty VolumeInfo", func() {
			Expect(info.Disks).To(BeEmpty())
			Expect(info.Controllers).To(BeEmpty())
			Expect(info.Volumes).To(BeEmpty())
		})
	})

	When("moVM has no devices", func() {
		BeforeEach(func() {
			moVM.Config.Hardware.Device = nil
		})
		It("should return an empty VolumeInfo", func() {
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
		It("should return an empty VolumeInfo", func() {
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
		It("should return the expected VolumeInfo", func() {
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
		It("should return the expected VolumeInfo", func() {
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

				It("should return the expected VolumeInfo", func() {
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
							Expect(pkgvol.FilterOutSnapshots(info.Disks...)).To(BeEmpty())
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
							Expect(pkgvol.FilterOutLinkedClones(info.Disks...)).To(BeEmpty())
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
						Expect(pkgvol.FilterOutFCDs(info.Disks...)).To(BeEmpty())
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
						Expect(pkgvol.FilterOutEmptyUUIDOrFilename(info.Disks...)).To(BeEmpty())
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
						Expect(pkgvol.FilterOutEmptyUUIDOrFilename(info.Disks...)).To(BeEmpty())
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

			Context("with no target information", func() {

				JustBeforeEach(func() {
					Expect(info.Volumes).To(BeEmpty())
				})

				When("vm has a volume", func() {
					BeforeEach(func() {
						vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
							{
								Name: "vol1",
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
											Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
												VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
													FileName: "[LocalDS_0] vm1/disk.vmdk",
												},
												Uuid: "disk-uuid-1",
											},
											UnitNumber: ptr.To[int32](0),
										},
										CapacityInBytes: 1024 * 1024 * 1024,
									},
								},
							},
						}
					})

					It("should return the expected VolumeInfo", func() {
						Expect(info.Controllers).To(HaveLen(1))
						Expect(info.Controllers[100].Type).To(Equal(vmopv1.VirtualControllerTypeSCSI))
						Expect(info.Controllers[100].ScsiType).To(Equal(vmopv1.SCSIControllerTypeParaVirtualSCSI))

						Expect(info.Disks).To(HaveLen(1))
						Expect(info.Disks[0].UUID).To(Equal("disk-uuid-1"))
					})
				})

				When("vm has two volumes", func() {
					When("one disk is attached to a paravirtual scsi controller and the other disk to an nvme controller", func() {
						BeforeEach(func() {
							vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
								{
									Name: "vol1",
								},
								{
									Name: "vol2",
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

						It("should return the expected VolumeInfo", func() {
							Expect(info.Controllers).To(HaveLen(2))
							Expect(info.Controllers[100].Type).To(Equal(vmopv1.VirtualControllerTypeSCSI))
							Expect(info.Controllers[200].Type).To(Equal(vmopv1.VirtualControllerTypeNVME))

							Expect(info.Disks).To(HaveLen(2))
							Expect(info.Disks[0].UUID).To(Equal("disk-uuid-1"))
							Expect(info.Disks[1].UUID).To(Equal("disk-uuid-2"))
						})
					})
				})

				When("vm has three volumes", func() {
					When("two disks are attached to the same sata controller and the other disk to an ide controller", func() {
						BeforeEach(func() {
							vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
								{
									Name: "vol1",
								},
								{
									Name: "vol2",
								},
								{
									Name: "vol3",
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

						It("should return the expected VolumeInfo", func() {
							Expect(info.Controllers).To(HaveLen(2))
							Expect(info.Controllers[100].Type).To(Equal(vmopv1.VirtualControllerTypeSATA))
							Expect(info.Controllers[200].Type).To(Equal(vmopv1.VirtualControllerTypeIDE))

							Expect(info.Disks).To(HaveLen(3))
							Expect(info.Disks[0].UUID).To(Equal("disk-uuid-1"))
							Expect(info.Disks[1].UUID).To(Equal("disk-uuid-2"))
							Expect(info.Disks[2].UUID).To(Equal("disk-uuid-3"))
						})
					})
				})
			})

			Context("with target information", func() {

				JustBeforeEach(func() {
					Expect(info.Volumes).ToNot(BeEmpty())
				})

				When("vm has a volume", func() {
					BeforeEach(func() {
						vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
							{
								Name:                "vol1",
								ControllerType:      vmopv1.VirtualControllerTypeSCSI,
								ControllerBusNumber: ptr.To[int32](0),
								UnitNumber:          ptr.To[int32](0),
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

					It("should return the expected VolumeInfo", func() {
						Expect(info.Volumes).To(HaveLen(1))
						Expect(info.Volumes[vmopv1util.TargetID{
							ControllerType: vmopv1.VirtualControllerTypeSCSI,
							ControllerBus:  0,
							UnitNumber:     0,
						}.String()].Name).To(Equal("vol1"))

						Expect(info.Controllers).To(HaveLen(1))
						Expect(info.Controllers[100].Type).To(Equal(vmopv1.VirtualControllerTypeSCSI))
						Expect(info.Controllers[100].ScsiType).To(Equal(vmopv1.SCSIControllerTypeParaVirtualSCSI))

						Expect(info.Disks).To(HaveLen(1))
						Expect(info.Disks[0].UUID).To(Equal("disk-uuid-1"))
					})
				})

				When("vm has two volumes", func() {
					When("one disk is attached to a paravirtual scsi controller and the other disk to an nvme controller", func() {
						BeforeEach(func() {
							vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
								{
									Name:                "vol1",
									ControllerType:      vmopv1.VirtualControllerTypeSCSI,
									ControllerBusNumber: ptr.To[int32](0),
									UnitNumber:          ptr.To[int32](0),
								},
								{
									Name:                "vol2",
									ControllerType:      vmopv1.VirtualControllerTypeNVME,
									ControllerBusNumber: ptr.To[int32](0),
									UnitNumber:          ptr.To[int32](0),
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

						It("should return the expected VolumeInfo", func() {
							Expect(info.Volumes).To(HaveLen(2))
							Expect(info.Volumes[vmopv1util.TargetID{
								ControllerType: vmopv1.VirtualControllerTypeSCSI,
								ControllerBus:  0,
								UnitNumber:     0,
							}.String()].Name).To(Equal("vol1"))
							Expect(info.Volumes[vmopv1util.TargetID{
								ControllerType: vmopv1.VirtualControllerTypeNVME,
								ControllerBus:  0,
								UnitNumber:     0,
							}.String()].Name).To(Equal("vol2"))

							Expect(info.Controllers).To(HaveLen(2))
							Expect(info.Controllers[100].Type).To(Equal(vmopv1.VirtualControllerTypeSCSI))
							Expect(info.Controllers[200].Type).To(Equal(vmopv1.VirtualControllerTypeNVME))

							Expect(info.Disks).To(HaveLen(2))
							Expect(info.Disks[0].UUID).To(Equal("disk-uuid-1"))
							Expect(info.Disks[1].UUID).To(Equal("disk-uuid-2"))
						})
					})
				})

				When("vm has three volumes", func() {
					When("two disks are attached to the same sata controller and the other disk to an ide controller", func() {
						BeforeEach(func() {
							vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
								{
									Name:                "vol1",
									ControllerType:      vmopv1.VirtualControllerTypeSATA,
									ControllerBusNumber: ptr.To[int32](0),
									UnitNumber:          ptr.To[int32](0),
								},
								{
									Name:                "vol2",
									ControllerType:      vmopv1.VirtualControllerTypeSATA,
									ControllerBusNumber: ptr.To[int32](0),
									UnitNumber:          ptr.To[int32](1),
								},
								{
									Name:                "vol3",
									ControllerType:      vmopv1.VirtualControllerTypeIDE,
									ControllerBusNumber: ptr.To[int32](0),
									UnitNumber:          ptr.To[int32](0),
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

						It("should return the expected VolumeInfo", func() {
							Expect(info.Volumes).To(HaveLen(3))
							Expect(info.Volumes[vmopv1util.TargetID{
								ControllerType: vmopv1.VirtualControllerTypeSATA,
								ControllerBus:  0,
								UnitNumber:     0,
							}.String()].Name).To(Equal("vol1"))
							Expect(info.Volumes[vmopv1util.TargetID{
								ControllerType: vmopv1.VirtualControllerTypeSATA,
								ControllerBus:  0,
								UnitNumber:     1,
							}.String()].Name).To(Equal("vol2"))
							Expect(info.Volumes[vmopv1util.TargetID{
								ControllerType: vmopv1.VirtualControllerTypeIDE,
								ControllerBus:  0,
								UnitNumber:     0,
							}.String()].Name).To(Equal("vol3"))

							Expect(info.Controllers).To(HaveLen(2))
							Expect(info.Controllers[100].Type).To(Equal(vmopv1.VirtualControllerTypeSATA))
							Expect(info.Controllers[200].Type).To(Equal(vmopv1.VirtualControllerTypeIDE))

							Expect(info.Disks).To(HaveLen(3))
							Expect(info.Disks[0].UUID).To(Equal("disk-uuid-1"))
							Expect(info.Disks[1].UUID).To(Equal("disk-uuid-2"))
							Expect(info.Disks[2].UUID).To(Equal("disk-uuid-3"))
						})
					})
				})
			})
		})
	})

})

var _ = Describe("GetVolumeInfoFromConfigSpec", func() {
	var (
		vm         *vmopv1.VirtualMachine
		configSpec *vimtypes.VirtualMachineConfigSpec
		info       pkgvol.VolumeInfo
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
		configSpec = &vimtypes.VirtualMachineConfigSpec{}
	})

	JustBeforeEach(func() {
		info = pkgvol.GetVolumeInfoFromConfigSpec(vm, configSpec)
	})

	When("configSpec is nil", func() {
		BeforeEach(func() {
			configSpec = nil
		})
		It("should return an empty VolumeInfo", func() {
			Expect(info.Disks).To(BeEmpty())
			Expect(info.Controllers).To(BeEmpty())
			Expect(info.Volumes).To(BeEmpty())
		})
	})

	When("configSpec has no devices", func() {
		BeforeEach(func() {
			configSpec.DeviceChange = nil
		})
		It("should return an empty VolumeInfo", func() {
			Expect(info).To(BeZero())
			Expect(info.Disks).To(BeEmpty())
			Expect(info.Controllers).To(BeEmpty())
			Expect(info.Volumes).To(BeEmpty())
		})
	})

	When("there is a disk attached to each possible controller type", func() {

		const diskCount = 7

		BeforeEach(func() {

			for i := 0; i < diskCount; i++ {
				configSpec.DeviceChange = append(
					configSpec.DeviceChange,
					&vimtypes.VirtualDeviceConfigSpec{
						Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
						Device: &vimtypes.VirtualDisk{
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
						},
					})
			}

			configSpec.DeviceChange = append(
				configSpec.DeviceChange,
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: 100,
								},
								BusNumber: 0,
							},
						},
					}})
			configSpec.DeviceChange = append(
				configSpec.DeviceChange,
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualLsiLogicController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: 101,
								},
								BusNumber: 1,
							},
							SharedBus: vimtypes.VirtualSCSISharingVirtualSharing,
						},
					}})
			configSpec.DeviceChange = append(
				configSpec.DeviceChange,
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualLsiLogicSASController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: 102,
								},
								BusNumber: 2,
							},
							SharedBus: vimtypes.VirtualSCSISharingPhysicalSharing,
						},
					}})
			configSpec.DeviceChange = append(
				configSpec.DeviceChange,
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualBusLogicController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: 103,
								},
								BusNumber: 3,
							},
							SharedBus: vimtypes.VirtualSCSISharingNoSharing,
						},
					}})

			configSpec.DeviceChange = append(
				configSpec.DeviceChange,
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualNVMEController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: 104,
							},
							BusNumber: 0,
						},
					}})
			configSpec.DeviceChange = append(
				configSpec.DeviceChange,
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualNVMEController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: 105,
							},
							BusNumber: 1,
						},
						SharedBus: string(vimtypes.VirtualNVMEControllerSharingPhysicalSharing),
					}})
			configSpec.DeviceChange = append(
				configSpec.DeviceChange,
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualNVMEController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: 106,
							},
							BusNumber: 2,
						},
						SharedBus: string(vimtypes.VirtualNVMEControllerSharingNoSharing),
					}})
		})

		It("should return the expected VolumeInfo", func() {
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

})
