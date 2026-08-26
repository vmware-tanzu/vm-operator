// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package snapshotdisk_test

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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig"
	vmconfsnapshotdisk "github.com/vmware-tanzu/vm-operator/pkg/vmconfig/snapshotdisk"
)

var _ = Describe("New", func() {
	It("returns a non-nil Reconciler", func() {
		Expect(vmconfsnapshotdisk.New()).ToNot(BeNil())
	})
	It("has name 'snapshotdisk'", func() {
		Expect(vmconfsnapshotdisk.New().Name()).To(Equal("snapshotdisk"))
	})
})

var _ = Describe("OnResult", func() {
	It("is a no-op", func() {
		r := vmconfsnapshotdisk.New()
		Expect(r.OnResult(context.Background(), &vmopv1.VirtualMachine{}, mo.VirtualMachine{}, nil)).To(Succeed())
	})
})

var _ = Describe("Reconcile", func() {
	var (
		ctx        context.Context
		k8sClient  ctrlclient.Client
		vimClient  *vim25.Client
		vm         *vmopv1.VirtualMachine
		moVM       mo.VirtualMachine
		configSpec *vimtypes.VirtualMachineConfigSpec
		r          vmconfig.Reconciler
	)

	BeforeEach(func() {
		r = vmconfsnapshotdisk.New()
		ctx = pkgcfg.NewContextWithDefaultConfig()
		cfg := pkgcfg.FromContext(ctx)
		cfg.Features.CSIBackupAPI = true
		ctx = pkgcfg.WithContext(ctx, cfg)

		scheme := runtime.NewScheme()
		Expect(vmopv1.AddToScheme(scheme)).To(Succeed())
		k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()

		vimClient = &vim25.Client{}
		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test-vm",
			},
			Status: vmopv1.VirtualMachineStatus{
				Hardware: &vmopv1.VirtualMachineHardwareStatus{
					Controllers: []vmopv1.VirtualControllerStatus{
						{
							DeviceKey: 1000,
							Type:      vmopv1.VirtualControllerTypeSCSI,
							BusNumber: 0,
						},
					},
				},
			},
		}
		moVM = mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{},
		}
		configSpec = &vimtypes.VirtualMachineConfigSpec{}
	})

	Context("preconditions", func() {
		It("panics when ctx is nil", func() {
			Expect(func() {
				_ = r.Reconcile(nil, k8sClient, vimClient, vm, moVM, configSpec) //nolint:staticcheck
			}).To(PanicWith("context is nil"))
		})

		It("panics when k8sClient is nil", func() {
			Expect(func() {
				_ = r.Reconcile(ctx, nil, vimClient, vm, moVM, configSpec)
			}).To(PanicWith("k8sClient is nil"))
		})

		It("panics when vimClient is nil", func() {
			Expect(func() {
				_ = r.Reconcile(ctx, k8sClient, nil, vm, moVM, configSpec)
			}).To(PanicWith("vimClient is nil"))
		})

		It("panics when vm is nil", func() {
			Expect(func() {
				_ = r.Reconcile(ctx, k8sClient, vimClient, nil, moVM, configSpec)
			}).To(PanicWith("vm is nil"))
		})

		It("panics when configSpec is nil", func() {
			Expect(func() {
				_ = r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, nil)
			}).To(PanicWith("configSpec is nil"))
		})
	})

	Context("when snapshot is not found", func() {
		BeforeEach(func() {
			vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
				{
					Name: "snap-vol",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						VirtualMachineSnapshotDisk: &vmopv1.VirtualMachineSnapshotDiskSpec{
							Name:   "non-existent-snap",
							DiskID: "disk-123",
						},
					},
				},
			}
		})

		It("sets error on volume status", func() {
			err := r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
			Expect(err).ToNot(HaveOccurred())
			Expect(vm.Status.Volumes).To(HaveLen(1))
			Expect(vm.Status.Volumes[0].Error).To(Equal("VirtualMachineSnapshot non-existent-snap not found"))
			Expect(vm.Status.Volumes[0].Attached).To(BeFalse())
		})
	})

	Context("when snapshot is not ready", func() {
		BeforeEach(func() {
			vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
				{
					Name: "snap-vol",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						VirtualMachineSnapshotDisk: &vmopv1.VirtualMachineSnapshotDiskSpec{
							Name:   "not-ready-snap",
							DiskID: "disk-123",
						},
					},
				},
			}

			snapshot := &vmopv1.VirtualMachineSnapshot{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "not-ready-snap",
				},
			}

			scheme := runtime.NewScheme()
			Expect(vmopv1.AddToScheme(scheme)).To(Succeed())
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(snapshot).Build()
		})

		It("sets error on volume status", func() {
			err := r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
			Expect(err).ToNot(HaveOccurred())
			Expect(vm.Status.Volumes).To(HaveLen(1))
			Expect(vm.Status.Volumes[0].Error).To(Equal("VirtualMachineSnapshot not-ready-snap is not ready"))
			Expect(vm.Status.Volumes[0].Attached).To(BeFalse())
		})
	})

	Context("when snapshot has no UniqueID", func() {
		BeforeEach(func() {
			vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
				{
					Name: "snap-vol",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						VirtualMachineSnapshotDisk: &vmopv1.VirtualMachineSnapshotDiskSpec{
							Name:   "no-unique-id-snap",
							DiskID: "disk-123",
						},
					},
				},
			}

			snapshot := &vmopv1.VirtualMachineSnapshot{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "no-unique-id-snap",
				},
				Status: vmopv1.VirtualMachineSnapshotStatus{
					Conditions: []metav1.Condition{
						{
							Type:   string(vmopv1.VirtualMachineSnapshotReadyCondition),
							Status: metav1.ConditionTrue,
						},
					},
				},
			}

			scheme := runtime.NewScheme()
			Expect(vmopv1.AddToScheme(scheme)).To(Succeed())
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(snapshot).Build()
		})

		It("sets error on volume status", func() {
			err := r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
			Expect(err).ToNot(HaveOccurred())
			Expect(vm.Status.Volumes).To(HaveLen(1))
			Expect(vm.Status.Volumes[0].Type).To(Equal(vmopv1.VolumeTypeVirtualMachineSnapshotDisk))
			Expect(vm.Status.Volumes[0].Error).To(Equal("VirtualMachineSnapshot no-unique-id-snap has no UniqueID"))
			Expect(vm.Status.Volumes[0].Attached).To(BeFalse())
		})
	})

	Context("when multiple volumes reference the same snapshot (avoid N+1 queries)", func() {
		var (
			fetchCallCount int
			cleanup        func()
		)

		BeforeEach(func() {
			fetchCallCount = 0

			vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
				{
					Name: "vol-1",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						VirtualMachineSnapshotDisk: &vmopv1.VirtualMachineSnapshotDiskSpec{
							Name:   "shared-snap",
							DiskID: "disk-uuid-1",
						},
					},
				},
				{
					Name: "vol-2",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						VirtualMachineSnapshotDisk: &vmopv1.VirtualMachineSnapshotDiskSpec{
							Name:   "shared-snap",
							DiskID: "disk-uuid-2",
						},
					},
				},
			}

			snapshot := &vmopv1.VirtualMachineSnapshot{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "shared-snap",
				},
				Status: vmopv1.VirtualMachineSnapshotStatus{
					UniqueID: "snapshot-100",
					Conditions: []metav1.Condition{
						{
							Type:   string(vmopv1.VirtualMachineSnapshotReadyCondition),
							Status: metav1.ConditionTrue,
						},
					},
				},
			}

			scheme := runtime.NewScheme()
			Expect(vmopv1.AddToScheme(scheme)).To(Succeed())
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(snapshot).Build()

			cleanup = vmconfsnapshotdisk.SetRetrieveSnapshotHardware(func(
				_ context.Context,
				_ *vim25.Client,
				snapRef vimtypes.ManagedObjectReference,
			) (*mo.VirtualMachineSnapshot, error) {
				fetchCallCount++
				Expect(snapRef.Value).To(Equal("snapshot-100"))
				return &mo.VirtualMachineSnapshot{
					Config: vimtypes.VirtualMachineConfigInfo{
						Hardware: vimtypes.VirtualHardware{
							Device: []vimtypes.BaseVirtualDevice{
								&vimtypes.VirtualDisk{
									VirtualDevice: vimtypes.VirtualDevice{
										Key: 2000,
										Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
											Uuid: "disk-uuid-1",
										},
									},
								},
								&vimtypes.VirtualDisk{
									VirtualDevice: vimtypes.VirtualDevice{
										Key: 2001,
										Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
											Uuid: "disk-uuid-2",
										},
									},
								},
							},
						},
					},
				}, nil
			})
		})

		AfterEach(func() {
			if cleanup != nil {
				cleanup()
			}
		})

		It("fetches snapshot hardware only once for multiple volumes from the same snapshot", func() {
			err := r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
			Expect(err).ToNot(HaveOccurred())

			// Exactly 1 call instead of 2 (resolves N+1 vSphere queries)
			Expect(fetchCallCount).To(Equal(1))

			// Both volumes should be attached
			Expect(vm.Status.Volumes).To(HaveLen(2))
			Expect(vm.Status.Volumes[0].Name).To(Equal("vol-1"))
			Expect(vm.Status.Volumes[0].Attached).To(BeTrue())
			Expect(vm.Status.Volumes[0].DiskUUID).To(Equal("disk-uuid-1"))
			Expect(vm.Status.Volumes[0].Error).To(BeEmpty())

			Expect(vm.Status.Volumes[1].Name).To(Equal("vol-2"))
			Expect(vm.Status.Volumes[1].Attached).To(BeTrue())
			Expect(vm.Status.Volumes[1].DiskUUID).To(Equal("disk-uuid-2"))
			Expect(vm.Status.Volumes[1].Error).To(BeEmpty())

			// Both disk device changes should be added with distinct keys
			var disks []*vimtypes.VirtualDisk
			for _, change := range configSpec.DeviceChange {
				devChange := change.GetVirtualDeviceConfigSpec()
				if disk, ok := devChange.Device.(*vimtypes.VirtualDisk); ok && devChange.Operation == vimtypes.VirtualDeviceConfigSpecOperationAdd {
					disks = append(disks, disk)
				}
			}
			Expect(disks).To(HaveLen(2))
			Expect(disks[0].Key).To(Equal(int32(-100)))
			Expect(disks[1].Key).To(Equal(int32(-101)))
		})

		It("breaks early on error when snapshot fetch fails", func() {
			cleanup() // reset previous mock
			cleanup = vmconfsnapshotdisk.SetRetrieveSnapshotHardware(func(
				_ context.Context,
				_ *vim25.Client,
				_ vimtypes.ManagedObjectReference,
			) (*mo.VirtualMachineSnapshot, error) {
				fetchCallCount++
				return nil, fmt.Errorf("simulated vCenter error")
			})

			err := r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("simulated vCenter error"))

			// Exactly 1 call because it breaks early on error
			Expect(fetchCallCount).To(Equal(1))
			Expect(vm.Status.Volumes).To(HaveLen(1))
			Expect(vm.Status.Volumes[0].Error).To(Equal("simulated vCenter error"))
			Expect(vm.Status.Volumes[0].Attached).To(BeFalse())
		})

		It("rolls back newly marked attached status of previous volumes when a subsequent volume fails", func() {
			vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
				{
					Name: "vol-1",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						VirtualMachineSnapshotDisk: &vmopv1.VirtualMachineSnapshotDiskSpec{
							Name:   "snap-a",
							DiskID: "disk-uuid-a",
						},
					},
				},
				{
					Name: "vol-2",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						VirtualMachineSnapshotDisk: &vmopv1.VirtualMachineSnapshotDiskSpec{
							Name:   "snap-b",
							DiskID: "disk-uuid-b",
						},
					},
				},
			}

			snapA := &vmopv1.VirtualMachineSnapshot{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "snap-a",
				},
				Status: vmopv1.VirtualMachineSnapshotStatus{
					UniqueID: "snapshot-100",
					Conditions: []metav1.Condition{
						{
							Type:   string(vmopv1.VirtualMachineSnapshotReadyCondition),
							Status: metav1.ConditionTrue,
						},
					},
				},
			}

			snapB := &vmopv1.VirtualMachineSnapshot{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "snap-b",
				},
				Status: vmopv1.VirtualMachineSnapshotStatus{
					UniqueID: "snapshot-200",
					Conditions: []metav1.Condition{
						{
							Type:   string(vmopv1.VirtualMachineSnapshotReadyCondition),
							Status: metav1.ConditionTrue,
						},
					},
				},
			}

			scheme := runtime.NewScheme()
			Expect(vmopv1.AddToScheme(scheme)).To(Succeed())
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(snapA, snapB).Build()

			cleanup()
			cleanup = vmconfsnapshotdisk.SetRetrieveSnapshotHardware(func(
				_ context.Context,
				_ *vim25.Client,
				snapRef vimtypes.ManagedObjectReference,
			) (*mo.VirtualMachineSnapshot, error) {
				if snapRef.Value == "snapshot-100" {
					return &mo.VirtualMachineSnapshot{
						Config: vimtypes.VirtualMachineConfigInfo{
							Hardware: vimtypes.VirtualHardware{
								Device: []vimtypes.BaseVirtualDevice{
									&vimtypes.VirtualDisk{
										VirtualDevice: vimtypes.VirtualDevice{
											Key: 2000,
											Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
												Uuid: "disk-uuid-a",
											},
										},
									},
								},
							},
						},
					}, nil
				}
				return nil, fmt.Errorf("connection reset for snap-b")
			})

			err := r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("connection reset for snap-b"))

			// vol-1 was processed first and added to configSpec, but because vol-2 failed with an error,
			// vol-1's newly attached status must be rolled back to avoid phantom attachment!
			for _, volStatus := range vm.Status.Volumes {
				if volStatus.Name == "vol-1" {
					Fail("vol-1 should have been rolled back from status since it was not originally in status")
				}
			}
			// vol-2 should have its failure recorded in status
			Expect(vm.Status.Volumes).To(HaveLen(1))
			Expect(vm.Status.Volumes[0].Name).To(Equal("vol-2"))
			Expect(vm.Status.Volumes[0].Error).To(ContainSubstring("connection reset for snap-b"))
			Expect(vm.Status.Volumes[0].Attached).To(BeFalse())
		})

		It("caches snapshot fetch errors for non-transient failures across multiple volumes", func() {
			snap := &vmopv1.VirtualMachineSnapshot{}
			Expect(k8sClient.Get(ctx, ctrlclient.ObjectKey{Namespace: "default", Name: "shared-snap"}, snap)).To(Succeed())
			snap.Status.Conditions = []metav1.Condition{
				{
					Type:   string(vmopv1.VirtualMachineSnapshotReadyCondition),
					Status: metav1.ConditionFalse,
				},
			}
			Expect(k8sClient.Update(ctx, snap)).To(Succeed())

			err := r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
			Expect(err).ToNot(HaveOccurred())

			// Both volumes should be marked failed in status with the cached error without breaking
			Expect(vm.Status.Volumes).To(HaveLen(2))
			Expect(vm.Status.Volumes[0].Error).To(ContainSubstring("is not ready"))
			Expect(vm.Status.Volumes[0].Attached).To(BeFalse())
			Expect(vm.Status.Volumes[1].Error).To(ContainSubstring("is not ready"))
			Expect(vm.Status.Volumes[1].Attached).To(BeFalse())
		})

		It("fetches snapshot hardware once per snapshot for volumes from different snapshots", func() {
			vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
				{
					Name: "vol-1",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						VirtualMachineSnapshotDisk: &vmopv1.VirtualMachineSnapshotDiskSpec{
							Name:   "snap-a",
							DiskID: "disk-uuid-a",
						},
					},
				},
				{
					Name: "vol-2",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						VirtualMachineSnapshotDisk: &vmopv1.VirtualMachineSnapshotDiskSpec{
							Name:   "snap-b",
							DiskID: "disk-uuid-b",
						},
					},
				},
			}

			snapA := &vmopv1.VirtualMachineSnapshot{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "snap-a"},
				Status: vmopv1.VirtualMachineSnapshotStatus{
					UniqueID: "snapshot-a",
					Conditions: []metav1.Condition{
						{Type: string(vmopv1.VirtualMachineSnapshotReadyCondition), Status: metav1.ConditionTrue},
					},
				},
			}
			snapB := &vmopv1.VirtualMachineSnapshot{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "snap-b"},
				Status: vmopv1.VirtualMachineSnapshotStatus{
					UniqueID: "snapshot-b",
					Conditions: []metav1.Condition{
						{Type: string(vmopv1.VirtualMachineSnapshotReadyCondition), Status: metav1.ConditionTrue},
					},
				},
			}

			scheme := runtime.NewScheme()
			Expect(vmopv1.AddToScheme(scheme)).To(Succeed())
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(snapA, snapB).Build()

			cleanup() // reset previous mock
			cleanup = vmconfsnapshotdisk.SetRetrieveSnapshotHardware(func(
				_ context.Context,
				_ *vim25.Client,
				snapRef vimtypes.ManagedObjectReference,
			) (*mo.VirtualMachineSnapshot, error) {
				fetchCallCount++
				diskUUID := "disk-uuid-a"
				if snapRef.Value == "snapshot-b" {
					diskUUID = "disk-uuid-b"
				}
				return &mo.VirtualMachineSnapshot{
					Config: vimtypes.VirtualMachineConfigInfo{
						Hardware: vimtypes.VirtualHardware{
							Device: []vimtypes.BaseVirtualDevice{
								&vimtypes.VirtualDisk{
									VirtualDevice: vimtypes.VirtualDevice{
										Key: 2000,
										Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
											Uuid: diskUUID,
										},
									},
								},
							},
						},
					},
				}, nil
			})

			err := r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
			Expect(err).ToNot(HaveOccurred())

			// Exactly 2 calls (one for each distinct snapshot)
			Expect(fetchCallCount).To(Equal(2))
			Expect(vm.Status.Volumes).To(HaveLen(2))
			Expect(vm.Status.Volumes[0].Attached).To(BeTrue())
			Expect(vm.Status.Volumes[1].Attached).To(BeTrue())
		})
	})
})

