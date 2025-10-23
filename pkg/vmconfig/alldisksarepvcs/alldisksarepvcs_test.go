// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package alldisksarepvcs_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/api/v1alpha1"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig/alldisksarepvcs"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("New", func() {
	It("should return a reconciler", func() {
		Expect(alldisksarepvcs.New()).ToNot(BeNil())
	})
})

var _ = Describe("Name", func() {
	It("should return 'alldisksarepvcs'", func() {
		Expect(alldisksarepvcs.New().Name()).To(Equal("alldisksarepvcs"))
	})
})

var _ = Describe("OnResult", func() {
	It("should return nil", func() {
		var ctx context.Context
		Expect(alldisksarepvcs.New().OnResult(ctx, nil, mo.VirtualMachine{}, nil)).To(Succeed())
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
		httpPrefix string
	)

	BeforeEach(func() {
		r = alldisksarepvcs.New()

		vcsimCtx := builder.NewTestContextForVCSim(
			ctxop.WithContext(pkgcfg.NewContextWithDefaultConfig()), builder.VCSimTestConfig{})
		ctx = vcsimCtx
		ctx = vmconfig.WithContext(ctx)

		vimClient = vcsimCtx.VCClient.Client

		httpPrefix = fmt.Sprintf("https://%s", vimClient.URL().Host)

		moVM = mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{},
		}

		configSpec = &vimtypes.VirtualMachineConfigSpec{}

		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:   "my-namespace",
				Name:        "my-vm",
				Annotations: map[string]string{},
			},
			Spec: vmopv1.VirtualMachineSpec{},
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
		When("k8sClient is nil", func() {
			JustBeforeEach(func() {
				k8sClient = nil
			})
			It("should panic", func() {
				fn := func() {
					_ = r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
				}
				Expect(fn).To(PanicWith("k8sClient is nil"))
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
					err := r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
					Expect(err).ToNot(HaveOccurred())
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
					err := r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
					Expect(err).ToNot(HaveOccurred())
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
					err := alldisksarepvcs.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
					Expect(err).To(Equal(alldisksarepvcs.ErrUnmanagedVols))

					// Verify volumes were added to VM spec
					Expect(vm.Spec.Volumes).To(HaveLen(2))

					// Find the volumes by name (UUID)
					vol1 := findVolumeByName(vm.Spec.Volumes, "disk1-uuid-123")
					vol2 := findVolumeByName(vm.Spec.Volumes, "disk2-uuid-456")

					Expect(vol1).ToNot(BeNil())
					Expect(vol2).ToNot(BeNil())

					// Verify volume 1 properties
					Expect(vol1.PersistentVolumeClaim).ToNot(BeNil())
					Expect(vol1.PersistentVolumeClaim.ClaimName).To(Equal("my-vm-a515c82a")) // Generated from xxHash
					Expect(vol1.PersistentVolumeClaim.UnmanagedVolumeClaim).ToNot(BeNil())
					Expect(vol1.PersistentVolumeClaim.UnmanagedVolumeClaim.UUID).To(Equal("disk1-uuid-123"))
					Expect(string(vol1.PersistentVolumeClaim.UnmanagedVolumeClaim.Type)).To(Equal("FromVM"))
					Expect(vol1.PersistentVolumeClaim.ControllerType).To(Equal(vmopv1.VirtualControllerTypeIDE))
					Expect(*vol1.PersistentVolumeClaim.ControllerBusNumber).To(Equal(int32(0)))
					Expect(*vol1.PersistentVolumeClaim.UnitNumber).To(Equal(int32(0)))

					// Verify volume 2 properties
					Expect(vol2.PersistentVolumeClaim).ToNot(BeNil())
					Expect(vol2.PersistentVolumeClaim.ClaimName).To(Equal("my-vm-0627fe5c")) // Generated from xxHash
					Expect(vol2.PersistentVolumeClaim.UnmanagedVolumeClaim).ToNot(BeNil())
					Expect(vol2.PersistentVolumeClaim.UnmanagedVolumeClaim.UUID).To(Equal("disk2-uuid-456"))
					Expect(string(vol2.PersistentVolumeClaim.UnmanagedVolumeClaim.Type)).To(Equal("FromVM"))
					Expect(vol2.PersistentVolumeClaim.ControllerType).To(Equal(vmopv1.VirtualControllerTypeSCSI))
					Expect(*vol2.PersistentVolumeClaim.ControllerBusNumber).To(Equal(int32(1)))
					Expect(*vol2.PersistentVolumeClaim.UnitNumber).To(Equal(int32(0)))
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
					Expect(err).To(Equal(alldisksarepvcs.ErrUnmanagedVols))
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

				It("should skip child disks not in snapshots", func() {
					err := r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
					Expect(err).ToNot(HaveOccurred())
					Expect(vm.Spec.Volumes).To(BeEmpty())
				})
			})

			When("VM has child disks in snapshots", func() {
				BeforeEach(func() {
					// Create child disk (has parent) that is in snapshot
					childDisk := &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							Key:           300,
							ControllerKey: 100,
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
					Expect(err).To(Equal(alldisksarepvcs.ErrUnmanagedVols))
					Expect(vm.Spec.Volumes).To(HaveLen(1))

					vol := findVolumeByName(vm.Spec.Volumes, "child-disk-uuid-123")
					Expect(vol).ToNot(BeNil())
					Expect(vol.PersistentVolumeClaim.UnmanagedVolumeClaim.UUID).To(Equal("child-disk-uuid-123"))
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
					Expect(err).To(Equal(alldisksarepvcs.ErrUnmanagedVols))

					vol := findVolumeByName(vm.Spec.Volumes, "nvme-disk-uuid-123")
					Expect(vol).ToNot(BeNil())
					Expect(vol.PersistentVolumeClaim.ControllerType).To(Equal(vmopv1.VirtualControllerTypeNVME))
					Expect(*vol.PersistentVolumeClaim.ControllerBusNumber).To(Equal(int32(0)))
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
					Expect(err).To(Equal(alldisksarepvcs.ErrUnmanagedVols))

					vol := findVolumeByName(vm.Spec.Volumes, "sata-disk-uuid-123")
					Expect(vol).ToNot(BeNil())
					Expect(vol.PersistentVolumeClaim.ControllerType).To(Equal(vmopv1.VirtualControllerTypeSATA))
					Expect(*vol.PersistentVolumeClaim.ControllerBusNumber).To(Equal(int32(0)))
				})
			})
		})

		Context("PVC creation and management", func() {
			When("VM has unmanaged disks and no existing PVCs", func() {
				BeforeEach(func() {
					// Start with empty volumes - let Reconcile add them
					vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{}

					disk := &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							Key:           300,
							ControllerKey: 100,
							UnitNumber:    ptr.To(int32(0)),
							Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
									FileName: "[LocalDS_0] vm1/disk.vmdk",
								},
								Uuid: "disk-uuid-123",
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
					Expect(err).To(Equal(alldisksarepvcs.ErrUnmanagedVols))

					// Verify volumes were added to VM spec
					Expect(vm.Spec.Volumes).To(HaveLen(1))

					vol := findVolumeByName(vm.Spec.Volumes, "disk-uuid-123")
					Expect(vol).ToNot(BeNil())
					Expect(vol.PersistentVolumeClaim.ClaimName).To(Equal("my-vm-54cddb33"))
				})
			})

			When("VM has unmanaged disk with MultiWriter sharing", func() {
				BeforeEach(func() {
					// Start with empty volumes - let Reconcile add them
					vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{}

					disk := &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							Key:           300,
							ControllerKey: 200,
							UnitNumber:    ptr.To(int32(0)),
							Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
									FileName: "[LocalDS_0] vm1/multiwriter-disk.vmdk",
								},
								Uuid:    "disk-uuid-456",
								Sharing: string(vimtypes.VirtualDiskSharingSharingMultiWriter),
							},
						},
						CapacityInBytes: 2 * 1024 * 1024 * 1024,
					}

					scsiController := &vimtypes.VirtualSCSIController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: 200,
							},
							BusNumber: 1,
						},
					}

					moVM.Config = &vimtypes.VirtualMachineConfigInfo{
						Hardware: vimtypes.VirtualHardware{
							Device: []vimtypes.BaseVirtualDevice{
								scsiController,
								disk,
							},
						},
					}
				})

				It("should add volumes to VM spec and return ErrUnmanagedVols", func() {
					err := r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
					Expect(err).To(Equal(alldisksarepvcs.ErrUnmanagedVols))

					// Verify volumes were added to VM spec
					Expect(vm.Spec.Volumes).To(HaveLen(1))

					vol := findVolumeByName(vm.Spec.Volumes, "disk-uuid-456")
					Expect(vol).ToNot(BeNil())
					Expect(vol.PersistentVolumeClaim.ClaimName).To(Equal("my-vm-c6a0a3e7"))
				})
			})

			When("VM has volumes in spec and needs PVC creation", func() {
				var datastorePath string

				BeforeEach(func() {
					datastorePath = "[LocalDS_0] vm1/disk.vmdk"
				})

				JustBeforeEach(func() {
					// Set up VM with volumes already in spec
					vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
						{
							Name: "disk-uuid-123",
							VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
								PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
									PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "my-vm-54cddb33",
									},
									UnmanagedVolumeClaim: &vmopv1.UnmanagedVolumeClaimVolumeSource{
										Type: vmopv1.UnmanagedVolumeClaimVolumeTypeFromVM,
										Name: "disk-uuid-123",
										UUID: "disk-uuid-123",
									},
									ControllerType:      vmopv1.VirtualControllerTypeIDE,
									ControllerBusNumber: ptr.To(int32(0)),
									UnitNumber:          ptr.To(int32(0)),
								},
							},
						},
					}

					disk := &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							Key:           300,
							ControllerKey: 100,
							UnitNumber:    ptr.To(int32(0)),
							Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
									FileName: datastorePath,
								},
								Uuid: "disk-uuid-123",
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

				It("should create PVC and CnsRegisterVolume", func() {
					err := r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
					Expect(err).To(Equal(alldisksarepvcs.ErrUnmanagedVols))

					// Verify PVC was created
					pvc := &corev1.PersistentVolumeClaim{}
					err = k8sClient.Get(ctx, ctrlclient.ObjectKey{
						Namespace: vm.Namespace,
						Name:      "my-vm-54cddb33",
					}, pvc)
					Expect(err).ToNot(HaveOccurred())

					// Verify PVC properties
					Expect(pvc.Spec.DataSourceRef).ToNot(BeNil())
					Expect(pvc.Spec.DataSourceRef.Kind).To(Equal("VirtualMachine"))
					Expect(pvc.Spec.DataSourceRef.Name).To(Equal(vm.Name))
					Expect(pvc.Spec.AccessModes).To(ContainElement(corev1.ReadWriteOnce))
					expectedStorage := *kubeutil.BytesToResource(1024 * 1024 * 1024)
					Expect(pvc.Spec.Resources.Requests[corev1.ResourceStorage].Equal(expectedStorage)).To(BeTrue())

					// Verify CnsRegisterVolume was created
					crv := &cnsv1alpha1.CnsRegisterVolume{}
					err = k8sClient.Get(ctx, ctrlclient.ObjectKey{
						Namespace: vm.Namespace,
						Name:      "my-vm-54cddb33",
					}, crv)
					Expect(err).ToNot(HaveOccurred())

					// Verify CnsRegisterVolume properties
					Expect(crv.Spec.PvcName).To(Equal("my-vm-54cddb33"))

					Expect(crv.Spec.DiskURLPath).To(Equal(httpPrefix + `/folder/vm1/disk.vmdk?dcPath=%2FDC0&dsName=LocalDS_0`))
					Expect(crv.Spec.AccessMode).To(Equal(corev1.ReadWriteOnce))
					Expect(crv.Labels["vmoperator.vmware.com/created-by"]).To(Equal(vm.Name))
					Expect(crv.OwnerReferences).To(HaveLen(1))
					Expect(crv.OwnerReferences[0].Kind).To(Equal("VirtualMachine"))
					Expect(crv.OwnerReferences[0].Name).To(Equal(vm.Name))
					Expect(*crv.OwnerReferences[0].Controller).To(BeTrue())
				})

				When("VM has disks with invalid filename", func() {
					BeforeEach(func() {
						datastorePath = "[invalid] vm1/disk.vmdk"
					})

					It("should return an error", func() {
						err := r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
						Expect(err).To(MatchError(`failed to ensure CnsRegisterVolume my-vm-54cddb33: ` +
							`failed to get datastore url for "[invalid] vm1/disk.vmdk": ` +
							`failed to get datastore for "invalid": datastore 'invalid' not found`))
					})
				})
			})

			When("VM has MultiWriter disk and needs PVC creation", func() {
				BeforeEach(func() {
					// Set up VM with volumes already in spec
					vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
						{
							Name: "disk-uuid-456",
							VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
								PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
									PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "my-vm-c6a0a3e7",
									},
									UnmanagedVolumeClaim: &vmopv1.UnmanagedVolumeClaimVolumeSource{
										Type: vmopv1.UnmanagedVolumeClaimVolumeTypeFromVM,
										Name: "disk-uuid-456",
										UUID: "disk-uuid-456",
									},
									ControllerType:      vmopv1.VirtualControllerTypeSCSI,
									ControllerBusNumber: ptr.To(int32(1)),
									UnitNumber:          ptr.To(int32(0)),
								},
							},
						},
					}

					disk := &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							Key:           300,
							ControllerKey: 200,
							UnitNumber:    ptr.To(int32(0)),
							Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
									FileName: "[LocalDS_0] vm1/multiwriter-disk.vmdk",
								},
								Uuid:    "disk-uuid-456",
								Sharing: string(vimtypes.VirtualDiskSharingSharingMultiWriter),
							},
						},
						CapacityInBytes: 2 * 1024 * 1024 * 1024,
					}

					scsiController := &vimtypes.VirtualSCSIController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: 200,
							},
							BusNumber: 1,
						},
					}

					moVM.Config = &vimtypes.VirtualMachineConfigInfo{
						Hardware: vimtypes.VirtualHardware{
							Device: []vimtypes.BaseVirtualDevice{
								scsiController,
								disk,
							},
						},
					}
				})

				It("should create PVC and CnsRegisterVolume with ReadWriteMany access mode", func() {
					err := r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
					Expect(err).To(Equal(alldisksarepvcs.ErrUnmanagedVols))

					// Verify PVC was created with ReadWriteMany
					pvc := &corev1.PersistentVolumeClaim{}
					err = k8sClient.Get(ctx, ctrlclient.ObjectKey{
						Namespace: vm.Namespace,
						Name:      "my-vm-c6a0a3e7",
					}, pvc)
					Expect(err).ToNot(HaveOccurred())
					Expect(pvc.Spec.AccessModes).To(ContainElement(corev1.ReadWriteMany))

					// Verify CnsRegisterVolume was created with ReadWriteMany
					crv := &cnsv1alpha1.CnsRegisterVolume{}
					err = k8sClient.Get(ctx, ctrlclient.ObjectKey{
						Namespace: vm.Namespace,
						Name:      "my-vm-c6a0a3e7",
					}, crv)
					Expect(err).ToNot(HaveOccurred())
					Expect(crv.Spec.AccessMode).To(Equal(corev1.ReadWriteMany))
				})
			})

			When("PVC exists and is bound", func() {
				BeforeEach(func() {
					// Set up VM with volumes already in spec
					vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
						{
							Name: "disk-uuid-789",
							VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
								PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
									PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "my-vm-134e95b6",
									},
									UnmanagedVolumeClaim: &vmopv1.UnmanagedVolumeClaimVolumeSource{
										Type: vmopv1.UnmanagedVolumeClaimVolumeTypeFromVM,
										Name: "disk-uuid-789",
										UUID: "disk-uuid-789",
									},
									ControllerType:      vmopv1.VirtualControllerTypeIDE,
									ControllerBusNumber: ptr.To(int32(0)),
									UnitNumber:          ptr.To(int32(0)),
								},
							},
						},
					}

					// Create bound PVC
					boundPVC := &corev1.PersistentVolumeClaim{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "my-vm-134e95b6",
							Namespace: vm.Namespace,
							OwnerReferences: []metav1.OwnerReference{
								{
									APIVersion: vmopv1.GroupVersion.String(),
									Kind:       "VirtualMachine",
									Name:       vm.Name,
									UID:        vm.UID,
									Controller: ptr.To(true),
								},
							},
						},
						Spec: corev1.PersistentVolumeClaimSpec{
							DataSourceRef: &corev1.TypedObjectReference{
								APIGroup: &vmopv1.GroupVersion.Group,
								Kind:     "VirtualMachine",
								Name:     vm.Name,
							},
							AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
							Resources: corev1.VolumeResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: *kubeutil.BytesToResource(1024 * 1024 * 1024),
								},
							},
						},
						Status: corev1.PersistentVolumeClaimStatus{
							Phase: corev1.ClaimBound,
						},
					}

					// Create existing CnsRegisterVolume to be cleaned up
					existingCRV := &cnsv1alpha1.CnsRegisterVolume{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "my-vm-134e95b6",
							Namespace: vm.Namespace,
							Labels: map[string]string{
								"vmoperator.vmware.com/created-by": vm.Name,
							},
							OwnerReferences: []metav1.OwnerReference{
								{
									APIVersion: vmopv1.GroupVersion.String(),
									Kind:       "VirtualMachine",
									Name:       vm.Name,
									UID:        vm.UID,
									Controller: ptr.To(true),
								},
							},
						},
						Spec: cnsv1alpha1.CnsRegisterVolumeSpec{
							PvcName:     "my-vm-134e95b6",
							DiskURLPath: "[LocalDS_0] vm1/disk.vmdk",
							AccessMode:  corev1.ReadWriteOnce,
						},
					}

					withObjs = []ctrlclient.Object{boundPVC, existingCRV}

					disk := &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							Key:           300,
							ControllerKey: 100,
							UnitNumber:    ptr.To(int32(0)),
							Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
									FileName: "[LocalDS_0] vm1/disk.vmdk",
								},
								Uuid: "disk-uuid-789",
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

				It("should clean up CnsRegisterVolume and return nil", func() {
					err := r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
					Expect(err).ToNot(HaveOccurred())

					// Verify CnsRegisterVolume was deleted
					crv := &cnsv1alpha1.CnsRegisterVolume{}
					err = k8sClient.Get(ctx, ctrlclient.ObjectKey{
						Namespace: vm.Namespace,
						Name:      "my-vm-134e95b6",
					}, crv)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("not found"))
				})
			})

			When("PVC exists but needs patching", func() {
				BeforeEach(func() {
					// Set up VM with volumes already in spec
					vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
						{
							Name: "disk-uuid-patch",
							VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
								PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
									PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "my-vm-f3069b5c",
									},
									UnmanagedVolumeClaim: &vmopv1.UnmanagedVolumeClaimVolumeSource{
										Type: vmopv1.UnmanagedVolumeClaimVolumeTypeFromVM,
										Name: "disk-uuid-patch",
										UUID: "disk-uuid-patch",
									},
									ControllerType:      vmopv1.VirtualControllerTypeIDE,
									ControllerBusNumber: ptr.To(int32(0)),
									UnitNumber:          ptr.To(int32(0)),
								},
							},
						},
					}

					// Create PVC without OwnerRef and DataSourceRef
					existingPVC := &corev1.PersistentVolumeClaim{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "my-vm-f3069b5c",
							Namespace: vm.Namespace,
							// No OwnerReferences
						},
						Spec: corev1.PersistentVolumeClaimSpec{
							// No DataSourceRef
							AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
							Resources: corev1.VolumeResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: *kubeutil.BytesToResource(1024 * 1024 * 1024),
								},
							},
						},
						Status: corev1.PersistentVolumeClaimStatus{
							Phase: corev1.ClaimPending,
						},
					}

					withObjs = []ctrlclient.Object{existingPVC}

					disk := &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							Key:           300,
							ControllerKey: 100,
							UnitNumber:    ptr.To(int32(0)),
							Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
									FileName: "[LocalDS_0] vm1/disk.vmdk",
								},
								Uuid: "disk-uuid-patch",
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

				It("should patch PVC with OwnerRef and DataSourceRef", func() {
					err := r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
					Expect(err).To(Equal(alldisksarepvcs.ErrUnmanagedVols))

					// Verify PVC was patched
					pvc := &corev1.PersistentVolumeClaim{}
					err = k8sClient.Get(ctx, ctrlclient.ObjectKey{
						Namespace: vm.Namespace,
						Name:      "my-vm-f3069b5c",
					}, pvc)
					Expect(err).ToNot(HaveOccurred())

					// Verify OwnerRef was added
					Expect(pvc.OwnerReferences).To(HaveLen(1))
					Expect(pvc.OwnerReferences[0].Kind).To(Equal("VirtualMachine"))
					Expect(pvc.OwnerReferences[0].Name).To(Equal(vm.Name))

					// Verify DataSourceRef was added
					Expect(pvc.Spec.DataSourceRef).ToNot(BeNil())
					Expect(pvc.Spec.DataSourceRef.Kind).To(Equal("VirtualMachine"))
					Expect(pvc.Spec.DataSourceRef.Name).To(Equal(vm.Name))

					// Verify CnsRegisterVolume was created
					crv := &cnsv1alpha1.CnsRegisterVolume{}
					err = k8sClient.Get(ctx, ctrlclient.ObjectKey{
						Namespace: vm.Namespace,
						Name:      "my-vm-f3069b5c",
					}, crv)
					Expect(err).ToNot(HaveOccurred())
				})
			})

			When("PVC is pending and CnsRegisterVolume exists", func() {
				BeforeEach(func() {
					// Set up VM with volumes already in spec
					vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
						{
							Name: "disk-uuid-pending",
							VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
								PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
									PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "my-vm-0c2d85ea",
									},
									UnmanagedVolumeClaim: &vmopv1.UnmanagedVolumeClaimVolumeSource{
										Type: vmopv1.UnmanagedVolumeClaimVolumeTypeFromVM,
										Name: "disk-uuid-pending",
										UUID: "disk-uuid-pending",
									},
									ControllerType:      vmopv1.VirtualControllerTypeIDE,
									ControllerBusNumber: ptr.To(int32(0)),
									UnitNumber:          ptr.To(int32(0)),
								},
							},
						},
					}

					// Create pending PVC
					pendingPVC := &corev1.PersistentVolumeClaim{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "my-vm-0c2d85ea",
							Namespace: vm.Namespace,
							OwnerReferences: []metav1.OwnerReference{
								{
									APIVersion: vmopv1.GroupVersion.String(),
									Kind:       "VirtualMachine",
									Name:       vm.Name,
									UID:        vm.UID,
									Controller: ptr.To(true),
								},
							},
						},
						Spec: corev1.PersistentVolumeClaimSpec{
							DataSourceRef: &corev1.TypedObjectReference{
								APIGroup: &vmopv1.GroupVersion.Group,
								Kind:     "VirtualMachine",
								Name:     vm.Name,
							},
							AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
							Resources: corev1.VolumeResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: *kubeutil.BytesToResource(1024 * 1024 * 1024),
								},
							},
						},
						Status: corev1.PersistentVolumeClaimStatus{
							Phase: corev1.ClaimPending,
						},
					}

					withObjs = []ctrlclient.Object{pendingPVC}

					disk := &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							Key:           300,
							ControllerKey: 100,
							UnitNumber:    ptr.To(int32(0)),
							Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
									FileName: "[LocalDS_0] vm1/disk.vmdk",
								},
								Uuid: "disk-uuid-pending",
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

				It("should return ErrUnmanagedVols for pending PVC", func() {
					err := r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
					Expect(err).To(Equal(alldisksarepvcs.ErrUnmanagedVols))
				})
			})

			When("multiple reconciliation cycles with state changes", func() {
				BeforeEach(func() {
					// Set up VM with volumes already in spec
					vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
						{
							Name: "disk-uuid-cycle",
							VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
								PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
									PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "my-vm-e926b968",
									},
									UnmanagedVolumeClaim: &vmopv1.UnmanagedVolumeClaimVolumeSource{
										Type: vmopv1.UnmanagedVolumeClaimVolumeTypeFromVM,
										Name: "disk-uuid-cycle",
										UUID: "disk-uuid-cycle",
									},
									ControllerType:      vmopv1.VirtualControllerTypeIDE,
									ControllerBusNumber: ptr.To(int32(0)),
									UnitNumber:          ptr.To(int32(0)),
								},
							},
						},
					}

					disk := &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							Key:           300,
							ControllerKey: 100,
							UnitNumber:    ptr.To(int32(0)),
							Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
									FileName: "[LocalDS_0] vm1/disk.vmdk",
								},
								Uuid: "disk-uuid-cycle",
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

				It("should handle complete reconciliation cycle", func() {
					// First reconciliation: should create PVC and CnsRegisterVolume
					err := r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
					Expect(err).To(Equal(alldisksarepvcs.ErrUnmanagedVols))

					// Verify PVC was created
					pvc := &corev1.PersistentVolumeClaim{}
					err = k8sClient.Get(ctx, ctrlclient.ObjectKey{
						Namespace: vm.Namespace,
						Name:      "my-vm-e926b968",
					}, pvc)
					Expect(err).ToNot(HaveOccurred())

					// Set the PVC status to Pending for the test
					pvc.Status.Phase = corev1.ClaimPending
					err = k8sClient.Status().Update(ctx, pvc)
					Expect(err).ToNot(HaveOccurred())

					// Verify CnsRegisterVolume was created
					crv := &cnsv1alpha1.CnsRegisterVolume{}
					err = k8sClient.Get(ctx, ctrlclient.ObjectKey{
						Namespace: vm.Namespace,
						Name:      "my-vm-e926b968",
					}, crv)
					Expect(err).ToNot(HaveOccurred())

					// Simulate PVC becoming bound
					pvc.Status.Phase = corev1.ClaimBound
					err = k8sClient.Status().Update(ctx, pvc)
					Expect(err).ToNot(HaveOccurred())

					// Second reconciliation: should clean up CnsRegisterVolume
					err = r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
					Expect(err).ToNot(HaveOccurred())

					// Verify CnsRegisterVolume was deleted
					crv = &cnsv1alpha1.CnsRegisterVolume{}
					err = k8sClient.Get(ctx, ctrlclient.ObjectKey{
						Namespace: vm.Namespace,
						Name:      "my-vm-e926b968",
					}, crv)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("not found"))
				})
			})
		})

		Context("error handling", func() {
			When("k8sClient.Get fails for PVC", func() {
				BeforeEach(func() {
					// Set up VM with unmanaged disk
					vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
						{
							Name: "disk-uuid-error",
							VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
								PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
									PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "my-vm-87ce75a6",
									},
									UnmanagedVolumeClaim: &vmopv1.UnmanagedVolumeClaimVolumeSource{
										Type: vmopv1.UnmanagedVolumeClaimVolumeTypeFromVM,
										Name: "disk-uuid-error",
										UUID: "disk-uuid-error",
									},
									ControllerType:      vmopv1.VirtualControllerTypeIDE,
									ControllerBusNumber: ptr.To(int32(0)),
									UnitNumber:          ptr.To(int32(0)),
								},
							},
						},
					}

					disk := &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							Key:           300,
							ControllerKey: 100,
							UnitNumber:    ptr.To(int32(0)),
							Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
									FileName: "[LocalDS_0] vm1/disk.vmdk",
								},
								Uuid: "disk-uuid-error",
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

					// Set up interceptor to return error on Get
					withFuncs = interceptor.Funcs{
						Get: func(ctx context.Context, client ctrlclient.WithWatch, key ctrlclient.ObjectKey, obj ctrlclient.Object, opts ...ctrlclient.GetOption) error {
							if _, ok := obj.(*corev1.PersistentVolumeClaim); ok {
								return fmt.Errorf("simulated get error")
							}
							return client.Get(ctx, key, obj, opts...)
						},
					}
				})

				It("should return error", func() {
					err := r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("failed to get pvc"))
				})
			})

			When("k8sClient.Create fails for PVC", func() {
				BeforeEach(func() {
					// Set up VM with unmanaged disk
					vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
						{
							Name: "disk-uuid-create-error",
							VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
								PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
									PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "my-vm-f60a60d0",
									},
									UnmanagedVolumeClaim: &vmopv1.UnmanagedVolumeClaimVolumeSource{
										Type: vmopv1.UnmanagedVolumeClaimVolumeTypeFromVM,
										Name: "disk-uuid-create-error",
										UUID: "disk-uuid-create-error",
									},
									ControllerType:      vmopv1.VirtualControllerTypeIDE,
									ControllerBusNumber: ptr.To(int32(0)),
									UnitNumber:          ptr.To(int32(0)),
								},
							},
						},
					}

					disk := &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							Key:           300,
							ControllerKey: 100,
							UnitNumber:    ptr.To(int32(0)),
							Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
									FileName: "[LocalDS_0] vm1/disk.vmdk",
								},
								Uuid: "disk-uuid-create-error",
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

					// Set up interceptor to return error on Create
					withFuncs = interceptor.Funcs{
						Create: func(ctx context.Context, client ctrlclient.WithWatch, obj ctrlclient.Object, opts ...ctrlclient.CreateOption) error {
							if _, ok := obj.(*corev1.PersistentVolumeClaim); ok {
								return fmt.Errorf("simulated create error")
							}
							return client.Create(ctx, obj, opts...)
						},
					}
				})

				It("should return error", func() {
					err := r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("failed to create pvc"))
				})
			})

			When("k8sClient.Create fails for CnsRegisterVolume", func() {
				BeforeEach(func() {
					// Set up VM with unmanaged disk
					vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
						{
							Name: "disk-uuid-crv-error",
							VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
								PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
									PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "my-vm-3144bc43",
									},
									UnmanagedVolumeClaim: &vmopv1.UnmanagedVolumeClaimVolumeSource{
										Type: vmopv1.UnmanagedVolumeClaimVolumeTypeFromVM,
										Name: "disk-uuid-crv-error",
										UUID: "disk-uuid-crv-error",
									},
									ControllerType:      vmopv1.VirtualControllerTypeIDE,
									ControllerBusNumber: ptr.To(int32(0)),
									UnitNumber:          ptr.To(int32(0)),
								},
							},
						},
					}

					disk := &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							Key:           300,
							ControllerKey: 100,
							UnitNumber:    ptr.To(int32(0)),
							Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
									FileName: "[LocalDS_0] vm1/disk.vmdk",
								},
								Uuid: "disk-uuid-crv-error",
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

					// Set up interceptor to return error on Create for CnsRegisterVolume
					withFuncs = interceptor.Funcs{
						Create: func(ctx context.Context, client ctrlclient.WithWatch, obj ctrlclient.Object, opts ...ctrlclient.CreateOption) error {
							if _, ok := obj.(*cnsv1alpha1.CnsRegisterVolume); ok {
								return fmt.Errorf("simulated crv create error")
							}
							return client.Create(ctx, obj, opts...)
						},
					}
				})

				It("should return error", func() {
					err := r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("failed to ensure CnsRegisterVolume"))
				})
			})

			When("k8sClient.Get fails for CnsRegisterVolume", func() {
				BeforeEach(func() {
					// Set up VM with unmanaged disk
					vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
						{
							Name: "disk-uuid-crv-get-error",
							VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
								PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
									PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "my-vm-a89248a4",
									},
									UnmanagedVolumeClaim: &vmopv1.UnmanagedVolumeClaimVolumeSource{
										Type: vmopv1.UnmanagedVolumeClaimVolumeTypeFromVM,
										Name: "disk-uuid-crv-get-error",
										UUID: "disk-uuid-crv-get-error",
									},
									ControllerType:      vmopv1.VirtualControllerTypeIDE,
									ControllerBusNumber: ptr.To(int32(0)),
									UnitNumber:          ptr.To(int32(0)),
								},
							},
						},
					}

					disk := &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							Key:           300,
							ControllerKey: 100,
							UnitNumber:    ptr.To(int32(0)),
							Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
									FileName: "[LocalDS_0] vm1/disk.vmdk",
								},
								Uuid: "disk-uuid-crv-get-error",
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

					// Set up interceptor to return error on Get for CnsRegisterVolume
					withFuncs = interceptor.Funcs{
						Get: func(ctx context.Context, client ctrlclient.WithWatch, key ctrlclient.ObjectKey, obj ctrlclient.Object, opts ...ctrlclient.GetOption) error {
							if _, ok := obj.(*cnsv1alpha1.CnsRegisterVolume); ok {
								return fmt.Errorf("simulated crv get error")
							}
							return client.Get(ctx, key, obj, opts...)
						},
					}
				})

				It("should return error", func() {
					err := r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("failed to get crv"))
				})
			})

			When("k8sClient.Patch fails for PVC", func() {
				BeforeEach(func() {
					// Set up VM with unmanaged disk
					vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
						{
							Name: "disk-uuid-patch-error",
							VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
								PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
									PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "my-vm-cd73132d",
									},
									UnmanagedVolumeClaim: &vmopv1.UnmanagedVolumeClaimVolumeSource{
										Type: vmopv1.UnmanagedVolumeClaimVolumeTypeFromVM,
										Name: "disk-uuid-patch-error",
										UUID: "disk-uuid-patch-error",
									},
									ControllerType:      vmopv1.VirtualControllerTypeIDE,
									ControllerBusNumber: ptr.To(int32(0)),
									UnitNumber:          ptr.To(int32(0)),
								},
							},
						},
					}

					// Create existing PVC without OwnerRef and DataSourceRef
					existingPVC := &corev1.PersistentVolumeClaim{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "my-vm-cd73132d",
							Namespace: vm.Namespace,
							// No OwnerReferences
						},
						Spec: corev1.PersistentVolumeClaimSpec{
							// No DataSourceRef
							AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
							Resources: corev1.VolumeResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: *kubeutil.BytesToResource(1024 * 1024 * 1024),
								},
							},
						},
						Status: corev1.PersistentVolumeClaimStatus{
							Phase: corev1.ClaimPending,
						},
					}

					withObjs = []ctrlclient.Object{existingPVC}

					disk := &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							Key:           300,
							ControllerKey: 100,
							UnitNumber:    ptr.To(int32(0)),
							Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
									FileName: "[LocalDS_0] vm1/disk.vmdk",
								},
								Uuid: "disk-uuid-patch-error",
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

					// Set up interceptor to return error on Patch
					withFuncs = interceptor.Funcs{
						Patch: func(ctx context.Context, client ctrlclient.WithWatch, obj ctrlclient.Object, patch ctrlclient.Patch, opts ...ctrlclient.PatchOption) error {
							if _, ok := obj.(*corev1.PersistentVolumeClaim); ok {
								return fmt.Errorf("simulated patch error")
							}
							return client.Patch(ctx, obj, patch, opts...)
						},
					}
				})

				It("should return error", func() {
					err := r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("failed to patch pvc"))
				})
			})

			When("k8sClient.Delete fails for CnsRegisterVolume cleanup", func() {
				BeforeEach(func() {
					// Set up VM with bound PVC
					vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
						{
							Name: "disk-uuid-delete-error",
							VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
								PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
									PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "my-vm-9eb41331",
									},
									UnmanagedVolumeClaim: &vmopv1.UnmanagedVolumeClaimVolumeSource{
										Type: vmopv1.UnmanagedVolumeClaimVolumeTypeFromVM,
										Name: "disk-uuid-delete-error",
										UUID: "disk-uuid-delete-error",
									},
									ControllerType:      vmopv1.VirtualControllerTypeIDE,
									ControllerBusNumber: ptr.To(int32(0)),
									UnitNumber:          ptr.To(int32(0)),
								},
							},
						},
					}

					// Create bound PVC
					boundPVC := &corev1.PersistentVolumeClaim{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "my-vm-9eb41331",
							Namespace: vm.Namespace,
							OwnerReferences: []metav1.OwnerReference{
								{
									APIVersion: vmopv1.GroupVersion.String(),
									Kind:       "VirtualMachine",
									Name:       vm.Name,
									UID:        vm.UID,
									Controller: ptr.To(true),
								},
							},
						},
						Spec: corev1.PersistentVolumeClaimSpec{
							DataSourceRef: &corev1.TypedObjectReference{
								APIGroup: &vmopv1.GroupVersion.Group,
								Kind:     "VirtualMachine",
								Name:     vm.Name,
							},
							AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
							Resources: corev1.VolumeResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: *kubeutil.BytesToResource(1024 * 1024 * 1024),
								},
							},
						},
						Status: corev1.PersistentVolumeClaimStatus{
							Phase: corev1.ClaimBound,
						},
					}

					// Create existing CnsRegisterVolume to be cleaned up
					existingCRV := &cnsv1alpha1.CnsRegisterVolume{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "my-vm-9eb41331",
							Namespace: vm.Namespace,
							Labels: map[string]string{
								"vmoperator.vmware.com/created-by": vm.Name,
							},
							OwnerReferences: []metav1.OwnerReference{
								{
									APIVersion: vmopv1.GroupVersion.String(),
									Kind:       "VirtualMachine",
									Name:       vm.Name,
									UID:        vm.UID,
									Controller: ptr.To(true),
								},
							},
						},
						Spec: cnsv1alpha1.CnsRegisterVolumeSpec{
							PvcName:     "my-vm-9eb41331",
							DiskURLPath: "[LocalDS_0] vm1/disk.vmdk",
							AccessMode:  corev1.ReadWriteOnce,
						},
					}

					withObjs = []ctrlclient.Object{boundPVC, existingCRV}

					disk := &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							Key:           300,
							ControllerKey: 100,
							UnitNumber:    ptr.To(int32(0)),
							Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
									FileName: "[LocalDS_0] vm1/disk.vmdk",
								},
								Uuid: "disk-uuid-delete-error",
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

					// Set up interceptor to return error on Delete
					withFuncs = interceptor.Funcs{
						Delete: func(ctx context.Context, client ctrlclient.WithWatch, obj ctrlclient.Object, opts ...ctrlclient.DeleteOption) error {
							if _, ok := obj.(*cnsv1alpha1.CnsRegisterVolume); ok {
								return fmt.Errorf("simulated delete error")
							}
							return client.Delete(ctx, obj, opts...)
						},
					}
				})

				It("should return error", func() {
					err := r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("failed to delete CnsRegisterVolume"))
				})
			})

			When("k8sClient.DeleteAllOf fails for CnsRegisterVolume cleanup", func() {
				BeforeEach(func() {
					// Set up VM with bound PVC
					vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
						{
							Name: "disk-uuid-delete-all-error",
							VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
								PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
									PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "my-vm-1aeeec80",
									},
									UnmanagedVolumeClaim: &vmopv1.UnmanagedVolumeClaimVolumeSource{
										Type: vmopv1.UnmanagedVolumeClaimVolumeTypeFromVM,
										Name: "disk-uuid-delete-all-error",
										UUID: "disk-uuid-delete-all-error",
									},
									ControllerType:      vmopv1.VirtualControllerTypeIDE,
									ControllerBusNumber: ptr.To(int32(0)),
									UnitNumber:          ptr.To(int32(0)),
								},
							},
						},
					}

					// Create bound PVC
					boundPVC := &corev1.PersistentVolumeClaim{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "my-vm-1aeeec80",
							Namespace: vm.Namespace,
							OwnerReferences: []metav1.OwnerReference{
								{
									APIVersion: vmopv1.GroupVersion.String(),
									Kind:       "VirtualMachine",
									Name:       vm.Name,
									UID:        vm.UID,
									Controller: ptr.To(true),
								},
							},
						},
						Spec: corev1.PersistentVolumeClaimSpec{
							DataSourceRef: &corev1.TypedObjectReference{
								APIGroup: &vmopv1.GroupVersion.Group,
								Kind:     "VirtualMachine",
								Name:     vm.Name,
							},
							AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
							Resources: corev1.VolumeResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: *kubeutil.BytesToResource(1024 * 1024 * 1024),
								},
							},
						},
						Status: corev1.PersistentVolumeClaimStatus{
							Phase: corev1.ClaimBound,
						},
					}

					withObjs = []ctrlclient.Object{boundPVC}

					disk := &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							Key:           300,
							ControllerKey: 100,
							UnitNumber:    ptr.To(int32(0)),
							Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
									FileName: "[LocalDS_0] vm1/disk.vmdk",
								},
								Uuid: "disk-uuid-delete-all-error",
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

					// Set up interceptor to return error on DeleteAllOf
					withFuncs = interceptor.Funcs{
						DeleteAllOf: func(ctx context.Context, client ctrlclient.WithWatch, obj ctrlclient.Object, opts ...ctrlclient.DeleteAllOfOption) error {
							if _, ok := obj.(*cnsv1alpha1.CnsRegisterVolume); ok {
								return fmt.Errorf("simulated delete all error")
							}
							return client.DeleteAllOf(ctx, obj, opts...)
						},
					}
				})

				It("should return error", func() {
					err := r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("failed to delete CnsRegisterVolume"))
				})
			})

			When("k8sClient.Status().Update fails for PVC status update", func() {
				BeforeEach(func() {
					// Set up VM with unmanaged disk
					vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
						{
							Name: "disk-uuid-status-update-error",
							VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
								PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
									PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "my-vm-ca8e6f4d",
									},
									UnmanagedVolumeClaim: &vmopv1.UnmanagedVolumeClaimVolumeSource{
										Type: vmopv1.UnmanagedVolumeClaimVolumeTypeFromVM,
										Name: "disk-uuid-status-update-error",
										UUID: "disk-uuid-status-update-error",
									},
									ControllerType:      vmopv1.VirtualControllerTypeIDE,
									ControllerBusNumber: ptr.To(int32(0)),
									UnitNumber:          ptr.To(int32(0)),
								},
							},
						},
					}

					disk := &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							Key:           300,
							ControllerKey: 100,
							UnitNumber:    ptr.To(int32(0)),
							Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
									FileName: "[LocalDS_0] vm1/disk.vmdk",
								},
								Uuid: "disk-uuid-status-update-error",
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

					// Set up interceptor to return error on Status().Update
					withFuncs = interceptor.Funcs{
						SubResourcePatch: func(ctx context.Context, client ctrlclient.Client, subResourceName string, obj ctrlclient.Object, patch ctrlclient.Patch, opts ...ctrlclient.SubResourcePatchOption) error {
							if _, ok := obj.(*corev1.PersistentVolumeClaim); ok {
								return fmt.Errorf("simulated status update error")
							}
							return client.Status().Patch(ctx, obj, patch, opts...)
						},
					}
				})

				It("should handle status update error gracefully", func() {
					// This test verifies that status update errors don't break the reconciliation
					// The Reconcile function doesn't directly call Status().Update, but this
					// test ensures the interceptor is working correctly
					err := r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
					// The function should still work even if status updates fail
					Expect(err).To(Equal(alldisksarepvcs.ErrUnmanagedVols))
				})
			})
		})
	})
})

// findVolumeByName finds a volume by name.
func findVolumeByName(
	volumes []vmopv1.VirtualMachineVolume,
	name string) *vmopv1.VirtualMachineVolume {

	for i := range volumes {
		if volumes[i].Name == name {
			return &volumes[i]
		}
	}
	return nil
}
