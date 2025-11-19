// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package backfill_test

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgcond "github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig"
	unmanagedvolsfill "github.com/vmware-tanzu/vm-operator/pkg/vmconfig/volumes/unmanaged/backfill"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("New", func() {
	It("should return a reconciler", func() {
		Expect(unmanagedvolsfill.New()).ToNot(BeNil())
	})
})

var _ = Describe("Name", func() {
	It("should return 'unmanagedvolumes-backfill'", func() {
		Expect(unmanagedvolsfill.New().Name()).To(Equal("unmanagedvolumes-backfill"))
	})
})

var _ = Describe("OnResult", func() {
	It("should return nil", func() {
		var ctx context.Context
		Expect(unmanagedvolsfill.New().OnResult(ctx, nil, mo.VirtualMachine{}, nil)).To(Succeed())
	})
})

var _ = Describe("Reconcile", func() {

	var (
		r          vmconfig.Reconciler
		ctx        context.Context
		k8sClient  ctrlclient.Client
		vimClient  *vim25.Client
		moVM       mo.VirtualMachine
		vm         *vmopv1.VirtualMachine
		withObjs   []ctrlclient.Object
		withFuncs  interceptor.Funcs
		configSpec *vimtypes.VirtualMachineConfigSpec
	)

	BeforeEach(func() {
		r = unmanagedvolsfill.New()

		ctx = pkgcfg.NewContext()
		ctx = vmconfig.WithContext(ctx)
		ctx = logr.NewContext(ctx, klog.Background())

		moVM = mo.VirtualMachine{
			Config:   &vimtypes.VirtualMachineConfigInfo{},
			LayoutEx: &vimtypes.VirtualMachineFileLayoutEx{},
		}

		configSpec = &vimtypes.VirtualMachineConfigSpec{}

		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:   "my-namespace",
				Name:        "my-vm",
				Annotations: map[string]string{},
			},
			Spec: vmopv1.VirtualMachineSpec{},
			Status: vmopv1.VirtualMachineStatus{
				UniqueID: "vm-1",
			},
		}

		withFuncs = interceptor.Funcs{}
		withObjs = []ctrlclient.Object{}
	})

	JustBeforeEach(func() {
		k8sClient = builder.NewFakeClientWithInterceptors(withFuncs, withObjs...)
	})

	Context("a panic is expected", func() {
		When("ctx is nil", func() {
			JustBeforeEach(func() {
				ctx = nil
			})
			It("should panic", func() {
				fn := func() {
					_ = r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
				}
				Expect(fn).To(PanicWith("context is nil"))
			})
		})
		When("vm is nil", func() {
			JustBeforeEach(func() {
				vm = nil
			})
			It("should panic", func() {
				fn := func() {
					_ = r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
				}
				Expect(fn).To(PanicWith("vm is nil"))
			})
		})
	})

	When("no panic is expected", func() {

		Context("early return conditions", func() {
			When("moVM.Config is nil", func() {
				JustBeforeEach(func() {
					moVM.Config = nil
				})
				It("should return nil without error", func() {
					Expect(r.Reconcile(
						ctx,
						k8sClient,
						vimClient,
						vm,
						moVM,
						configSpec)).To(Succeed())
				})
			})

			When("moVM.Config.Hardware.Device is empty", func() {
				JustBeforeEach(func() {
					moVM.Config = &vimtypes.VirtualMachineConfigInfo{
						Hardware: vimtypes.VirtualHardware{
							Device: []vimtypes.BaseVirtualDevice{},
						},
					}
				})
				It("should return nil without error", func() {
					Expect(r.Reconcile(
						ctx,
						k8sClient,
						vimClient,
						vm,
						moVM,
						configSpec)).To(Succeed())
				})
			})

			When("the vm has a true condition", func() {
				BeforeEach(func() {
					pkgcond.MarkTrue(vm, unmanagedvolsfill.Condition)
				})
				It("should return nil without error", func() {
					Expect(r.Reconcile(
						ctx,
						k8sClient,
						vimClient,
						vm,
						moVM,
						configSpec)).To(Succeed())
				})
			})
		})

		Context("disk processing", func() {
			When("VM has unmanaged disks", func() {
				var (
					disk1          *vimtypes.VirtualDisk
					disk2          *vimtypes.VirtualDisk
					ideController  *vimtypes.VirtualIDEController
					scsiController *vimtypes.VirtualSCSIController
				)

				BeforeEach(func() {
					// Create IDE controller
					ideController = &vimtypes.VirtualIDEController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: 100,
							},
							BusNumber: 0,
						},
					}

					// Create SCSI controller
					scsiController = &vimtypes.VirtualSCSIController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: 200,
							},
							BusNumber: 1,
						},
					}

					// Create disk 1 with SeSparse backing (has UUID)
					disk1 = &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							Key:           300,
							ControllerKey: 100,
							UnitNumber:    ptr.To(int32(0)),
							Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
									FileName: "[LocalDS_0] vm1/disk1.vmdk",
								},
								Uuid: "disk1-uuid-123",
							},
							DeviceInfo: &vimtypes.Description{
								Label: "Hard disk 1",
							},
						},
						CapacityInBytes: 1024 * 1024 * 1024, // 1GB
					}

					// Create disk 2 with FlatVer2 backing (has UUID and sharing)
					disk2 = &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							Key:           400,
							ControllerKey: 200,
							UnitNumber:    ptr.To(int32(0)),
							Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
									FileName: "[LocalDS_0] vm1/disk2.vmdk",
								},
								Uuid:    "disk2-uuid-456",
								Sharing: string(vimtypes.VirtualDiskSharingSharingMultiWriter),
							},
							DeviceInfo: &vimtypes.Description{
								Label: "Hard disk 2",
							},
						},
						CapacityInBytes: 2 * 1024 * 1024 * 1024, // 2GB
					}

					moVM.Config = &vimtypes.VirtualMachineConfigInfo{
						Hardware: vimtypes.VirtualHardware{
							Device: []vimtypes.BaseVirtualDevice{
								ideController,
								scsiController,
								disk1,
								disk2,
							},
						},
					}
				})

				It("should add volumes to VM spec and return ErrUnmanagedVols", func() {
					err := unmanagedvolsfill.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
					Expect(err).To(Equal(unmanagedvolsfill.ErrPendingBackfill))

					// Verify volumes were added to VM spec
					Expect(vm.Spec.Volumes).To(HaveLen(2))

					// Find the volumes by name (UUID)
					vol1 := vmopv1util.FindByTargetID(
						vmopv1.VirtualControllerTypeIDE, 0, 0,
						vm.Spec.Volumes...)
					vol2 := vmopv1util.FindByTargetID(
						vmopv1.VirtualControllerTypeSCSI, 1, 0,
						vm.Spec.Volumes...)

					Expect(vol1).ToNot(BeNil())
					Expect(vol2).ToNot(BeNil())

					// Verify volume 1 properties
					Expect(vol1.PersistentVolumeClaim).To(BeNil())
					Expect(vol1.ControllerType).To(Equal(vmopv1.VirtualControllerTypeIDE))
					Expect(*vol1.ControllerBusNumber).To(Equal(int32(0)))
					Expect(*vol1.UnitNumber).To(Equal(int32(0)))

					// Verify volume 2 properties
					Expect(vol2.PersistentVolumeClaim).To(BeNil())
					Expect(vol2.ControllerType).To(Equal(vmopv1.VirtualControllerTypeSCSI))
					Expect(*vol2.ControllerBusNumber).To(Equal(int32(1)))
					Expect(*vol2.UnitNumber).To(Equal(int32(0)))
				})
			})

			When("VM has existing volumes with matching UUIDs", func() {
				BeforeEach(func() {
					// Start with empty volumes - let Reconcile add them
					vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{}

					// Create a disk with matching UUID
					disk := &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							Key:           300,
							ControllerKey: 100,
							UnitNumber:    ptr.To(int32(0)),
							Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
									FileName: "[LocalDS_0] vm1/existing-disk.vmdk",
								},
								Uuid: "existing-disk-uuid",
							},
						},
						CapacityInBytes: 1024 * 1024 * 1024,
					}

					ideController := &vimtypes.VirtualIDEController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: 100,
							},
							BusNumber: 0,
						},
					}

					moVM.Config = &vimtypes.VirtualMachineConfigInfo{
						Hardware: vimtypes.VirtualHardware{
							Device: []vimtypes.BaseVirtualDevice{
								ideController,
								disk,
							},
						},
					}

				})

				It("should add volumes to VM spec and return ErrUnmanagedVols", func() {
					err := r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
					Expect(err).To(Equal(unmanagedvolsfill.ErrPendingBackfill))
					Expect(vm.Spec.Volumes).To(HaveLen(1))
				})
			})
		})

		Context("disk filtering", func() {
			When("VM has FCD disks", func() {
				BeforeEach(func() {
					// Create FCD disk (has VDiskId)
					fcdDisk := &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							Key:           300,
							ControllerKey: 100,
							UnitNumber:    ptr.To[int32](0),
							Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
									FileName: "[LocalDS_0] vm1/fcd-disk.vmdk",
								},
								Uuid: "fcd-uuid-123",
							},
						},
						CapacityInBytes: 1024 * 1024 * 1024,
						VDiskId: &vimtypes.ID{
							Id: "fcd-id-123",
						},
					}

					ideController := &vimtypes.VirtualIDEController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: 100,
							},
							BusNumber: 0,
						},
					}

					moVM.Config = &vimtypes.VirtualMachineConfigInfo{
						Hardware: vimtypes.VirtualHardware{
							Device: []vimtypes.BaseVirtualDevice{
								ideController,
								fcdDisk,
							},
						},
					}
				})

				It("should skip FCD disks", func() {
					err := r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
					Expect(err).ToNot(HaveOccurred())
					Expect(vm.Spec.Volumes).To(BeEmpty())
				})
			})

			When("VM has disks without UUID", func() {
				BeforeEach(func() {
					// Create disk without UUID (SparseVer1 backing)
					disk := &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							Key:           300,
							ControllerKey: 100,
							UnitNumber:    ptr.To[int32](0),
							Backing: &vimtypes.VirtualDiskSparseVer1BackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
									FileName: "[LocalDS_0] vm1/no-uuid-disk.vmdk",
								},
							},
						},
						CapacityInBytes: 1024 * 1024 * 1024,
					}

					ideController := &vimtypes.VirtualIDEController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: 100,
							},
							BusNumber: 0,
						},
					}

					moVM.Config = &vimtypes.VirtualMachineConfigInfo{
						Hardware: vimtypes.VirtualHardware{
							Device: []vimtypes.BaseVirtualDevice{
								ideController,
								disk,
							},
						},
					}
				})

				It("should skip disks without UUID", func() {
					err := r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
					Expect(err).ToNot(HaveOccurred())
					Expect(vm.Spec.Volumes).To(BeEmpty())
				})
			})

			When("VM has disks without filename", func() {
				BeforeEach(func() {
					// Create disk without filename
					disk := &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							Key:           300,
							ControllerKey: 100,
							UnitNumber:    ptr.To[int32](0),
							Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{},
								Uuid:                         "no-filename-uuid-123",
							},
						},
						CapacityInBytes: 1024 * 1024 * 1024,
					}

					ideController := &vimtypes.VirtualIDEController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: 100,
							},
							BusNumber: 0,
						},
					}

					moVM.Config = &vimtypes.VirtualMachineConfigInfo{
						Hardware: vimtypes.VirtualHardware{
							Device: []vimtypes.BaseVirtualDevice{
								ideController,
								disk,
							},
						},
					}
				})

				It("should skip disks without filename", func() {
					err := r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
					Expect(err).ToNot(HaveOccurred())
					Expect(vm.Spec.Volumes).To(BeEmpty())
				})
			})

			When("VM has child disks not in snapshots", func() {
				BeforeEach(func() {
					// Create child disk (has parent)
					childDisk := &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							Key:           300,
							ControllerKey: 100,
							UnitNumber:    ptr.To[int32](0),
							Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
									FileName: "[LocalDS_0] vm1/child-disk.vmdk",
								},
								Uuid: "child-disk-uuid-123",
								Parent: &vimtypes.VirtualDiskSeSparseBackingInfo{
									VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{},
									Uuid:                         "parent-disk-uuid",
								},
							},
						},
						CapacityInBytes: 1024 * 1024 * 1024,
					}

					ideController := &vimtypes.VirtualIDEController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: 100,
							},
							BusNumber: 0,
						},
					}

					moVM.Config = &vimtypes.VirtualMachineConfigInfo{
						Hardware: vimtypes.VirtualHardware{
							Device: []vimtypes.BaseVirtualDevice{
								ideController,
								childDisk,
							},
						},
					}
				})

				It("should include child disks not in snapshots", func() {
					err := r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
					Expect(err).To(Equal(unmanagedvolsfill.ErrPendingBackfill))
					Expect(vm.Spec.Volumes).To(HaveLen(1))

					vol := vmopv1util.FindByTargetID(
						vmopv1.VirtualControllerTypeIDE, 0, 0,
						vm.Spec.Volumes...)
					Expect(vol).ToNot(BeNil())
				})
			})

			When("VM has child disks in snapshots", func() {
				BeforeEach(func() {
					// Create child disk (has parent) that is in snapshot
					childDisk := &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							Key:           300,
							ControllerKey: 100,
							UnitNumber:    ptr.To[int32](0),
							Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
									FileName: "[LocalDS_0] vm1/child-disk.vmdk",
								},
								Uuid: "child-disk-uuid-123",
								Parent: &vimtypes.VirtualDiskSeSparseBackingInfo{
									VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{},
									Uuid:                         "parent-disk-uuid",
								},
							},
						},
						CapacityInBytes: 1024 * 1024 * 1024,
					}

					ideController := &vimtypes.VirtualIDEController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: 100,
							},
							BusNumber: 0,
						},
					}

					// Set up snapshot layout with this disk
					moVM.LayoutEx = &vimtypes.VirtualMachineFileLayoutEx{
						Snapshot: []vimtypes.VirtualMachineFileLayoutExSnapshotLayout{
							{
								Disk: []vimtypes.VirtualMachineFileLayoutExDiskLayout{
									{
										Key: 300, // Same key as child disk
									},
								},
							},
						},
					}

					moVM.Config = &vimtypes.VirtualMachineConfigInfo{
						Hardware: vimtypes.VirtualHardware{
							Device: []vimtypes.BaseVirtualDevice{
								ideController,
								childDisk,
							},
						},
					}
				})

				It("should include child disks in snapshots", func() {
					err := r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
					Expect(err).To(Equal(unmanagedvolsfill.ErrPendingBackfill))
					Expect(vm.Spec.Volumes).To(HaveLen(1))

					vol := vmopv1util.FindByTargetID(
						vmopv1.VirtualControllerTypeIDE, 0, 0,
						vm.Spec.Volumes...)
					Expect(vol).ToNot(BeNil())
				})
			})
		})

		Context("controller types", func() {
			When("VM has NVME controller", func() {
				BeforeEach(func() {
					nvmeController := &vimtypes.VirtualNVMEController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: 100,
							},
							BusNumber: 0,
						},
					}

					disk := &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							Key:           300,
							ControllerKey: 100,
							UnitNumber:    ptr.To(int32(0)),
							Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
									FileName: "[LocalDS_0] vm1/nvme-disk.vmdk",
								},
								Uuid: "nvme-disk-uuid-123",
							},
						},
						CapacityInBytes: 1024 * 1024 * 1024,
					}

					moVM.Config = &vimtypes.VirtualMachineConfigInfo{
						Hardware: vimtypes.VirtualHardware{
							Device: []vimtypes.BaseVirtualDevice{
								nvmeController,
								disk,
							},
						},
					}
				})

				It("should detect NVME controller type", func() {
					err := r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
					Expect(err).To(Equal(unmanagedvolsfill.ErrPendingBackfill))

					vol := vmopv1util.FindByTargetID(
						vmopv1.VirtualControllerTypeNVME, 0, 0,
						vm.Spec.Volumes...)
					Expect(vol).ToNot(BeNil())
					Expect(vol.ControllerType).To(Equal(vmopv1.VirtualControllerTypeNVME))
					Expect(*vol.ControllerBusNumber).To(Equal(int32(0)))
				})
			})

			When("VM has SATA controller", func() {
				BeforeEach(func() {
					sataController := &vimtypes.VirtualSATAController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: 100,
							},
							BusNumber: 0,
						},
					}

					disk := &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							Key:           300,
							ControllerKey: 100,
							UnitNumber:    ptr.To(int32(0)),
							Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
									FileName: "[LocalDS_0] vm1/sata-disk.vmdk",
								},
								Uuid: "sata-disk-uuid-123",
							},
						},
						CapacityInBytes: 1024 * 1024 * 1024,
					}

					moVM.Config = &vimtypes.VirtualMachineConfigInfo{
						Hardware: vimtypes.VirtualHardware{
							Device: []vimtypes.BaseVirtualDevice{
								sataController,
								disk,
							},
						},
					}
				})

				It("should detect SATA controller type", func() {
					err := r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
					Expect(err).To(Equal(unmanagedvolsfill.ErrPendingBackfill))

					vol := vmopv1util.FindByTargetID(
						vmopv1.VirtualControllerTypeSATA, 0, 0,
						vm.Spec.Volumes...)
					Expect(vol).ToNot(BeNil())
					Expect(vol.ControllerType).To(Equal(vmopv1.VirtualControllerTypeSATA))
					Expect(*vol.ControllerBusNumber).To(Equal(int32(0)))
				})
			})
		})
	})
})