var _ = Describe("DetachSnapshotDisks", func() {
	It("adds remove DeviceChange for obsolete snapshot disks", func() {
		moVM := mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{
				Hardware: vimtypes.VirtualHardware{
					Device: []vimtypes.BaseVirtualDevice{
						&vimtypes.VirtualDisk{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: 2000,
								Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
									Uuid:     "disk-uuid-123",
									Parent:   &vimtypes.VirtualDiskFlatVer2BackingInfo{},
									DiskMode: string(vimtypes.VirtualDiskModeIndependent_nonpersistent),
								},
							},
						},
						&vimtypes.VirtualDisk{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: 2001,
								Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
									Uuid: "disk-uuid-456",
								},
							},
						},
					},
				},
			},
		}

		configSpec := &vimtypes.VirtualMachineConfigSpec{}
		vm := &vmopv1.VirtualMachine{
			Status: vmopv1.VirtualMachineStatus{
				Volumes: []vmopv1.VirtualMachineVolumeStatus{
					{
						Name:     "snap-vol-1",
						Type:     vmopv1.VolumeTypeVirtualMachineSnapshotDisk,
						Attached: true,
						DiskUUID: "disk-uuid-123",
					},
				},
			},
		}

		vmconfsnapshotdisk.DetachSnapshotDisks(moVM, vm, configSpec)

		Expect(configSpec.DeviceChange).To(HaveLen(1))
		change := configSpec.DeviceChange[0].GetVirtualDeviceConfigSpec()
		Expect(change.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationRemove))
		Expect(change.Device.GetVirtualDevice().Key).To(Equal(int32(2000)))
		Expect(vm.Status.Volumes[0].Attached).To(BeFalse())
	})

	It("removes obsolete snapshot disk without affecting regular disk with same UUID", func() {
		sameUUID := "shared-disk-uuid"
		unit0 := int32(0)
		unit1 := int32(1)
		moVM := mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{
				Hardware: vimtypes.VirtualHardware{
					Device: []vimtypes.BaseVirtualDevice{
						// Regular persistent disk (e.g. boot disk) on slot 0
						&vimtypes.VirtualDisk{
							VirtualDevice: vimtypes.VirtualDevice{
								Key:        1000,
								UnitNumber: &unit0,
								Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
									Uuid:     sameUUID,
									DiskMode: string(vimtypes.VirtualDiskModePersistent),
								},
							},
						},
						// Obsolete snapshot disk on slot 1 with same UUID
						&vimtypes.VirtualDisk{
							VirtualDevice: vimtypes.VirtualDevice{
								Key:        2000,
								UnitNumber: &unit1,
								Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
									Uuid:     sameUUID,
									Parent:   &vimtypes.VirtualDiskFlatVer2BackingInfo{},
									DiskMode: string(vimtypes.VirtualDiskModeIndependent_nonpersistent),
								},
							},
						},
					},
				},
			},
		}

		vm := &vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				Volumes: []vmopv1.VirtualMachineVolume{
					{
						Name: "boot-disk",
					},
					// Note: snapshot volume was removed from spec
				},
			},
			Status: vmopv1.VirtualMachineStatus{
				Volumes: []vmopv1.VirtualMachineVolumeStatus{
					{
						Name:       "boot-disk",
						Type:       vmopv1.VolumeTypeClassic,
						Attached:   true,
						DiskUUID:   sameUUID,
						UnitNumber: &unit0,
					},
					{
						Name:       "snapshot-disk",
						Type:       vmopv1.VolumeTypeVirtualMachineSnapshotDisk,
						Attached:   true,
						DiskUUID:   sameUUID,
						UnitNumber: &unit1,
					},
				},
			},
		}

		configSpec := &vimtypes.VirtualMachineConfigSpec{}

		vmconfsnapshotdisk.DetachSnapshotDisks(moVM, vm, configSpec)

		// Only the snapshot disk matching slot 1 should be removed
		Expect(configSpec.DeviceChange).To(HaveLen(1))
		change := configSpec.DeviceChange[0].GetVirtualDeviceConfigSpec()
		Expect(change.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationRemove))
		Expect(change.Device.GetVirtualDevice().Key).To(Equal(int32(2000)))

		// boot-disk status must remain Attached=true, only snapshot-disk becomes Attached=false
		Expect(vm.Status.Volumes[0].Name).To(Equal("boot-disk"))
		Expect(vm.Status.Volumes[0].Attached).To(BeTrue())
		Expect(vm.Status.Volumes[1].Name).To(Equal("snapshot-disk"))
		Expect(vm.Status.Volumes[1].Attached).To(BeFalse())
	})

	It("does not remove boot disk when snapshot disk is in spec with same UUID as boot disk", func() {
		sameUUID := "shared-boot-and-snap-uuid"
		unit0 := int32(0)
		unit1 := int32(1)
		moVM := mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{
				Hardware: vimtypes.VirtualHardware{
					Device: []vimtypes.BaseVirtualDevice{
						// Boot disk on slot 0
						&vimtypes.VirtualDisk{
							VirtualDevice: vimtypes.VirtualDevice{
								Key:        2000,
								UnitNumber: &unit0,
								Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
									Uuid:     sameUUID,
									DiskMode: string(vimtypes.VirtualDiskModePersistent),
								},
							},
						},
					},
				},
			},
		}

		vm := &vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				Volumes: []vmopv1.VirtualMachineVolume{
					{
						Name: "boot-disk",
					},
					{
						Name:       "snap-vol",
						UnitNumber: &unit1,
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							VirtualMachineSnapshotDisk: &vmopv1.VirtualMachineSnapshotDiskSpec{
								Name:   "snap-1",
								DiskID: sameUUID,
							},
						},
					},
				},
			},
			Status: vmopv1.VirtualMachineStatus{
				Volumes: []vmopv1.VirtualMachineVolumeStatus{
					{
						Name:       "boot-disk",
						Type:       vmopv1.VolumeTypeClassic,
						Attached:   true,
						DiskUUID:   sameUUID,
						UnitNumber: &unit0,
					},
					{
						Name:       "snap-vol",
						Type:       vmopv1.VolumeTypeVirtualMachineSnapshotDisk,
						Attached:   true,
						DiskUUID:   sameUUID,
						UnitNumber: &unit1,
					},
				},
			},
		}

		configSpec := &vimtypes.VirtualMachineConfigSpec{}

		vmconfsnapshotdisk.DetachSnapshotDisks(moVM, vm, configSpec)

		// Neither the boot disk nor the newly attached snapshot disk should be removed
		Expect(configSpec.DeviceChange).To(BeEmpty())
		Expect(vm.Status.Volumes[0].Attached).To(BeTrue())
		Expect(vm.Status.Volumes[1].Attached).To(BeTrue())
	})

	It("does not remove boot disk or treat snapshot volume as obsolete when status.Volumes unitNumber was nil", func() {
		sameUUID := "shared-boot-and-snap-uuid"
		unit0 := int32(0)
		unit1 := int32(1)
		moVM := mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{
				Hardware: vimtypes.VirtualHardware{
					Device: []vimtypes.BaseVirtualDevice{
						// Boot disk on slot 0
						&vimtypes.VirtualDisk{
							VirtualDevice: vimtypes.VirtualDevice{
								Key:        2000,
								UnitNumber: &unit0,
								Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
									Uuid:     sameUUID,
									DiskMode: string(vimtypes.VirtualDiskModePersistent),
								},
							},
						},
					},
				},
			},
		}

		vm := &vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				Volumes: []vmopv1.VirtualMachineVolume{
					{
						Name: "boot-disk",
					},
					{
						Name:       "snap-vol",
						UnitNumber: &unit1,
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							VirtualMachineSnapshotDisk: &vmopv1.VirtualMachineSnapshotDiskSpec{
								Name:   "snap-1",
								DiskID: sameUUID,
							},
						},
					},
				},
			},
			Status: vmopv1.VirtualMachineStatus{
				Volumes: []vmopv1.VirtualMachineVolumeStatus{
					{
						Name:       "boot-disk",
						Type:       vmopv1.VolumeTypeClassic,
						Attached:   true,
						DiskUUID:   sameUUID,
						UnitNumber: &unit0,
					},
					{
						Name:     "snap-vol",
						Type:     vmopv1.VolumeTypeVirtualMachineSnapshotDisk,
						Attached: true,
						DiskUUID: sameUUID,
						// UnitNumber is nil in status!
					},
				},
			},
		}

		configSpec := &vimtypes.VirtualMachineConfigSpec{}

		vmconfsnapshotdisk.DetachSnapshotDisks(moVM, vm, configSpec)

		// Neither the boot disk nor snap-vol should be removed!
		Expect(configSpec.DeviceChange).To(BeEmpty())
	})

	It("does not remove disk when volume is not in status.volumes", func() {
		moVM := mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{
				Hardware: vimtypes.VirtualHardware{
					Device: []vimtypes.BaseVirtualDevice{
						// Standalone independent non-persistent disk not tracked in status
						&vimtypes.VirtualDisk{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: 3000,
								Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
									Uuid:     "standalone-nonpersistent-uuid",
									DiskMode: string(vimtypes.VirtualDiskModeIndependent_nonpersistent),
								},
							},
						},
					},
				},
			},
		}

		configSpec := &vimtypes.VirtualMachineConfigSpec{}
		vm := &vmopv1.VirtualMachine{}

		vmconfsnapshotdisk.DetachSnapshotDisks(moVM, vm, configSpec)

		// The standalone disk must NOT be removed because it is not in status.volumes as a snapshot disk
		Expect(configSpec.DeviceChange).To(BeEmpty())
	})

	It("does not remove Managed volume (PVC) even if it has independent non-persistent mode and parent backing", func() {
		sameUUID := "pvc-disk-uuid"
		unit1 := int32(1)
		moVM := mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{
				Hardware: vimtypes.VirtualHardware{
					Device: []vimtypes.BaseVirtualDevice{
						&vimtypes.VirtualDisk{
							VirtualDevice: vimtypes.VirtualDevice{
								Key:        2000,
								UnitNumber: &unit1,
								Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
									Uuid:     sameUUID,
									Parent:   &vimtypes.VirtualDiskFlatVer2BackingInfo{},
									DiskMode: string(vimtypes.VirtualDiskModeIndependent_nonpersistent),
								},
							},
						},
					},
				},
			},
		}

		vm := &vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				Volumes: []vmopv1.VirtualMachineVolume{
					{
						Name: "my-pvc",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "my-claim",
								},
							},
						},
					},
				},
			},
			Status: vmopv1.VirtualMachineStatus{
				Volumes: []vmopv1.VirtualMachineVolumeStatus{
					{
						Name:       "my-pvc",
						Type:       vmopv1.VolumeTypeManaged,
						Attached:   true,
						DiskUUID:   sameUUID,
						UnitNumber: &unit1,
					},
				},
			},
		}

		configSpec := &vimtypes.VirtualMachineConfigSpec{}

		vmconfsnapshotdisk.DetachSnapshotDisks(moVM, vm, configSpec)

		// The Managed volume (PVC) must NOT be removed
		Expect(configSpec.DeviceChange).To(BeEmpty())
		Expect(vm.Status.Volumes[0].Attached).To(BeTrue())
	})

	It("correctly handles same DiskUUID across multiple volumes by matching UnitNumber via status", func() {
		sameUUID := "duplicate-disk-uuid"
		unit1 := int32(1)
		unit2 := int32(2)
		moVM := mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{
				Hardware: vimtypes.VirtualHardware{
					Device: []vimtypes.BaseVirtualDevice{
						&vimtypes.VirtualDisk{
							VirtualDevice: vimtypes.VirtualDevice{
								Key:        2001,
								UnitNumber: &unit1,
								Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
									Uuid:     sameUUID,
									Parent:   &vimtypes.VirtualDiskFlatVer2BackingInfo{},
									DiskMode: string(vimtypes.VirtualDiskModeIndependent_nonpersistent),
								},
							},
						},
						&vimtypes.VirtualDisk{
							VirtualDevice: vimtypes.VirtualDevice{
								Key:        2002,
								UnitNumber: &unit2,
								Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
									Uuid:     sameUUID,
									Parent:   &vimtypes.VirtualDiskFlatVer2BackingInfo{},
									DiskMode: string(vimtypes.VirtualDiskModeIndependent_nonpersistent),
								},
							},
						},
					},
				},
			},
		}

		vm := &vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				Volumes: []vmopv1.VirtualMachineVolume{
					// vol-1 was removed, only vol-2 remains. Note: vol-2 has no UnitNumber in spec.
					{
						Name: "vol-2",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							VirtualMachineSnapshotDisk: &vmopv1.VirtualMachineSnapshotDiskSpec{
								Name:   "snap-1",
								DiskID: sameUUID,
							},
						},
					},
				},
			},
			Status: vmopv1.VirtualMachineStatus{
				Volumes: []vmopv1.VirtualMachineVolumeStatus{
					{
						Name:       "vol-1",
						Type:       vmopv1.VolumeTypeVirtualMachineSnapshotDisk,
						Attached:   true,
						DiskUUID:   sameUUID,
						UnitNumber: &unit1,
					},
					{
						Name:       "vol-2",
						Type:       vmopv1.VolumeTypeVirtualMachineSnapshotDisk,
						Attached:   true,
						DiskUUID:   sameUUID,
						UnitNumber: &unit2,
					},
				},
			},
		}

		configSpec := &vimtypes.VirtualMachineConfigSpec{}

		vmconfsnapshotdisk.DetachSnapshotDisks(moVM, vm, configSpec)

		// Only disk 2001 (Unit 1, belonging to removed vol-1) should be removed!
		// Disk 2002 (Unit 2, belonging to vol-2) must NOT be removed!
		Expect(configSpec.DeviceChange).To(HaveLen(1))
		change := configSpec.DeviceChange[0].GetVirtualDeviceConfigSpec()
		Expect(change.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationRemove))
		Expect(change.Device.GetVirtualDevice().Key).To(Equal(int32(2001)))

		// vol-1 status becomes Attached=false, vol-2 status remains Attached=true
		Expect(vm.Status.Volumes[0].Name).To(Equal("vol-1"))
		Expect(vm.Status.Volumes[0].Attached).To(BeFalse())
		Expect(vm.Status.Volumes[1].Name).To(Equal("vol-2"))
		Expect(vm.Status.Volumes[1].Attached).To(BeTrue())
	})

	It("correctly distinguishes disks with same UnitNumber across different controllers", func() {
		sameUUID := "controller-test-uuid"
		unit1 := int32(1)
		bus0 := int32(0)
		bus1 := int32(1)

		moVM := mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{
				Hardware: vimtypes.VirtualHardware{
					Device: []vimtypes.BaseVirtualDevice{
						// Controller 0 (SCSI Bus 0)
						&vimtypes.VirtualLsiLogicController{
							VirtualSCSIController: vimtypes.VirtualSCSIController{
								VirtualController: vimtypes.VirtualController{
									VirtualDevice: vimtypes.VirtualDevice{Key: 100},
									BusNumber:     0,
								},
							},
						},
						// Controller 1 (SCSI Bus 1)
						&vimtypes.VirtualLsiLogicController{
							VirtualSCSIController: vimtypes.VirtualSCSIController{
								VirtualController: vimtypes.VirtualController{
									VirtualDevice: vimtypes.VirtualDevice{Key: 200},
									BusNumber:     1,
								},
							},
						},
						// Disk on Controller 0, Unit 1 (obsolete)
						&vimtypes.VirtualDisk{
							VirtualDevice: vimtypes.VirtualDevice{
								Key:           2001,
								ControllerKey: 100,
								UnitNumber:    &unit1,
								Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
									Uuid:     sameUUID,
									Parent:   &vimtypes.VirtualDiskFlatVer2BackingInfo{},
									DiskMode: string(vimtypes.VirtualDiskModeIndependent_nonpersistent),
								},
							},
						},
						// Disk on Controller 1, Unit 1 (active in spec)
						&vimtypes.VirtualDisk{
							VirtualDevice: vimtypes.VirtualDevice{
								Key:           2002,
								ControllerKey: 200,
								UnitNumber:    &unit1,
								Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
									Uuid:     sameUUID,
									Parent:   &vimtypes.VirtualDiskFlatVer2BackingInfo{},
									DiskMode: string(vimtypes.VirtualDiskModeIndependent_nonpersistent),
								},
							},
						},
					},
				},
			},
		}

		vm := &vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				Volumes: []vmopv1.VirtualMachineVolume{
					// Only vol-2 (on controller 1, unit 1) remains in spec
					{
						Name:                "vol-2",
						ControllerType:      vmopv1.VirtualControllerTypeSCSI,
						ControllerBusNumber: &bus1,
						UnitNumber:          &unit1,
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							VirtualMachineSnapshotDisk: &vmopv1.VirtualMachineSnapshotDiskSpec{
								Name:   "snap-1",
								DiskID: sameUUID,
							},
						},
					},
				},
			},
			Status: vmopv1.VirtualMachineStatus{
				Volumes: []vmopv1.VirtualMachineVolumeStatus{
					{
						Name:                "vol-1",
						Type:                vmopv1.VolumeTypeVirtualMachineSnapshotDisk,
						Attached:            true,
						DiskUUID:            sameUUID,
						ControllerType:      vmopv1.VirtualControllerTypeSCSI,
						ControllerBusNumber: &bus0,
						UnitNumber:          &unit1,
					},
					{
						Name:                "vol-2",
						Type:                vmopv1.VolumeTypeVirtualMachineSnapshotDisk,
						Attached:            true,
						DiskUUID:            sameUUID,
						ControllerType:      vmopv1.VirtualControllerTypeSCSI,
						ControllerBusNumber: &bus1,
						UnitNumber:          &unit1,
					},
				},
			},
		}

		configSpec := &vimtypes.VirtualMachineConfigSpec{}

		vmconfsnapshotdisk.DetachSnapshotDisks(moVM, vm, configSpec)

		// Disk 2001 on Controller 0 (SCSI 0:1) should be removed, while Disk 2002 on Controller 1 (SCSI 1:1) is preserved
		Expect(configSpec.DeviceChange).To(HaveLen(1))
		change := configSpec.DeviceChange[0].GetVirtualDeviceConfigSpec()
		Expect(change.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationRemove))
		Expect(change.Device.GetVirtualDevice().Key).To(Equal(int32(2001)))

		Expect(vm.Status.Volumes[0].Name).To(Equal("vol-1"))
		Expect(vm.Status.Volumes[0].Attached).To(BeFalse())
		Expect(vm.Status.Volumes[1].Name).To(Equal("vol-2"))
		Expect(vm.Status.Volumes[1].Attached).To(BeTrue())
	})

	It("marks Attached=false even if disk is already removed from hardware", func() {
		moVM := mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{
				Hardware: vimtypes.VirtualHardware{
					Device: []vimtypes.BaseVirtualDevice{}, // empty hardware
				},
			},
		}

		vm := &vmopv1.VirtualMachine{
			Status: vmopv1.VirtualMachineStatus{
				Volumes: []vmopv1.VirtualMachineVolumeStatus{
					{
						Name:     "removed-snap-vol",
						Type:     vmopv1.VolumeTypeVirtualMachineSnapshotDisk,
						Attached: true,
						DiskUUID: "non-existent-uuid",
					},
				},
			},
		}

		configSpec := &vimtypes.VirtualMachineConfigSpec{}
		vmconfsnapshotdisk.DetachSnapshotDisks(moVM, vm, configSpec)

		Expect(configSpec.DeviceChange).To(BeEmpty())
		Expect(vm.Status.Volumes[0].Attached).To(BeFalse())
	})
})

