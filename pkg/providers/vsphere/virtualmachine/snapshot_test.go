// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	"strings"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha5/common"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/test/builder"
	"github.com/vmware-tanzu/vm-operator/test/testutil"
)

func snapShotTests() {
	var (
		ctx        *builder.TestContextForVCSim
		vcVM       *object.VirtualMachine
		vmCtx      pkgctx.VirtualMachineContext
		vmSnapshot vmopv1.VirtualMachineSnapshot
		testConfig builder.VCSimTestConfig
		vm         *vmopv1.VirtualMachine
		err        error
	)

	BeforeEach(func() {
		testConfig = builder.VCSimTestConfig{}
		ctx = suite.NewTestContextForVCSim(testConfig)
	})

	JustBeforeEach(func() {
		vcVM, err = ctx.Finder.VirtualMachine(ctx, "DC0_C0_RP0_VM0")
		Expect(err).NotTo(HaveOccurred())

		vm = builder.DummyVirtualMachine()
		timeout, err := time.ParseDuration("1h35m")
		Expect(err).To(BeNil())
		vmSnapshot = *builder.DummyVirtualMachineSnapshot(vm.Namespace, "snap-1", vm.Name)
		vmSnapshot.Spec.Quiesce = &vmopv1.QuiesceSpec{
			Timeout: &metav1.Duration{Duration: timeout},
		}
		vmSnapshot.Spec.Memory = true
		vmSnapshot.Spec.Description = "This is a dummy-snap"

		vm.Spec.CurrentSnapshot = &vmopv1common.LocalObjectRef{
			APIVersion: vmSnapshot.APIVersion,
			Kind:       vmSnapshot.Kind,
			Name:       vmSnapshot.Name,
		}

		logger := testutil.GinkgoLogr(5)
		vmCtx = pkgctx.VirtualMachineContext{
			Context: logr.NewContext(ctx, logger),
			Logger:  logger.WithValues("vmName", vcVM.Name()),
			VM:      vm,
		}

		Expect(vcVM.Properties(vmCtx, vcVM.Reference(), vsphere.VMUpdatePropertiesSelector, &vmCtx.MoVM)).To(Succeed())
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		vcVM = nil
		vm = nil
	})

	Context("SnapshotVirtualMachine", func() {
		It("succeeds", func() {
			args := virtualmachine.SnapshotArgs{
				VMCtx:      vmCtx,
				VMSnapshot: vmSnapshot,
				VcVM:       vcVM,
			}

			snapMo, err := virtualmachine.SnapshotVirtualMachine(args)
			Expect(err).ToNot(HaveOccurred())
			Expect(snapMo).ToNot(BeNil())
			Expect(vmCtx.VM.Status.CurrentSnapshot).To(Equal(&vmopv1common.LocalObjectRef{
				APIVersion: vmSnapshot.APIVersion,
				Kind:       vmSnapshot.Kind,
				Name:       vmSnapshot.Name,
			}))

			moVM := mo.VirtualMachine{}
			Expect(vcVM.Properties(ctx, vcVM.Reference(), []string{"snapshot"}, &moVM)).To(Succeed())
			Expect(moVM.Snapshot).ToNot(BeNil())
			Expect(moVM.Snapshot.CurrentSnapshot).ToNot(BeNil())
			Expect(moVM.Snapshot.CurrentSnapshot.Value).To(Equal(snapMo.Value))
			Expect(moVM.Snapshot.RootSnapshotList).To(HaveLen(1))
			Expect(moVM.Snapshot.RootSnapshotList[0].Name).To(Equal(args.VMSnapshot.Name))

			// retry the same snapshot again, no-op (ie) no child snapshot created.
			snapMoDup, err := virtualmachine.SnapshotVirtualMachine(args)
			Expect(err).ToNot(HaveOccurred())
			Expect(snapMo).ToNot(BeNil())
			Expect(snapMo).To(Equal(snapMoDup))
			Expect(vmCtx.VM.Status.CurrentSnapshot).To(Equal(&vmopv1common.LocalObjectRef{
				APIVersion: vmSnapshot.APIVersion,
				Kind:       vmSnapshot.Kind,
				Name:       vmSnapshot.Name,
			}))

			Expect(vcVM.Properties(ctx, vcVM.Reference(), []string{"snapshot"}, &moVM)).To(Succeed())
			Expect(moVM.Snapshot).ToNot(BeNil())
			Expect(moVM.Snapshot.CurrentSnapshot).ToNot(BeNil())
			/// should point to the same one.
			Expect(moVM.Snapshot.CurrentSnapshot.Value).To(Equal(snapMoDup.Value))
			Expect(moVM.Snapshot.RootSnapshotList).To(HaveLen(1))
			Expect(moVM.Snapshot.RootSnapshotList[0].Name).To(Equal(args.VMSnapshot.Name))
			// zero child snapshots
			Expect(moVM.Snapshot.RootSnapshotList[0].ChildSnapshotList).To(HaveLen(0))

			// Create a new snapshot with a different name, child snapshot created.
			args.VMSnapshot.Name = "snap-2"
			snapMo2, err := virtualmachine.SnapshotVirtualMachine(args)
			Expect(err).ToNot(HaveOccurred())
			Expect(snapMo2).ToNot(BeNil())
			Expect(vmCtx.VM.Status.CurrentSnapshot).To(Equal(&vmopv1common.LocalObjectRef{
				APIVersion: vmSnapshot.APIVersion,
				Kind:       vmSnapshot.Kind,
				Name:       args.VMSnapshot.Name,
			}))

			Expect(vcVM.Properties(ctx, vcVM.Reference(), []string{"snapshot"}, &moVM)).To(Succeed())
			Expect(moVM.Snapshot).ToNot(BeNil())
			Expect(moVM.Snapshot.CurrentSnapshot).ToNot(BeNil())
			Expect(moVM.Snapshot.CurrentSnapshot.Value).To(Equal(snapMo2.Value))
			Expect(moVM.Snapshot.RootSnapshotList).To(HaveLen(1))
			Expect(moVM.Snapshot.RootSnapshotList[0].Name).To(Equal("snap-1"))
			Expect(moVM.Snapshot.RootSnapshotList[0].ChildSnapshotList).To(HaveLen(1))
			Expect(moVM.Snapshot.RootSnapshotList[0].ChildSnapshotList[0].Name).To(Equal(args.VMSnapshot.Name))
		})
	})

	Context("CreateSnapshot", func() {
		It("succeeds", func() {
			args := virtualmachine.SnapshotArgs{
				VMCtx:      vmCtx,
				VMSnapshot: vmSnapshot,
				VcVM:       vcVM,
			}

			snapMo, err := virtualmachine.CreateSnapshot(args)
			Expect(err).To(BeNil())
			Expect(snapMo).ToNot(BeNil())
			moVM := mo.VirtualMachine{}
			Expect(vcVM.Properties(ctx, vcVM.Reference(), []string{"snapshot"}, &moVM)).To(Succeed())
			Expect(moVM.Snapshot).ToNot(BeNil())
			Expect(moVM.Snapshot.CurrentSnapshot).ToNot(BeNil())
			Expect(moVM.Snapshot.CurrentSnapshot.Value).To(Equal(snapMo.Value))
			Expect(moVM.Snapshot.RootSnapshotList).To(HaveLen(1))
			Expect(moVM.Snapshot.RootSnapshotList[0].Name).To(Equal("snap-1"))
		})
	})

	Context("DeleteSnapshot", func() {
		JustBeforeEach(func() {
			args := virtualmachine.SnapshotArgs{
				VMCtx:      vmCtx,
				VMSnapshot: vmSnapshot,
				VcVM:       vcVM,
			}
			snapMo, err := virtualmachine.CreateSnapshot(args)
			Expect(err).To(BeNil())
			Expect(snapMo).ToNot(BeNil())
			moVM := mo.VirtualMachine{}
			Expect(vcVM.Properties(ctx, vcVM.Reference(), []string{"snapshot"}, &moVM)).To(Succeed())
			Expect(moVM.Snapshot).ToNot(BeNil())
		})

		It("succeeds", func() {
			deleteArgs := virtualmachine.SnapshotArgs{
				VMCtx:      vmCtx,
				VMSnapshot: vmSnapshot,
				VcVM:       vcVM,
			}

			Expect(virtualmachine.DeleteSnapshot(deleteArgs)).To(Succeed())
			moVM := mo.VirtualMachine{}
			Expect(vcVM.Properties(ctx, vcVM.Reference(), []string{"snapshot"}, &moVM)).To(Succeed())
			Expect(moVM.Snapshot).To(BeNil())
		})

		When("no snapshot for the VM", func() {
			JustBeforeEach(func() {
				deleteArgs := virtualmachine.SnapshotArgs{
					VMCtx:      vmCtx,
					VMSnapshot: vmSnapshot,
					VcVM:       vcVM,
				}
				Expect(virtualmachine.DeleteSnapshot(deleteArgs)).To(Succeed())
			})

			It("returns error", func() {
				deleteArgs := virtualmachine.SnapshotArgs{
					VMCtx:      vmCtx,
					VMSnapshot: vmSnapshot,
					VcVM:       vcVM,
				}
				Expect(virtualmachine.DeleteSnapshot(deleteArgs)).To(MatchError(virtualmachine.ErrSnapshotNotFound))
			})
		})

		When("snapshot not found", func() {
			JustBeforeEach(func() {
				By("create a new snapshot on the VM")
				vmSnapshot2 := *builder.DummyVirtualMachineSnapshot(vm.Namespace, "snap-2", vm.Name)
				args := virtualmachine.SnapshotArgs{
					VMCtx:      vmCtx,
					VMSnapshot: vmSnapshot2,
					VcVM:       vcVM,
				}
				snapMo, err := virtualmachine.CreateSnapshot(args)
				Expect(err).To(BeNil())
				Expect(snapMo).ToNot(BeNil())
				moVM := mo.VirtualMachine{}
				Expect(vcVM.Properties(ctx, vcVM.Reference(), []string{"snapshot"}, &moVM)).To(Succeed())
				Expect(moVM.Snapshot).ToNot(BeNil())
				Expect(moVM.Snapshot.RootSnapshotList).To(HaveLen(1))
				Expect(moVM.Snapshot.RootSnapshotList[0].ChildSnapshotList).To(HaveLen(1))

				vmSnapshot = *builder.DummyVirtualMachineSnapshot(vm.Namespace, "snap-1", vm.Name)
				deleteArgs := virtualmachine.SnapshotArgs{
					VMCtx:      vmCtx,
					VMSnapshot: vmSnapshot,
					VcVM:       vcVM,
				}
				Expect(virtualmachine.DeleteSnapshot(deleteArgs)).To(Succeed())
				moVM = mo.VirtualMachine{}
				Expect(vcVM.Properties(ctx, vcVM.Reference(), []string{"snapshot"}, &moVM)).To(Succeed())
				Expect(moVM.Snapshot).NotTo(BeNil())
			})

			It("returns error", func() {
				vmSnapshot = *builder.DummyVirtualMachineSnapshot(vm.Namespace, "snap-1", vm.Name)
				deleteArgs := virtualmachine.SnapshotArgs{
					VMCtx:      vmCtx,
					VMSnapshot: vmSnapshot,
					VcVM:       vcVM,
				}
				Expect(virtualmachine.DeleteSnapshot(deleteArgs)).To(MatchError(virtualmachine.ErrSnapshotNotFound))
			})
		})
	})

	Context("GetSnapshotSize", func() {
		Context("With VCSim", func() {
			var moVM mo.VirtualMachine
			var sum int64
			JustBeforeEach(func() {
				args := virtualmachine.SnapshotArgs{
					VMCtx:      vmCtx,
					VMSnapshot: vmSnapshot,
					VcVM:       vcVM,
				}
				snapMo, err := virtualmachine.CreateSnapshot(args)
				Expect(err).To(BeNil())
				Expect(snapMo).ToNot(BeNil())
				Expect(vcVM.Properties(ctx, vcVM.Reference(), []string{"snapshot", "layoutEx", "config.hardware.device"}, &moVM)).To(Succeed())
				Expect(moVM.Snapshot).ToNot(BeNil())
				vmCtx.MoVM = moVM

				sum = 0
				for _, file := range moVM.LayoutEx.File {
					if strings.HasSuffix(file.Name, ".vmdk") || strings.HasSuffix(file.Name, ".vmsn") || strings.HasSuffix(file.Name, ".vmem") {
						sum += file.Size
					}
				}
			})

			It("succeeds", func() {
				Expect(virtualmachine.GetSnapshotSize(vmCtx, moVM.Snapshot.CurrentSnapshot)).To(Equal(sum))
			})

			When("the snapshot is nil", func() {
				It("returns 0", func() {
					size := virtualmachine.GetSnapshotSize(vmCtx, nil)
					Expect(size).To(Equal(int64(0)))
				})
			})

			When("the moVM.layoutEx is nil", func() {
				It("returns 0", func() {
					vmCtx.MoVM.LayoutEx = nil
					size := virtualmachine.GetSnapshotSize(vmCtx, moVM.Snapshot.CurrentSnapshot)
					Expect(size).To(Equal(int64(0)))
				})
			})

			When("the moVM.config.hardware.device is empty", func() {
				It("returns snapshot size", func() {
					vmCtx.MoVM.Config.Hardware.Device = nil
					Expect(virtualmachine.GetSnapshotSize(vmCtx, moVM.Snapshot.CurrentSnapshot)).To(Equal(sum))
				})
			})
		})

		Context("Mock VM with two snapshots, 1 disk and 1 FCD", func() {
			const oneGiBInBytes = 1 /* B */ * 1024 /* KiB */ * 1024 /* MiB */ * 1024 /* GiB */
			var snapshot1, snapshot2 vimtypes.ManagedObjectReference
			JustBeforeEach(func() {
				vmCtx.MoVM.Config = &vimtypes.VirtualMachineConfigInfo{
					Hardware: vimtypes.VirtualHardware{
						Device: []vimtypes.BaseVirtualDevice{
							// classic
							&vimtypes.VirtualDisk{
								VirtualDevice: vimtypes.VirtualDevice{
									Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
										VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
											FileName: "[datastore] vm/my-disk-101.vmdk",
										},
										Uuid: "101",
									},
									Key: 100,
								},
								CapacityInBytes: 1 * oneGiBInBytes,
							},
							// managed
							&vimtypes.VirtualDisk{
								VirtualDevice: vimtypes.VirtualDevice{
									Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{
										VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
											FileName: "[datastore] vm/my-disk-105.vmdk",
										},
										Uuid: "105",
										KeyId: &vimtypes.CryptoKeyId{
											KeyId: "my-key-id",
											ProviderId: &vimtypes.KeyProviderId{
												Id: "my-provider-id",
											},
										},
									},
									Key: 101,
								},
								CapacityInBytes: 5 * oneGiBInBytes,
								VDiskId: &vimtypes.ID{
									Id: "my-fcd-1",
								},
							},
						},
					},
				}
				vmCtx.MoVM.LayoutEx = &vimtypes.VirtualMachineFileLayoutEx{
					Disk: []vimtypes.VirtualMachineFileLayoutExDiskLayout{
						{
							Key: 100,
							Chain: []vimtypes.VirtualMachineFileLayoutExDiskUnit{
								{
									FileKey: []int32{1, 2},
								},
								{
									FileKey: []int32{3, 4},
								},
								{
									FileKey: []int32{5, 6},
								},
							},
						},
						{
							Key: 101,
							Chain: []vimtypes.VirtualMachineFileLayoutExDiskUnit{
								{
									FileKey: []int32{11, 12},
								},
								{
									FileKey: []int32{13, 14},
								},
								{
									FileKey: []int32{15, 16},
								},
							},
						},
					},
					File: []vimtypes.VirtualMachineFileLayoutExFileInfo{
						{
							Key:        1,
							Size:       500,
							UniqueSize: 500,
						},
						{
							Key:        2,
							Size:       2 * oneGiBInBytes,
							UniqueSize: 1 * oneGiBInBytes,
						},

						{
							Key:        3,
							Size:       500,
							UniqueSize: 500,
						},
						{
							Key:        4,
							Size:       0.5 * oneGiBInBytes,
							UniqueSize: 0.25 * oneGiBInBytes,
						},

						{
							Key:        5,
							Size:       500,
							UniqueSize: 500,
						},
						{
							Key:        6,
							Size:       1 * oneGiBInBytes,
							UniqueSize: 0.5 * oneGiBInBytes,
						},

						{
							Key:        11,
							Size:       500,
							UniqueSize: 500,
						},
						{
							Key:        12,
							Size:       2 * oneGiBInBytes,
							UniqueSize: 1 * oneGiBInBytes,
						},
						{
							Key:        13,
							Size:       500,
							UniqueSize: 500,
						},
						{
							Key:        14,
							Size:       0.5 * oneGiBInBytes,
							UniqueSize: 0.25 * oneGiBInBytes,
						},
						{
							Key:        15,
							Size:       500,
							UniqueSize: 500,
						},
						{
							Key:        16,
							Size:       1 * oneGiBInBytes,
							UniqueSize: 0.5 * oneGiBInBytes,
						},
						{
							Key:        17,
							Size:       500,
							UniqueSize: 500,
							Name:       ".vmem",
						},
						{
							Key:        18,
							Size:       2 * oneGiBInBytes,
							UniqueSize: 3 * oneGiBInBytes,
							Name:       ".vmsn",
						},
						{
							Key:        19,
							Size:       1 * oneGiBInBytes,
							UniqueSize: 0.5 * oneGiBInBytes,
							Name:       ".vmsn",
						},
					},
					Snapshot: []vimtypes.VirtualMachineFileLayoutExSnapshotLayout{
						{
							Key: vimtypes.ManagedObjectReference{
								Type:  "Snapshot",
								Value: "Snapshot-1",
							},
							MemoryKey: 17,
							Disk: []vimtypes.VirtualMachineFileLayoutExDiskLayout{
								{
									Key: 100,
									Chain: []vimtypes.VirtualMachineFileLayoutExDiskUnit{
										{
											FileKey: []int32{3, 4},
										},
									},
								},
								{
									Key: 101,
									Chain: []vimtypes.VirtualMachineFileLayoutExDiskUnit{
										{
											FileKey: []int32{13, 14},
										},
									},
								},
							},
							DataKey: 18,
						},
						{
							Key: vimtypes.ManagedObjectReference{
								Type:  "Snapshot",
								Value: "Snapshot-2",
							},
							MemoryKey: -1,
							Disk: []vimtypes.VirtualMachineFileLayoutExDiskLayout{
								{
									Key: 100,
									Chain: []vimtypes.VirtualMachineFileLayoutExDiskUnit{
										{
											FileKey: []int32{3, 4},
										},
										{
											FileKey: []int32{5, 6},
										},
									},
								},
								{
									Key: 101,
									Chain: []vimtypes.VirtualMachineFileLayoutExDiskUnit{
										{
											FileKey: []int32{13, 14},
										},
										{
											FileKey: []int32{15, 16},
										},
									},
								},
							},
							DataKey: 19,
						},
					},
				}

				snapshot1 = vimtypes.ManagedObjectReference{
					Type:  "Snapshot",
					Value: "Snapshot-1",
				}
				snapshot2 = vimtypes.ManagedObjectReference{
					Type:  "Snapshot",
					Value: "Snapshot-2",
				}
			})

			When("Get the snapshot size of Snapshot-1", func() {
				It("returns the size by adding size of file 3, 4, 17, 18", func() {
					Expect(virtualmachine.GetSnapshotSize(vmCtx, &snapshot1)).To(Equal(int64(0.5*oneGiBInBytes + 500 + 500 + 2*oneGiBInBytes)))
				})
			})

			When("Get the snapshot size of Snapshot-2", func() {
				It("returns the size by adding size of file 5, 6, 19", func() {
					Expect(virtualmachine.GetSnapshotSize(vmCtx, &snapshot2)).To(Equal(int64(1*oneGiBInBytes + 500 + 1*oneGiBInBytes)))
				})
			})
		})
	})

	Describe("GetParentSnapshot", func() {
		var childSnapshot *vmopv1.VirtualMachineSnapshot

		JustBeforeEach(func() {
			By("Creating parent snapshot")
			args := virtualmachine.SnapshotArgs{
				VMCtx:      vmCtx,
				VMSnapshot: vmSnapshot,
				VcVM:       vcVM,
			}
			snapMo, err := virtualmachine.CreateSnapshot(args)
			Expect(err).To(BeNil())
			Expect(snapMo).ToNot(BeNil())

			By("Creating child snapshot")
			childSnapshot = builder.DummyVirtualMachineSnapshot(vm.Namespace, "snap-2", vm.Name)
			args = virtualmachine.SnapshotArgs{
				VMCtx:      vmCtx,
				VMSnapshot: *childSnapshot,
				VcVM:       vcVM,
			}
			snapMo2, err := virtualmachine.CreateSnapshot(args)
			Expect(err).To(BeNil())
			Expect(snapMo2).ToNot(BeNil())
			moVM := mo.VirtualMachine{}
			Expect(vcVM.Properties(ctx, vcVM.Reference(), []string{"snapshot"}, &moVM)).To(Succeed())
			Expect(moVM.Snapshot).ToNot(BeNil())
			vmCtx.MoVM = moVM
		})

		It("should return the parent snapshot of the child snapshot", func() {
			parent := virtualmachine.GetParentSnapshot(vmCtx, childSnapshot.Name)
			Expect(parent).ToNot(BeNil())
			Expect(parent.Name).To(Equal(vmSnapshot.Name))
		})

		When("there is no parent snapshot", func() {
			It("should return nil", func() {
				parent := virtualmachine.GetParentSnapshot(vmCtx, vmSnapshot.Name)
				Expect(parent).To(BeNil())
			})
		})

		When("snapshot doesn't exist", func() {
			It("should return nil", func() {
				childSnapshot.Name = ""
				parent := virtualmachine.GetParentSnapshot(vmCtx, childSnapshot.Name)
				Expect(parent).To(BeNil())
			})
		})
	})

}