var _ = Describe("IsSnapshotDiskVolumeInSpec", func() {
	It("returns true when volume matches name, diskuuid, and slot", func() {
		unit0 := int32(0)
		statusVol := vmopv1.VirtualMachineVolumeStatus{
			Name:       "vol-1",
			Type:       vmopv1.VolumeTypeVirtualMachineSnapshotDisk,
			DiskUUID:   "uuid-123",
			UnitNumber: &unit0,
		}
		specVolumes := []vmopv1.VirtualMachineVolume{
			{
				Name:       "vol-1",
				UnitNumber: &unit0,
				VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
					VirtualMachineSnapshotDisk: &vmopv1.VirtualMachineSnapshotDiskSpec{
						Name:   "snap-1",
						DiskID: "uuid-123",
					},
				},
			},
		}
		Expect(vmconfsnapshotdisk.IsSnapshotDiskVolumeInSpec(statusVol, specVolumes)).To(BeTrue())
	})

	It("returns false when name does not match", func() {
		statusVol := vmopv1.VirtualMachineVolumeStatus{
			Name:     "vol-1",
			Type:     vmopv1.VolumeTypeVirtualMachineSnapshotDisk,
			DiskUUID: "uuid-123",
		}
		specVolumes := []vmopv1.VirtualMachineVolume{
			{
				Name: "vol-2",
				VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
					VirtualMachineSnapshotDisk: &vmopv1.VirtualMachineSnapshotDiskSpec{
						Name:   "snap-1",
						DiskID: "uuid-123",
					},
				},
			},
		}
		Expect(vmconfsnapshotdisk.IsSnapshotDiskVolumeInSpec(statusVol, specVolumes)).To(BeFalse())
	})

	It("returns false when diskuuid does not match", func() {
		statusVol := vmopv1.VirtualMachineVolumeStatus{
			Name:     "vol-1",
			Type:     vmopv1.VolumeTypeVirtualMachineSnapshotDisk,
			DiskUUID: "uuid-123",
		}
		specVolumes := []vmopv1.VirtualMachineVolume{
			{
				Name: "vol-1",
				VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
					VirtualMachineSnapshotDisk: &vmopv1.VirtualMachineSnapshotDiskSpec{
						Name:   "snap-1",
						DiskID: "uuid-456",
					},
				},
			},
		}
		Expect(vmconfsnapshotdisk.IsSnapshotDiskVolumeInSpec(statusVol, specVolumes)).To(BeFalse())
	})

	It("returns false when slot does not match", func() {
		unit0 := int32(0)
		unit1 := int32(1)
		statusVol := vmopv1.VirtualMachineVolumeStatus{
			Name:       "vol-1",
			Type:       vmopv1.VolumeTypeVirtualMachineSnapshotDisk,
			DiskUUID:   "uuid-123",
			UnitNumber: &unit0,
		}
		specVolumes := []vmopv1.VirtualMachineVolume{
			{
				Name:       "vol-1",
				UnitNumber: &unit1, // Spec explicitly requests slot 1, but status is slot 0
				VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
					VirtualMachineSnapshotDisk: &vmopv1.VirtualMachineSnapshotDiskSpec{
						Name:   "snap-1",
						DiskID: "uuid-123",
					},
				},
			},
		}
		Expect(vmconfsnapshotdisk.IsSnapshotDiskVolumeInSpec(statusVol, specVolumes)).To(BeFalse())
	})
})

var _ = Describe("GetVirtualDiskUUID", func() {
	It("returns error for nil disk or nil backing", func() {
		_, err := vmconfsnapshotdisk.GetVirtualDiskUUID(nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("virtual disk is nil"))

		_, err = vmconfsnapshotdisk.GetVirtualDiskUUID(&vimtypes.VirtualDisk{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("virtual disk backing is nil"))
	})

	It("returns error for unsupported backing type", func() {
		disk := &vimtypes.VirtualDisk{
			VirtualDevice: vimtypes.VirtualDevice{
				Backing: &vimtypes.VirtualDiskRawDiskVer2BackingInfo{},
			},
		}
		_, err := vmconfsnapshotdisk.GetVirtualDiskUUID(disk)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unsupported disk backing type"))
	})

	It("returns error when backing has empty UUID", func() {
		disk := &vimtypes.VirtualDisk{
			VirtualDevice: vimtypes.VirtualDevice{
				Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{Uuid: ""},
			},
		}
		_, err := vmconfsnapshotdisk.GetVirtualDiskUUID(disk)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("virtual disk backing UUID is empty"))
	})

	It("returns uuid for supported backing types", func() {
		flatDisk := &vimtypes.VirtualDisk{
			VirtualDevice: vimtypes.VirtualDevice{
				Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{Uuid: "flat-uuid"},
			},
		}
		uuid, err := vmconfsnapshotdisk.GetVirtualDiskUUID(flatDisk)
		Expect(err).ToNot(HaveOccurred())
		Expect(uuid).To(Equal("flat-uuid"))

		seDisk := &vimtypes.VirtualDisk{
			VirtualDevice: vimtypes.VirtualDevice{
				Backing: &vimtypes.VirtualDiskSeSparseBackingInfo{Uuid: "sesparse-uuid"},
			},
		}
		uuid, err = vmconfsnapshotdisk.GetVirtualDiskUUID(seDisk)
		Expect(err).ToNot(HaveOccurred())
		Expect(uuid).To(Equal("sesparse-uuid"))

		sparseDisk := &vimtypes.VirtualDisk{
			VirtualDevice: vimtypes.VirtualDevice{
				Backing: &vimtypes.VirtualDiskSparseVer2BackingInfo{Uuid: "sparse-uuid"},
			},
		}
		uuid, err = vmconfsnapshotdisk.GetVirtualDiskUUID(sparseDisk)
		Expect(err).ToNot(HaveOccurred())
		Expect(uuid).To(Equal("sparse-uuid"))

		rdmDisk := &vimtypes.VirtualDisk{
			VirtualDevice: vimtypes.VirtualDevice{
				Backing: &vimtypes.VirtualDiskRawDiskMappingVer1BackingInfo{Uuid: "rdm-uuid"},
			},
		}
		uuid, err = vmconfsnapshotdisk.GetVirtualDiskUUID(rdmDisk)
		Expect(err).ToNot(HaveOccurred())
		Expect(uuid).To(Equal("rdm-uuid"))
	})
})

var _ = Describe("FindDiskInSnapshot", func() {
	It("returns nil when moSnap is nil", func() {
		disk, info := vmconfsnapshotdisk.FindDiskInSnapshot(nil, "disk-1")
		Expect(disk).To(BeNil())
		Expect(info).To(BeNil())
	})

	It("returns nil when disk is not found", func() {
		moSnap := &mo.VirtualMachineSnapshot{
			Config: vimtypes.VirtualMachineConfigInfo{
				Hardware: vimtypes.VirtualHardware{
					Device: []vimtypes.BaseVirtualDevice{
						&vimtypes.VirtualDisk{
							VirtualDevice: vimtypes.VirtualDevice{
								Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{Uuid: "other-disk"},
							},
						},
					},
				},
			},
		}
		disk, info := vmconfsnapshotdisk.FindDiskInSnapshot(moSnap, "disk-1")
		Expect(disk).To(BeNil())
		Expect(info).To(BeNil())
	})

	It("returns nil when device is not a VirtualDisk or has unsupported backing", func() {
		moSnap := &mo.VirtualMachineSnapshot{
			Config: vimtypes.VirtualMachineConfigInfo{
				Hardware: vimtypes.VirtualHardware{
					Device: []vimtypes.BaseVirtualDevice{
						&vimtypes.VirtualEthernetCard{},
						&vimtypes.VirtualDisk{
							VirtualDevice: vimtypes.VirtualDevice{
								Backing: &vimtypes.VirtualDiskRawDiskVer2BackingInfo{},
							},
						},
					},
				},
			},
		}
		disk, info := vmconfsnapshotdisk.FindDiskInSnapshot(moSnap, "disk-1")
		Expect(disk).To(BeNil())
		Expect(info).To(BeNil())
	})

	It("returns disk and backingInfo when matching disk is found (case-insensitive)", func() {
		flatBacking := &vimtypes.VirtualDiskFlatVer2BackingInfo{
			VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: "[ds] test.vmdk"},
			Uuid:                         "Disk-UUID-1",
		}
		targetDisk := &vimtypes.VirtualDisk{
			CapacityInBytes: 1024 * 1024 * 1024,
			VirtualDevice: vimtypes.VirtualDevice{
				Key:     2000,
				Backing: flatBacking,
			},
		}
		moSnap := &mo.VirtualMachineSnapshot{
			Config: vimtypes.VirtualMachineConfigInfo{
				Hardware: vimtypes.VirtualHardware{
					Device: []vimtypes.BaseVirtualDevice{targetDisk},
				},
			},
		}

		disk, info := vmconfsnapshotdisk.FindDiskInSnapshot(moSnap, "disk-uuid-1")
		Expect(disk).To(Equal(targetDisk))
		Expect(info).ToNot(BeNil())

		// Verify createBacking produces a delta backing
		childBacking := info.CreateBacking()
		flatChild, ok := childBacking.(*vimtypes.VirtualDiskFlatVer2BackingInfo)
		Expect(ok).To(BeTrue())
		Expect(flatChild.DiskMode).To(Equal(string(vimtypes.VirtualDiskModeIndependent_nonpersistent)))
		Expect(flatChild.Parent).To(Equal(flatBacking))
		Expect(flatChild.Uuid).To(Equal("Disk-UUID-1"))
	})

	It("handles SeSparse backing and creates delta backing", func() {
		seBacking := &vimtypes.VirtualDiskSeSparseBackingInfo{
			VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: "[ds] test-sesparse.vmdk"},
			Uuid:                         "SeSparse-UUID-1",
		}
		targetDisk := &vimtypes.VirtualDisk{
			VirtualDevice: vimtypes.VirtualDevice{Key: 2001, Backing: seBacking},
		}
		moSnap := &mo.VirtualMachineSnapshot{
			Config: vimtypes.VirtualMachineConfigInfo{
				Hardware: vimtypes.VirtualHardware{
					Device: []vimtypes.BaseVirtualDevice{targetDisk},
				},
			},
		}

		disk, info := vmconfsnapshotdisk.FindDiskInSnapshot(moSnap, "sesparse-uuid-1")
		Expect(disk).To(Equal(targetDisk))
		Expect(info).ToNot(BeNil())

		childBacking := info.CreateBacking()
		seChild, ok := childBacking.(*vimtypes.VirtualDiskSeSparseBackingInfo)
		Expect(ok).To(BeTrue())
		Expect(seChild.DiskMode).To(Equal(string(vimtypes.VirtualDiskModeIndependent_nonpersistent)))
		Expect(seChild.Parent).To(Equal(seBacking))
		Expect(seChild.Uuid).To(Equal("SeSparse-UUID-1"))
	})

	It("handles SparseVer2 backing and creates delta backing", func() {
		sparseBacking := &vimtypes.VirtualDiskSparseVer2BackingInfo{
			VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: "[ds] test-sparse.vmdk"},
			Uuid:                         "Sparse-UUID-1",
		}
		targetDisk := &vimtypes.VirtualDisk{
			VirtualDevice: vimtypes.VirtualDevice{Key: 2002, Backing: sparseBacking},
		}
		moSnap := &mo.VirtualMachineSnapshot{
			Config: vimtypes.VirtualMachineConfigInfo{
				Hardware: vimtypes.VirtualHardware{
					Device: []vimtypes.BaseVirtualDevice{targetDisk},
				},
			},
		}

		disk, info := vmconfsnapshotdisk.FindDiskInSnapshot(moSnap, "sparse-uuid-1")
		Expect(disk).To(Equal(targetDisk))
		Expect(info).ToNot(BeNil())

		childBacking := info.CreateBacking()
		spChild, ok := childBacking.(*vimtypes.VirtualDiskSparseVer2BackingInfo)
		Expect(ok).To(BeTrue())
		Expect(spChild.DiskMode).To(Equal(string(vimtypes.VirtualDiskModeIndependent_nonpersistent)))
		Expect(spChild.Parent).To(Equal(sparseBacking))
		Expect(spChild.Uuid).To(Equal("Sparse-UUID-1"))
	})

	It("handles RawDiskMappingVer1 backing and creates delta backing", func() {
		rdmBacking := &vimtypes.VirtualDiskRawDiskMappingVer1BackingInfo{
			VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: "[ds] test-rdm.vmdk"},
			Uuid:                         "RDM-UUID-1",
		}
		targetDisk := &vimtypes.VirtualDisk{
			VirtualDevice: vimtypes.VirtualDevice{Key: 2003, Backing: rdmBacking},
		}
		moSnap := &mo.VirtualMachineSnapshot{
			Config: vimtypes.VirtualMachineConfigInfo{
				Hardware: vimtypes.VirtualHardware{
					Device: []vimtypes.BaseVirtualDevice{targetDisk},
				},
			},
		}

		disk, info := vmconfsnapshotdisk.FindDiskInSnapshot(moSnap, "rdm-uuid-1")
		Expect(disk).To(Equal(targetDisk))
		Expect(info).ToNot(BeNil())

		childBacking := info.CreateBacking()
		rdmChild, ok := childBacking.(*vimtypes.VirtualDiskRawDiskMappingVer1BackingInfo)
		Expect(ok).To(BeTrue())
		Expect(rdmChild.DiskMode).To(Equal(string(vimtypes.VirtualDiskModeIndependent_nonpersistent)))
		Expect(rdmChild.Parent).To(Equal(rdmBacking))
		Expect(rdmChild.Uuid).To(Equal("RDM-UUID-1"))
	})
})

var _ = Describe("IsSnapshotDiskAttachedInVMMo", func() {
	var (
		vol  vmopv1.VirtualMachineVolume
		moVM mo.VirtualMachine
	)

	BeforeEach(func() {
		unit0 := int32(0)
		vol = vmopv1.VirtualMachineVolume{
			Name: "test-snap-vol",
			VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
				VirtualMachineSnapshotDisk: &vmopv1.VirtualMachineSnapshotDiskSpec{
					Name:   "snap-1",
					DiskID: "disk-uuid-1",
				},
			},
			UnitNumber: &unit0,
		}
	})

	It("returns false when vol has no VirtualMachineSnapshotDisk or has empty DiskID", func() {
		Expect(vmconfsnapshotdisk.IsSnapshotDiskAttachedInVMMo(moVM, vmopv1.VirtualMachineVolume{})).To(BeFalse())
		Expect(vmconfsnapshotdisk.IsSnapshotDiskAttachedInVMMo(moVM, vmopv1.VirtualMachineVolume{
			VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
				VirtualMachineSnapshotDisk: &vmopv1.VirtualMachineSnapshotDiskSpec{},
			},
		})).To(BeFalse())
	})

	It("returns false when VM has no matching devices or config is nil", func() {
		Expect(vmconfsnapshotdisk.IsSnapshotDiskAttachedInVMMo(moVM, vol)).To(BeFalse())
		moVM = mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{},
		}
		Expect(vmconfsnapshotdisk.IsSnapshotDiskAttachedInVMMo(moVM, vol)).To(BeFalse())
	})

	It("returns true when device in moVM matches both diskuuid and slot (unit number)", func() {
		unit0 := int32(0)
		attachedDisk := &vimtypes.VirtualDisk{
			VirtualDevice: vimtypes.VirtualDevice{
				Key:        2000,
				UnitNumber: &unit0,
				Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
					VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: "[ds] child.vmdk"},
					Uuid:                         "disk-uuid-1",
				},
			},
		}
		moVM = mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{
				Hardware: vimtypes.VirtualHardware{
					Device: []vimtypes.BaseVirtualDevice{attachedDisk},
				},
			},
		}
		Expect(vmconfsnapshotdisk.IsSnapshotDiskAttachedInVMMo(moVM, vol)).To(BeTrue())
	})

	It("returns false when device in moVM matches diskuuid but not slot (unit number)", func() {
		unit1 := int32(1)
		attachedDisk := &vimtypes.VirtualDisk{
			VirtualDevice: vimtypes.VirtualDevice{
				Key:        2000,
				UnitNumber: &unit1,
				Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
					VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: "[ds] child.vmdk"},
					Uuid:                         "disk-uuid-1",
				},
			},
		}
		moVM = mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{
				Hardware: vimtypes.VirtualHardware{
					Device: []vimtypes.BaseVirtualDevice{attachedDisk},
				},
			},
		}
		Expect(vmconfsnapshotdisk.IsSnapshotDiskAttachedInVMMo(moVM, vol)).To(BeFalse())
	})

	It("returns false when device in moVM matches slot but not diskuuid", func() {
		unit0 := int32(0)
		attachedDisk := &vimtypes.VirtualDisk{
			VirtualDevice: vimtypes.VirtualDevice{
				Key:        2000,
				UnitNumber: &unit0,
				Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
					VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: "[ds] child.vmdk"},
					Uuid:                         "different-uuid",
				},
			},
		}
		moVM = mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{
				Hardware: vimtypes.VirtualHardware{
					Device: []vimtypes.BaseVirtualDevice{attachedDisk},
				},
			},
		}
		Expect(vmconfsnapshotdisk.IsSnapshotDiskAttachedInVMMo(moVM, vol)).To(BeFalse())
	})

	It("returns false when device in moVM is not a VirtualDisk", func() {
		unit0 := int32(0)
		cdrom := &vimtypes.VirtualCdrom{
			VirtualDevice: vimtypes.VirtualDevice{
				Key:        2000,
				UnitNumber: &unit0,
			},
		}
		moVM = mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{
				Hardware: vimtypes.VirtualHardware{
					Device: []vimtypes.BaseVirtualDevice{cdrom},
				},
			},
		}
		Expect(vmconfsnapshotdisk.IsSnapshotDiskAttachedInVMMo(moVM, vol)).To(BeFalse())
	})

	It("checks controller info: returns true when controller type and bus number match", func() {
		unit0 := int32(0)
		vol.ControllerType = vmopv1.VirtualControllerTypeSCSI
		vol.ControllerBusNumber = ptr.To(int32(0))
		ctrl0 := &vimtypes.VirtualLsiLogicController{
			VirtualSCSIController: vimtypes.VirtualSCSIController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{Key: 100},
					BusNumber:     0,
				},
			},
		}
		attachedDisk := &vimtypes.VirtualDisk{
			VirtualDevice: vimtypes.VirtualDevice{
				Key:           2000,
				ControllerKey: 100,
				UnitNumber:    &unit0,
				Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
					VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: "[ds] child.vmdk"},
					Uuid:                         "disk-uuid-1",
				},
			},
		}
		moVM = mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{
				Hardware: vimtypes.VirtualHardware{
					Device: []vimtypes.BaseVirtualDevice{ctrl0, attachedDisk},
				},
			},
		}
		Expect(vmconfsnapshotdisk.IsSnapshotDiskAttachedInVMMo(moVM, vol)).To(BeTrue())
	})

	It("checks controller info: returns false when attached disk is on a different controller type", func() {
		unit0 := int32(0)
		vol.ControllerType = vmopv1.VirtualControllerTypeSATA
		vol.ControllerBusNumber = ptr.To(int32(0))
		ctrl0 := &vimtypes.VirtualLsiLogicController{
			VirtualSCSIController: vimtypes.VirtualSCSIController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{Key: 100},
					BusNumber:     0,
				},
			},
		}
		attachedDisk := &vimtypes.VirtualDisk{
			VirtualDevice: vimtypes.VirtualDevice{
				Key:           2000,
				ControllerKey: 100,
				UnitNumber:    &unit0,
				Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
					VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: "[ds] child.vmdk"},
					Uuid:                         "disk-uuid-1",
				},
			},
		}
		moVM = mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{
				Hardware: vimtypes.VirtualHardware{
					Device: []vimtypes.BaseVirtualDevice{ctrl0, attachedDisk},
				},
			},
		}
		Expect(vmconfsnapshotdisk.IsSnapshotDiskAttachedInVMMo(moVM, vol)).To(BeFalse())
	})

	It("checks controller info: returns false when attached disk is on a different bus number", func() {
		unit0 := int32(0)
		vol.ControllerType = vmopv1.VirtualControllerTypeSCSI
		vol.ControllerBusNumber = ptr.To(int32(1))
		ctrl0 := &vimtypes.VirtualLsiLogicController{
			VirtualSCSIController: vimtypes.VirtualSCSIController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{Key: 100},
					BusNumber:     0,
				},
			},
		}
		ctrl1 := &vimtypes.VirtualLsiLogicController{
			VirtualSCSIController: vimtypes.VirtualSCSIController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{Key: 200},
					BusNumber:     1,
				},
			},
		}
		attachedDisk := &vimtypes.VirtualDisk{
			VirtualDevice: vimtypes.VirtualDevice{
				Key:           2000,
				ControllerKey: 100, // BusNumber 0, but vol requested BusNumber 1!
				UnitNumber:    &unit0,
				Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
					VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: "[ds] child.vmdk"},
					Uuid:                         "disk-uuid-1",
				},
			},
		}
		moVM = mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{
				Hardware: vimtypes.VirtualHardware{
					Device: []vimtypes.BaseVirtualDevice{ctrl0, ctrl1, attachedDisk},
				},
			},
		}
		Expect(vmconfsnapshotdisk.IsSnapshotDiskAttachedInVMMo(moVM, vol)).To(BeFalse())
	})

	It("matches when slot info (UnitNumber, ControllerType, ControllerBusNumber) are not specified in vol", func() {
		vol.UnitNumber = nil
		vol.ControllerType = ""
		vol.ControllerBusNumber = nil

		unit3 := int32(3)
		ctrl0 := &vimtypes.VirtualLsiLogicController{
			VirtualSCSIController: vimtypes.VirtualSCSIController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{Key: 100},
					BusNumber:     0,
				},
			},
		}
		attachedDisk := &vimtypes.VirtualDisk{
			VirtualDevice: vimtypes.VirtualDevice{
				Key:           2000,
				ControllerKey: 100,
				UnitNumber:    &unit3,
				Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
					VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: "[ds] child.vmdk"},
					Uuid:                         "disk-uuid-1",
				},
			},
		}
		moVM = mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{
				Hardware: vimtypes.VirtualHardware{
					Device: []vimtypes.BaseVirtualDevice{ctrl0, attachedDisk},
				},
			},
		}
		Expect(vmconfsnapshotdisk.IsSnapshotDiskAttachedInVMMo(moVM, vol)).To(BeTrue())
	})
})

var _ = Describe("FindControllerKeyForSnapshotDisk", func() {
	var (
		vm   *vmopv1.VirtualMachine
		moVM mo.VirtualMachine
		vol  vmopv1.VirtualMachineVolume
	)

	BeforeEach(func() {
		vm = &vmopv1.VirtualMachine{
			Status: vmopv1.VirtualMachineStatus{
				Hardware: &vmopv1.VirtualMachineHardwareStatus{
					Controllers: []vmopv1.VirtualControllerStatus{
						{
							DeviceKey: 200,
							Type:      vmopv1.VirtualControllerTypeSCSI,
							BusNumber: 0,
						},
						{
							DeviceKey: 201,
							Type:      vmopv1.VirtualControllerTypeSCSI,
							BusNumber: 1,
						},
						{
							DeviceKey: 300,
							Type:      vmopv1.VirtualControllerTypeSATA,
							BusNumber: 0,
						},
					},
				},
			},
		}
		moVM = mo.VirtualMachine{Config: &vimtypes.VirtualMachineConfigInfo{}}
		vol = vmopv1.VirtualMachineVolume{Name: "snap-vol"}
	})

	It("matches controller by explicit ControllerType and ControllerBusNumber in spec", func() {
		bus := int32(1)
		vol.ControllerType = vmopv1.VirtualControllerTypeSCSI
		vol.ControllerBusNumber = &bus
		key, err := vmconfsnapshotdisk.FindControllerKeyForSnapshotDisk(vm, moVM, vol)
		Expect(err).ToNot(HaveOccurred())
		Expect(key).To(Equal(int32(201)))

		sataBus := int32(0)
		vol.ControllerType = vmopv1.VirtualControllerTypeSATA
		vol.ControllerBusNumber = &sataBus
		key, err = vmconfsnapshotdisk.FindControllerKeyForSnapshotDisk(vm, moVM, vol)
		Expect(err).ToNot(HaveOccurred())
		Expect(key).To(Equal(int32(300)))
	})

	It("defaults to SCSI bus 0 when volume spec does not specify controller type or bus number", func() {
		key, err := vmconfsnapshotdisk.FindControllerKeyForSnapshotDisk(vm, moVM, vol)
		Expect(err).ToNot(HaveOccurred())
		Expect(key).To(Equal(int32(200)))
	})

	It("returns error when specified controller is not on the VM", func() {
		bus := int32(5)
		vol.ControllerType = vmopv1.VirtualControllerTypeSCSI
		vol.ControllerBusNumber = &bus
		key, err := vmconfsnapshotdisk.FindControllerKeyForSnapshotDisk(vm, moVM, vol)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("waiting for device controller SCSI 5 to be created"))
		Expect(key).To(Equal(int32(0)))
	})

	It("falls back to moVM.Config.Hardware.Device when vm.Status.Hardware is nil", func() {
		vm.Status.Hardware = nil
		bus0 := int32(0)
		moVM = mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{
				Hardware: vimtypes.VirtualHardware{
					Device: []vimtypes.BaseVirtualDevice{
						&vimtypes.VirtualLsiLogicSASController{
							VirtualSCSIController: vimtypes.VirtualSCSIController{
								VirtualController: vimtypes.VirtualController{
									VirtualDevice: vimtypes.VirtualDevice{Key: 500},
									BusNumber:     bus0,
								},
							},
						},
					},
				},
			},
		}

		key, err := vmconfsnapshotdisk.FindControllerKeyForSnapshotDisk(vm, moVM, vol)
		Expect(err).ToNot(HaveOccurred())
		Expect(key).To(Equal(int32(500)))
	})

	It("returns error when VM has no hardware status and moVM has no controllers", func() {
		vm.Status.Hardware = nil
		key, err := vmconfsnapshotdisk.FindControllerKeyForSnapshotDisk(vm, moVM, vol)
		Expect(err).To(HaveOccurred())
		Expect(key).To(Equal(int32(0)))

		vm.Status.Hardware = &vmopv1.VirtualMachineHardwareStatus{}
		key, err = vmconfsnapshotdisk.FindControllerKeyForSnapshotDisk(vm, moVM, vol)
		Expect(err).To(HaveOccurred())
		Expect(key).To(Equal(int32(0)))
	})
})

var _ = Describe("AddSnapshotDiskInConfigSpec", func() {
	var (
		ctx          context.Context
		k8sClient    ctrlclient.Client
		vimClient    *vim25.Client
		vm           *vmopv1.VirtualMachine
		moVM         mo.VirtualMachine
		configSpec   *vimtypes.VirtualMachineConfigSpec
		vol          vmopv1.VirtualMachineVolume
		newDeviceKey int32
		cleanup      func()
	)

	BeforeEach(func() {
		ctx = pkgcfg.NewContextWithDefaultConfig()
		newDeviceKey = -100
		configSpec = &vimtypes.VirtualMachineConfigSpec{}
		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test-vm",
			},
			Status: vmopv1.VirtualMachineStatus{
				Hardware: &vmopv1.VirtualMachineHardwareStatus{
					Controllers: []vmopv1.VirtualControllerStatus{
						{
							DeviceKey: 1000,
							Type:      vmopv1.VirtualControllerTypeSCSI,
							BusNumber: 0,
						},
					},
				},
			},
		}
		moVM = mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{},
		}
		vol = vmopv1.VirtualMachineVolume{
			Name: "snap-vol",
			VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
				VirtualMachineSnapshotDisk: &vmopv1.VirtualMachineSnapshotDiskSpec{
					Name:   "snap-1",
					DiskID: "disk-1",
				},
			},
		}

		snap := &vmopv1.VirtualMachineSnapshot{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "snap-1",
			},
			Status: vmopv1.VirtualMachineSnapshotStatus{
				UniqueID: "snapshot-100",
				Conditions: []metav1.Condition{
					{
						Type:   string(vmopv1.VirtualMachineSnapshotReadyCondition),
						Status: metav1.ConditionTrue,
					},
				},
			},
		}

		scheme := runtime.NewScheme()
		Expect(vmopv1.AddToScheme(scheme)).To(Succeed())
		k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(snap).Build()

		cleanup = vmconfsnapshotdisk.SetRetrieveSnapshotHardware(func(
			_ context.Context,
			_ *vim25.Client,
			_ vimtypes.ManagedObjectReference,
		) (*mo.VirtualMachineSnapshot, error) {
			return &mo.VirtualMachineSnapshot{
				Config: vimtypes.VirtualMachineConfigInfo{
					Hardware: vimtypes.VirtualHardware{
						Device: []vimtypes.BaseVirtualDevice{
							&vimtypes.VirtualDisk{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: 2000,
									Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
										Uuid: "disk-1",
									},
								},
							},
						},
					},
				},
			}, nil
		})
	})

	AfterEach(func() {
		if cleanup != nil {
			cleanup()
		}
	})

	It("returns (true, nil) when snapshot disk is newly added", func() {
		cache := make(map[string]vmconfsnapshotdisk.SnapshotFetchResult)
		added, err := vmconfsnapshotdisk.AddSnapshotDiskInConfigSpec(
			ctx, k8sClient, vimClient, vm, moVM, configSpec, vol, &newDeviceKey, cache)
		Expect(err).ToNot(HaveOccurred())
		Expect(added).To(BeTrue())
		Expect(configSpec.DeviceChange).To(HaveLen(1))
		Expect(configSpec.DeviceChange[0].GetVirtualDeviceConfigSpec().Device.GetVirtualDevice().ControllerKey).To(Equal(int32(1000)))
		Expect(vm.Status.Volumes).To(HaveLen(1))
		Expect(vm.Status.Volumes[0].Type).To(Equal(vmopv1.VolumeTypeVirtualMachineSnapshotDisk))
		Expect(vm.Status.Volumes[0].Attached).To(BeTrue())
	})

	It("marks snapshot disk as failed and returns error to requeue when target controller is not found on the VM", func() {
		bus := int32(9)
		vol.ControllerBusNumber = &bus
		cache := make(map[string]vmconfsnapshotdisk.SnapshotFetchResult)
		added, err := vmconfsnapshotdisk.AddSnapshotDiskInConfigSpec(
			ctx, k8sClient, vimClient, vm, moVM, configSpec, vol, &newDeviceKey, cache)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("waiting for device controller"))
		Expect(added).To(BeFalse())
		Expect(configSpec.DeviceChange).To(BeEmpty())
		Expect(vm.Status.Volumes).To(HaveLen(1))
		Expect(vm.Status.Volumes[0].Type).To(Equal(vmopv1.VolumeTypeVirtualMachineSnapshotDisk))
		Expect(vm.Status.Volumes[0].Attached).To(BeFalse())
		Expect(vm.Status.Volumes[0].Error).To(ContainSubstring("waiting for device controller"))
	})

	It("returns (false, nil) when snapshot disk is not found (business error)", func() {
		vol.VirtualMachineSnapshotDisk.Name = "missing-snap"
		cache := make(map[string]vmconfsnapshotdisk.SnapshotFetchResult)
		added, err := vmconfsnapshotdisk.AddSnapshotDiskInConfigSpec(
			ctx, k8sClient, vimClient, vm, moVM, configSpec, vol, &newDeviceKey, cache)
		Expect(err).ToNot(HaveOccurred())
		Expect(added).To(BeFalse())
		Expect(vm.Status.Volumes[0].Type).To(Equal(vmopv1.VolumeTypeVirtualMachineSnapshotDisk))
		Expect(vm.Status.Volumes[0].Error).To(ContainSubstring("missing-snap not found"))
	})

	It("returns (false, err) when vCenter fetch encounters a transient error", func() {
		cleanup()
		cleanup = vmconfsnapshotdisk.SetRetrieveSnapshotHardware(func(
			_ context.Context,
			_ *vim25.Client,
			_ vimtypes.ManagedObjectReference,
		) (*mo.VirtualMachineSnapshot, error) {
			return nil, fmt.Errorf("connection reset by peer")
		})

		cache := make(map[string]vmconfsnapshotdisk.SnapshotFetchResult)
		added, err := vmconfsnapshotdisk.AddSnapshotDiskInConfigSpec(
			ctx, k8sClient, vimClient, vm, moVM, configSpec, vol, &newDeviceKey, cache)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("connection reset by peer"))
		Expect(added).To(BeFalse())
		Expect(vm.Status.Volumes[0].Error).To(ContainSubstring("connection reset by peer"))
	})

	Context("Status.volumes already attached check", func() {
		It("considers volume already attached and skips fetchSnapshotHardware when volume is in moVM hardware and status.volumes with matching uuid and attached=true", func() {
			cleanup()
			cleanup = vmconfsnapshotdisk.SetRetrieveSnapshotHardware(func(
				_ context.Context,
				_ *vim25.Client,
				_ vimtypes.ManagedObjectReference,
			) (*mo.VirtualMachineSnapshot, error) {
				Fail("retrieveSnapshotHardware should not be called when volume is already attached in hardware")
				return nil, nil
			})

			moVM.Config.Hardware.Device = append(moVM.Config.Hardware.Device, &vimtypes.VirtualDisk{
				VirtualDevice: vimtypes.VirtualDevice{
					Key:           2000,
					ControllerKey: 1000,
					Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
						Uuid: "disk-1",
					},
				},
			})

			vm.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{
				{
					Name:     vol.Name,
					Type:     vmopv1.VolumeTypeVirtualMachineSnapshotDisk,
					Attached: true,
					DiskUUID: "disk-1",
				},
			}

			cache := make(map[string]vmconfsnapshotdisk.SnapshotFetchResult)
			added, err := vmconfsnapshotdisk.AddSnapshotDiskInConfigSpec(
				ctx, k8sClient, vimClient, vm, moVM, configSpec, vol, &newDeviceKey, cache)
			Expect(err).ToNot(HaveOccurred())
			Expect(added).To(BeFalse())
			Expect(configSpec.DeviceChange).To(BeEmpty())
		})

		It("considers volume already attached when spec, hardware, and status have matching explicit slot", func() {
			cleanup()
			cleanup = vmconfsnapshotdisk.SetRetrieveSnapshotHardware(func(
				_ context.Context,
				_ *vim25.Client,
				_ vimtypes.ManagedObjectReference,
			) (*mo.VirtualMachineSnapshot, error) {
				Fail("retrieveSnapshotHardware should not be called when volume slot matches in hardware")
				return nil, nil
			})

			unit := int32(2)
			vol.UnitNumber = &unit
			vol.ControllerType = vmopv1.VirtualControllerTypeSCSI
			vol.ControllerBusNumber = ptr.To(int32(0))

			moVM.Config.Hardware.Device = append(moVM.Config.Hardware.Device, &vimtypes.VirtualDisk{
				VirtualDevice: vimtypes.VirtualDevice{
					Key:           2000,
					ControllerKey: 1000,
					UnitNumber:    &unit,
					Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
						Uuid: "disk-1",
					},
				},
			})

			vm.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{
				{
					Name:                vol.Name,
					Type:                vmopv1.VolumeTypeVirtualMachineSnapshotDisk,
					Attached:            true,
					DiskUUID:            "disk-1",
					UnitNumber:          &unit,
					ControllerType:      vmopv1.VirtualControllerTypeSCSI,
					ControllerBusNumber: ptr.To(int32(0)),
				},
			}

			cache := make(map[string]vmconfsnapshotdisk.SnapshotFetchResult)
			added, err := vmconfsnapshotdisk.AddSnapshotDiskInConfigSpec(
				ctx, k8sClient, vimClient, vm, moVM, configSpec, vol, &newDeviceKey, cache)
			Expect(err).ToNot(HaveOccurred())
			Expect(added).To(BeFalse())
			Expect(configSpec.DeviceChange).To(BeEmpty())
		})

		It("re-attaches disk when volume is in status.volumes as attached but missing from moVM hardware (self-healing)", func() {
			fetchCalled := false
			cleanup()
			cleanup = vmconfsnapshotdisk.SetRetrieveSnapshotHardware(func(
				_ context.Context,
				_ *vim25.Client,
				_ vimtypes.ManagedObjectReference,
			) (*mo.VirtualMachineSnapshot, error) {
				fetchCalled = true
				return &mo.VirtualMachineSnapshot{
					Config: vimtypes.VirtualMachineConfigInfo{
						Hardware: vimtypes.VirtualHardware{
							Device: []vimtypes.BaseVirtualDevice{
								&vimtypes.VirtualDisk{
									VirtualDevice: vimtypes.VirtualDevice{
										Key: 2000,
										Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
											Uuid: "disk-1",
										},
									},
								},
							},
						},
					},
				}, nil
			})

			// Status says attached=true, but moVM has no such disk in hardware
			vm.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{
				{
					Name:     vol.Name,
					Type:     vmopv1.VolumeTypeVirtualMachineSnapshotDisk,
					Attached: true,
					DiskUUID: "disk-1",
				},
			}

			cache := make(map[string]vmconfsnapshotdisk.SnapshotFetchResult)
			added, err := vmconfsnapshotdisk.AddSnapshotDiskInConfigSpec(
				ctx, k8sClient, vimClient, vm, moVM, configSpec, vol, &newDeviceKey, cache)
			Expect(err).ToNot(HaveOccurred())
			Expect(added).To(BeTrue())
			Expect(fetchCalled).To(BeTrue())
			Expect(configSpec.DeviceChange).To(HaveLen(1))
			Expect(vm.Status.Volumes[0].Attached).To(BeTrue())
		})

		It("fetches snapshot hardware when volume is in status.volumes but attached=false", func() {
			fetchCalled := false
			cleanup()
			cleanup = vmconfsnapshotdisk.SetRetrieveSnapshotHardware(func(
				_ context.Context,
				_ *vim25.Client,
				_ vimtypes.ManagedObjectReference,
			) (*mo.VirtualMachineSnapshot, error) {
				fetchCalled = true
				return &mo.VirtualMachineSnapshot{
					Config: vimtypes.VirtualMachineConfigInfo{
						Hardware: vimtypes.VirtualHardware{
							Device: []vimtypes.BaseVirtualDevice{
								&vimtypes.VirtualDisk{
									VirtualDevice: vimtypes.VirtualDevice{
										Key: 2000,
										Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
											Uuid: "disk-1",
										},
									},
								},
							},
						},
					},
				}, nil
			})

			vm.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{
				{
					Name:     vol.Name,
					Type:     vmopv1.VolumeTypeVirtualMachineSnapshotDisk,
					Attached: false,
					DiskUUID: "disk-1",
				},
			}

			cache := make(map[string]vmconfsnapshotdisk.SnapshotFetchResult)
			added, err := vmconfsnapshotdisk.AddSnapshotDiskInConfigSpec(
				ctx, k8sClient, vimClient, vm, moVM, configSpec, vol, &newDeviceKey, cache)
			Expect(err).ToNot(HaveOccurred())
			Expect(added).To(BeTrue())
			Expect(fetchCalled).To(BeTrue())
			Expect(configSpec.DeviceChange).To(HaveLen(1))
			Expect(vm.Status.Volumes[0].Attached).To(BeTrue())
		})

		It("fetches snapshot hardware when volume is in status.volumes but diskUUID does not match", func() {
			fetchCalled := false
			cleanup()
			cleanup = vmconfsnapshotdisk.SetRetrieveSnapshotHardware(func(
				_ context.Context,
				_ *vim25.Client,
				_ vimtypes.ManagedObjectReference,
			) (*mo.VirtualMachineSnapshot, error) {
				fetchCalled = true
				return &mo.VirtualMachineSnapshot{
					Config: vimtypes.VirtualMachineConfigInfo{
						Hardware: vimtypes.VirtualHardware{
							Device: []vimtypes.BaseVirtualDevice{
								&vimtypes.VirtualDisk{
									VirtualDevice: vimtypes.VirtualDevice{
										Key: 2000,
										Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
											Uuid: "disk-1",
										},
									},
								},
							},
						},
					},
				}, nil
			})

			vm.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{
				{
					Name:     vol.Name,
					Type:     vmopv1.VolumeTypeVirtualMachineSnapshotDisk,
					Attached: true,
					DiskUUID: "different-disk-uuid",
				},
			}

			cache := make(map[string]vmconfsnapshotdisk.SnapshotFetchResult)
			added, err := vmconfsnapshotdisk.AddSnapshotDiskInConfigSpec(
				ctx, k8sClient, vimClient, vm, moVM, configSpec, vol, &newDeviceKey, cache)
			Expect(err).ToNot(HaveOccurred())
			Expect(added).To(BeTrue())
			Expect(fetchCalled).To(BeTrue())
			Expect(configSpec.DeviceChange).To(HaveLen(1))
			Expect(vm.Status.Volumes[0].DiskUUID).To(Equal("disk-1"))
		})

		It("fetches snapshot hardware when volume is in status.volumes but slot does not match", func() {
			fetchCalled := false
			cleanup()
			cleanup = vmconfsnapshotdisk.SetRetrieveSnapshotHardware(func(
				_ context.Context,
				_ *vim25.Client,
				_ vimtypes.ManagedObjectReference,
			) (*mo.VirtualMachineSnapshot, error) {
				fetchCalled = true
				return &mo.VirtualMachineSnapshot{
					Config: vimtypes.VirtualMachineConfigInfo{
						Hardware: vimtypes.VirtualHardware{
							Device: []vimtypes.BaseVirtualDevice{
								&vimtypes.VirtualDisk{
									VirtualDevice: vimtypes.VirtualDevice{
										Key: 2000,
										Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
											Uuid: "disk-1",
										},
									},
								},
							},
						},
					},
				}, nil
			})

			unit1 := int32(1)
			unit2 := int32(2)
			vol.UnitNumber = &unit1

			vm.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{
				{
					Name:       vol.Name,
					Type:       vmopv1.VolumeTypeVirtualMachineSnapshotDisk,
					Attached:   true,
					DiskUUID:   "disk-1",
					UnitNumber: &unit2,
				},
			}

			cache := make(map[string]vmconfsnapshotdisk.SnapshotFetchResult)
			added, err := vmconfsnapshotdisk.AddSnapshotDiskInConfigSpec(
				ctx, k8sClient, vimClient, vm, moVM, configSpec, vol, &newDeviceKey, cache)
			Expect(err).ToNot(HaveOccurred())
			Expect(added).To(BeTrue())
			Expect(fetchCalled).To(BeTrue())
		})

		It("skips fetchSnapshotHardware when volume is already attached in moVM hardware (even if not in status)", func() {
			cleanup()
			cleanup = vmconfsnapshotdisk.SetRetrieveSnapshotHardware(func(
				_ context.Context,
				_ *vim25.Client,
				_ vimtypes.ManagedObjectReference,
			) (*mo.VirtualMachineSnapshot, error) {
				Fail("retrieveSnapshotHardware should not be called when volume is already attached in moVM hardware")
				return nil, nil
			})

			unit0 := int32(0)
			vol.UnitNumber = &unit0
			moVM.Config.Hardware.Device = append(moVM.Config.Hardware.Device, &vimtypes.VirtualDisk{
				VirtualDevice: vimtypes.VirtualDevice{
					Key:           2000,
					ControllerKey: 100,
					UnitNumber:    &unit0,
					Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
						VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: "[ds] child.vmdk"},
						Uuid:                         "disk-1",
					},
				},
			})

			cache := make(map[string]vmconfsnapshotdisk.SnapshotFetchResult)
			added, err := vmconfsnapshotdisk.AddSnapshotDiskInConfigSpec(
				ctx, k8sClient, vimClient, vm, moVM, configSpec, vol, &newDeviceKey, cache)
			Expect(err).ToNot(HaveOccurred())
			Expect(added).To(BeFalse())
			Expect(configSpec.DeviceChange).To(BeEmpty())
			Expect(vm.Status.Volumes).To(HaveLen(1))
			Expect(vm.Status.Volumes[0].Attached).To(BeTrue())
			Expect(vm.Status.Volumes[0].DiskUUID).To(Equal("disk-1"))
			Expect(vm.Status.Volumes[0].UnitNumber).To(Equal(&unit0))
		})
	})

	Context("IsSnapshotDiskSlotMatch", func() {
		It("handles various slot combinations correctly", func() {
			u0 := int32(0)
			u1 := int32(1)
			b0 := int32(0)
			b1 := int32(1)

			// Both nil unit number -> match
			Expect(vmconfsnapshotdisk.IsSnapshotDiskSlotMatch(
				vmopv1.VirtualMachineVolume{},
				vmopv1.VirtualMachineVolumeStatus{},
			)).To(BeTrue())

			// Spec nil unit number, status has unit number -> match
			Expect(vmconfsnapshotdisk.IsSnapshotDiskSlotMatch(
				vmopv1.VirtualMachineVolume{},
				vmopv1.VirtualMachineVolumeStatus{UnitNumber: &u0},
			)).To(BeTrue())

			// Spec has unit number, status has same -> match
			Expect(vmconfsnapshotdisk.IsSnapshotDiskSlotMatch(
				vmopv1.VirtualMachineVolume{UnitNumber: &u1},
				vmopv1.VirtualMachineVolumeStatus{UnitNumber: &u1},
			)).To(BeTrue())

			// Spec has unit number, status has different -> mismatch
			Expect(vmconfsnapshotdisk.IsSnapshotDiskSlotMatch(
				vmopv1.VirtualMachineVolume{UnitNumber: &u1},
				vmopv1.VirtualMachineVolumeStatus{UnitNumber: &u0},
			)).To(BeFalse())

			// Spec has unit number, status is nil -> mismatch
			Expect(vmconfsnapshotdisk.IsSnapshotDiskSlotMatch(
				vmopv1.VirtualMachineVolume{UnitNumber: &u1},
				vmopv1.VirtualMachineVolumeStatus{},
			)).To(BeFalse())

			// ControllerBusNumber mismatch
			Expect(vmconfsnapshotdisk.IsSnapshotDiskSlotMatch(
				vmopv1.VirtualMachineVolume{ControllerBusNumber: &b1},
				vmopv1.VirtualMachineVolumeStatus{ControllerBusNumber: &b0},
			)).To(BeFalse())

			// ControllerType mismatch
			Expect(vmconfsnapshotdisk.IsSnapshotDiskSlotMatch(
				vmopv1.VirtualMachineVolume{ControllerType: vmopv1.VirtualControllerTypeSCSI},
				vmopv1.VirtualMachineVolumeStatus{ControllerType: vmopv1.VirtualControllerTypeSATA},
			)).To(BeFalse())
		})
	})
})

var _ = Describe("GetControllerMapFromMoVM", func() {
	It("returns empty map when moVM.Config is nil", func() {
		moVM := mo.VirtualMachine{}
		Expect(vmconfsnapshotdisk.GetControllerMapFromMoVM(moVM)).To(BeEmpty())
	})

	It("builds map of controllers from moVM devices", func() {
		moVM := mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{
				Hardware: vimtypes.VirtualHardware{
					Device: []vimtypes.BaseVirtualDevice{
						&vimtypes.ParaVirtualSCSIController{
							VirtualSCSIController: vimtypes.VirtualSCSIController{
								VirtualController: vimtypes.VirtualController{
									VirtualDevice: vimtypes.VirtualDevice{
										Key: 1000,
									},
									BusNumber: 0,
								},
							},
						},
						&vimtypes.VirtualAHCIController{
							VirtualSATAController: vimtypes.VirtualSATAController{
								VirtualController: vimtypes.VirtualController{
									VirtualDevice: vimtypes.VirtualDevice{
										Key: 15000,
									},
									BusNumber: 1,
								},
							},
						},
					},
				},
			},
		}
		ctrlMap := vmconfsnapshotdisk.GetControllerMapFromMoVM(moVM)
		Expect(ctrlMap).To(HaveLen(2))
		Expect(ctrlMap[1000].ControllerType).To(Equal(vmopv1.VirtualControllerTypeSCSI))
		Expect(ctrlMap[1000].BusNumber).To(Equal(int32(0)))
		Expect(ctrlMap[15000].ControllerType).To(Equal(vmopv1.VirtualControllerTypeSATA))
		Expect(ctrlMap[15000].BusNumber).To(Equal(int32(1)))
	})
})

var _ = Describe("NewControllerMapFn", func() {
	It("lazily evaluates and only triggers once", func() {
		moVM := mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{
				Hardware: vimtypes.VirtualHardware{
					Device: []vimtypes.BaseVirtualDevice{
						&vimtypes.ParaVirtualSCSIController{
							VirtualSCSIController: vimtypes.VirtualSCSIController{
								VirtualController: vimtypes.VirtualController{
									VirtualDevice: vimtypes.VirtualDevice{
										Key: 1000,
									},
									BusNumber: 0,
								},
							},
						},
					},
				},
			},
		}

		fn := vmconfsnapshotdisk.NewControllerMapFn(moVM)

		// First call computes the map
		map1 := fn()
		Expect(map1).To(HaveLen(1))
		Expect(map1[1000].ControllerType).To(Equal(vmopv1.VirtualControllerTypeSCSI))

		// Mutating moVM afterwards does not affect cached map
		moVM.Config.Hardware.Device = nil
		map2 := fn()
		Expect(map2).To(HaveLen(1))
		Expect(map2[1000].ControllerType).To(Equal(vmopv1.VirtualControllerTypeSCSI))
	})
})

var _ = Describe("ResolveControllerMapFn", func() {
	It("returns custom fn when provided", func() {
		customCalled := false
		customFn := func() map[int32]pkgutil.ControllerID {
			customCalled = true
			return map[int32]pkgutil.ControllerID{
				100: {ControllerType: vmopv1.VirtualControllerTypeSCSI, BusNumber: 1},
			}
		}

		resolved := vmconfsnapshotdisk.ResolveControllerMapFn(mo.VirtualMachine{}, []vmconfsnapshotdisk.ControllerMapFn{customFn})
		res := resolved()
		Expect(customCalled).To(BeTrue())
		Expect(res).To(HaveKey(int32(100)))
	})

	It("returns newControllerMapFn when no fn provided or fn is nil", func() {
		resolved := vmconfsnapshotdisk.ResolveControllerMapFn(mo.VirtualMachine{}, nil)
		Expect(resolved()).To(BeEmpty())

		resolvedNil := vmconfsnapshotdisk.ResolveControllerMapFn(mo.VirtualMachine{}, []vmconfsnapshotdisk.ControllerMapFn{nil})
		Expect(resolvedNil()).To(BeEmpty())
	})
})
