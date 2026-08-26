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
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
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

	Context("when CSIBackupAPI is disabled", func() {
		BeforeEach(func() {
			cfg := pkgcfg.FromContext(ctx)
			cfg.Features.CSIBackupAPI = false
			ctx = pkgcfg.WithContext(ctx, cfg)

			vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
				{
					Name: "snap-vol",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						VirtualMachineSnapshot: &vmopv1.VirtualMachineSnapshotDiskSpec{
							Name:   "snap-1",
							DiskID: "disk-1",
						},
					},
				},
			}
		})

		It("sets error on volume status and returns nil", func() {
			err := r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
			Expect(err).ToNot(HaveOccurred())
			Expect(vm.Status.Volumes).To(HaveLen(1))
			Expect(vm.Status.Volumes[0].Error).To(ContainSubstring("CSIBackupAPI feature is disabled"))
			Expect(vm.Status.Volumes[0].Attached).To(BeFalse())
		})
	})

	Context("when snapshot is not found", func() {
		BeforeEach(func() {
			vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
				{
					Name: "snap-vol",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						VirtualMachineSnapshot: &vmopv1.VirtualMachineSnapshotDiskSpec{
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
						VirtualMachineSnapshot: &vmopv1.VirtualMachineSnapshotDiskSpec{
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
						VirtualMachineSnapshot: &vmopv1.VirtualMachineSnapshotDiskSpec{
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
						VirtualMachineSnapshot: &vmopv1.VirtualMachineSnapshotDiskSpec{
							Name:   "shared-snap",
							DiskID: "disk-uuid-1",
						},
					},
				},
				{
					Name: "vol-2",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						VirtualMachineSnapshot: &vmopv1.VirtualMachineSnapshotDiskSpec{
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

		It("caches snapshot fetch errors so hardware is only retrieved once on failure", func() {
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

			// Exactly 1 call on failure too
			Expect(fetchCallCount).To(Equal(1))
			Expect(vm.Status.Volumes).To(HaveLen(2))
			Expect(vm.Status.Volumes[0].Error).To(Equal("simulated vCenter error"))
			Expect(vm.Status.Volumes[0].Attached).To(BeFalse())
			Expect(vm.Status.Volumes[1].Error).To(Equal("simulated vCenter error"))
			Expect(vm.Status.Volumes[1].Attached).To(BeFalse())
		})

		It("fetches snapshot hardware once per snapshot for volumes from different snapshots", func() {
			vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
				{
					Name: "vol-1",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						VirtualMachineSnapshot: &vmopv1.VirtualMachineSnapshotDiskSpec{
							Name:   "snap-a",
							DiskID: "disk-uuid-a",
						},
					},
				},
				{
					Name: "vol-2",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						VirtualMachineSnapshot: &vmopv1.VirtualMachineSnapshotDiskSpec{
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

var _ = Describe("RemoveObsoleteSnapshotDisks", func() {
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

		snapshotDisks := []vmopv1.VirtualMachineVolume{}
		configSpec := &vimtypes.VirtualMachineConfigSpec{}
		vm := &vmopv1.VirtualMachine{}

		vmconfsnapshotdisk.RemoveObsoleteSnapshotDisks(moVM, vm, snapshotDisks, configSpec)

		Expect(configSpec.DeviceChange).To(HaveLen(1))
		change := configSpec.DeviceChange[0].GetVirtualDeviceConfigSpec()
		Expect(change.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationRemove))
		Expect(change.Device.GetVirtualDevice().Key).To(Equal(int32(2000)))
	})

	It("removes obsolete snapshot disk without affecting regular disk with same UUID", func() {
		sameUUID := "shared-disk-uuid"
		moVM := mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{
				Hardware: vimtypes.VirtualHardware{
					Device: []vimtypes.BaseVirtualDevice{
						// Regular persistent disk (e.g. boot disk)
						&vimtypes.VirtualDisk{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: 1000,
								Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
									Uuid:     sameUUID,
									DiskMode: string(vimtypes.VirtualDiskModePersistent),
								},
							},
						},
						// Obsolete snapshot disk (independent non-persistent) with same UUID
						&vimtypes.VirtualDisk{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: 2000,
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
						Name:     "boot-disk",
						Type:     vmopv1.VolumeTypeClassic,
						Attached: true,
						DiskUUID: sameUUID,
					},
					{
						Name:     "snapshot-disk",
						Type:     vmopv1.VolumeTypeClassic,
						Attached: true,
						DiskUUID: sameUUID,
					},
				},
			},
		}

		snapshotDisks := []vmopv1.VirtualMachineVolume{}
		configSpec := &vimtypes.VirtualMachineConfigSpec{}

		vmconfsnapshotdisk.RemoveObsoleteSnapshotDisks(moVM, vm, snapshotDisks, configSpec)

		// Only the independent non-persistent snapshot disk should be removed
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

	It("does not remove independent non-persistent disk without parent backing", func() {
		moVM := mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{
				Hardware: vimtypes.VirtualHardware{
					Device: []vimtypes.BaseVirtualDevice{
						// Standalone independent non-persistent disk (not derived from a snapshot, no parent backing)
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

		snapshotDisks := []vmopv1.VirtualMachineVolume{}
		configSpec := &vimtypes.VirtualMachineConfigSpec{}
		vm := &vmopv1.VirtualMachine{}

		vmconfsnapshotdisk.RemoveObsoleteSnapshotDisks(moVM, vm, snapshotDisks, configSpec)

		// The standalone non-persistent disk must NOT be removed because it has no parent backing
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

		snapshotDisks := []vmopv1.VirtualMachineVolume{}
		configSpec := &vimtypes.VirtualMachineConfigSpec{}

		vmconfsnapshotdisk.RemoveObsoleteSnapshotDisks(moVM, vm, snapshotDisks, configSpec)

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
							VirtualMachineSnapshot: &vmopv1.VirtualMachineSnapshotDiskSpec{
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
						Type:       vmopv1.VolumeTypeClassic,
						Attached:   true,
						DiskUUID:   sameUUID,
						UnitNumber: &unit1,
					},
					{
						Name:       "vol-2",
						Type:       vmopv1.VolumeTypeClassic,
						Attached:   true,
						DiskUUID:   sameUUID,
						UnitNumber: &unit2,
					},
				},
			},
		}

		snapshotDisks := []vmopv1.VirtualMachineVolume{
			vm.Spec.Volumes[0],
		}
		configSpec := &vimtypes.VirtualMachineConfigSpec{}

		vmconfsnapshotdisk.RemoveObsoleteSnapshotDisks(moVM, vm, snapshotDisks, configSpec)

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
							VirtualMachineSnapshot: &vmopv1.VirtualMachineSnapshotDiskSpec{
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
						Type:                vmopv1.VolumeTypeClassic,
						Attached:            true,
						DiskUUID:            sameUUID,
						ControllerType:      vmopv1.VirtualControllerTypeSCSI,
						ControllerBusNumber: &bus0,
						UnitNumber:          &unit1,
					},
					{
						Name:                "vol-2",
						Type:                vmopv1.VolumeTypeClassic,
						Attached:            true,
						DiskUUID:            sameUUID,
						ControllerType:      vmopv1.VirtualControllerTypeSCSI,
						ControllerBusNumber: &bus1,
						UnitNumber:          &unit1,
					},
				},
			},
		}

		snapshotDisks := []vmopv1.VirtualMachineVolume{
			vm.Spec.Volumes[0],
		}
		configSpec := &vimtypes.VirtualMachineConfigSpec{}

		vmconfsnapshotdisk.RemoveObsoleteSnapshotDisks(moVM, vm, snapshotDisks, configSpec)

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
})

var _ = Describe("VolumePlacement", func() {
	Context("GetVolumeStatusPlacement", func() {
		It("extracts placement from volume status with defaults", func() {
			unit := int32(3)
			vs := vmopv1.VirtualMachineVolumeStatus{
				UnitNumber: &unit,
			}
			placement := vmconfsnapshotdisk.GetVolumeStatusPlacement(vs)
			Expect(placement.ControllerType).To(Equal(vmopv1.VirtualControllerTypeSCSI))
			Expect(placement.BusNumber).To(Equal(int32(0)))
			Expect(placement.UnitNumber).To(HaveValue(Equal(unit)))
		})

		It("extracts placement from volume status with explicit controller type and bus", func() {
			unit := int32(2)
			bus := int32(1)
			vs := vmopv1.VirtualMachineVolumeStatus{
				ControllerType:      vmopv1.VirtualControllerTypeSATA,
				ControllerBusNumber: &bus,
				UnitNumber:          &unit,
			}
			placement := vmconfsnapshotdisk.GetVolumeStatusPlacement(vs)
			Expect(placement.ControllerType).To(Equal(vmopv1.VirtualControllerTypeSATA))
			Expect(placement.BusNumber).To(Equal(bus))
			Expect(placement.UnitNumber).To(HaveValue(Equal(unit)))
		})
	})

	Context("GetEffectivePlacement", func() {
		It("prefers explicit spec placement over status placement", func() {
			specUnit := int32(1)
			specBus := int32(2)
			statusUnit := int32(5)
			statusBus := int32(0)

			vol := vmopv1.VirtualMachineVolume{
				Name:                "test-vol",
				ControllerType:      vmopv1.VirtualControllerTypeNVME,
				ControllerBusNumber: &specBus,
				UnitNumber:          &specUnit,
			}
			statusPlacementMap := map[string]vmconfsnapshotdisk.VolumePlacement{
				"test-vol": {
					ControllerType: vmopv1.VirtualControllerTypeSCSI,
					BusNumber:      statusBus,
					UnitNumber:     &statusUnit,
				},
			}

			placement := vmconfsnapshotdisk.GetEffectivePlacement(vol, statusPlacementMap)
			Expect(placement.ControllerType).To(Equal(vmopv1.VirtualControllerTypeNVME))
			Expect(placement.BusNumber).To(Equal(specBus))
			Expect(placement.UnitNumber).To(HaveValue(Equal(specUnit)))
		})

		It("falls back to status placement when spec fields are unset", func() {
			statusUnit := int32(5)
			statusBus := int32(1)

			vol := vmopv1.VirtualMachineVolume{
				Name: "test-vol",
			}
			statusPlacementMap := map[string]vmconfsnapshotdisk.VolumePlacement{
				"test-vol": {
					ControllerType: vmopv1.VirtualControllerTypeSATA,
					BusNumber:      statusBus,
					UnitNumber:     &statusUnit,
				},
			}

			placement := vmconfsnapshotdisk.GetEffectivePlacement(vol, statusPlacementMap)
			Expect(placement.ControllerType).To(Equal(vmopv1.VirtualControllerTypeSATA))
			Expect(placement.BusNumber).To(Equal(statusBus))
			Expect(placement.UnitNumber).To(HaveValue(Equal(statusUnit)))
		})

		It("uses default SCSI and 0 when neither spec nor status specifies controller info", func() {
			vol := vmopv1.VirtualMachineVolume{
				Name: "test-vol",
			}
			statusPlacementMap := map[string]vmconfsnapshotdisk.VolumePlacement{}

			placement := vmconfsnapshotdisk.GetEffectivePlacement(vol, statusPlacementMap)
			Expect(placement.ControllerType).To(Equal(vmopv1.VirtualControllerTypeSCSI))
			Expect(placement.BusNumber).To(Equal(int32(0)))
			Expect(placement.UnitNumber).To(BeNil())
		})
	})
})

var _ = Describe("CreateSnapshotDiskBacking", func() {
	It("returns error for nil disk or nil backing", func() {
		_, err := vmconfsnapshotdisk.CreateSnapshotDiskBacking(nil)
		Expect(err).To(HaveOccurred())

		_, err = vmconfsnapshotdisk.CreateSnapshotDiskBacking(&vimtypes.VirtualDisk{})
		Expect(err).To(HaveOccurred())
	})

	It("handles FlatVer2 backing", func() {
		orig := &vimtypes.VirtualDiskFlatVer2BackingInfo{
			VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: "[ds] test/test.vmdk"},
			Uuid:                         "uuid-123",
		}
		backing, err := vmconfsnapshotdisk.CreateSnapshotDiskBacking(&vimtypes.VirtualDisk{VirtualDevice: vimtypes.VirtualDevice{Backing: orig}})
		Expect(err).ToNot(HaveOccurred())
		flatBacking, ok := backing.(*vimtypes.VirtualDiskFlatVer2BackingInfo)
		Expect(ok).To(BeTrue())
		Expect(flatBacking.DiskMode).To(Equal(string(vimtypes.VirtualDiskModeIndependent_nonpersistent)))
		Expect(flatBacking.Parent).To(Equal(orig))
		Expect(flatBacking.Uuid).To(Equal("uuid-123"))
	})

	It("handles SeSparse backing", func() {
		orig := &vimtypes.VirtualDiskSeSparseBackingInfo{
			VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: "[ds] test/test-sesparse.vmdk"},
			Uuid:                         "uuid-456",
		}
		backing, err := vmconfsnapshotdisk.CreateSnapshotDiskBacking(&vimtypes.VirtualDisk{VirtualDevice: vimtypes.VirtualDevice{Backing: orig}})
		Expect(err).ToNot(HaveOccurred())
		seBacking, ok := backing.(*vimtypes.VirtualDiskSeSparseBackingInfo)
		Expect(ok).To(BeTrue())
		Expect(seBacking.DiskMode).To(Equal(string(vimtypes.VirtualDiskModeIndependent_nonpersistent)))
		Expect(seBacking.Parent).To(Equal(orig))
		Expect(seBacking.Uuid).To(Equal("uuid-456"))
	})

	It("handles SparseVer2 backing", func() {
		orig := &vimtypes.VirtualDiskSparseVer2BackingInfo{
			VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: "[ds] test/test-sparse.vmdk"},
			Uuid:                         "uuid-789",
		}
		backing, err := vmconfsnapshotdisk.CreateSnapshotDiskBacking(&vimtypes.VirtualDisk{VirtualDevice: vimtypes.VirtualDevice{Backing: orig}})
		Expect(err).ToNot(HaveOccurred())
		spBacking, ok := backing.(*vimtypes.VirtualDiskSparseVer2BackingInfo)
		Expect(ok).To(BeTrue())
		Expect(spBacking.DiskMode).To(Equal(string(vimtypes.VirtualDiskModeIndependent_nonpersistent)))
		Expect(spBacking.Parent).To(Equal(orig))
		Expect(spBacking.Uuid).To(Equal("uuid-789"))
	})

	It("handles RawDiskMappingVer1 backing", func() {
		orig := &vimtypes.VirtualDiskRawDiskMappingVer1BackingInfo{
			VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: "[ds] test/test-rdm.vmdk"},
			Uuid:                         "uuid-rdm",
		}
		backing, err := vmconfsnapshotdisk.CreateSnapshotDiskBacking(&vimtypes.VirtualDisk{VirtualDevice: vimtypes.VirtualDevice{Backing: orig}})
		Expect(err).ToNot(HaveOccurred())
		rdmBacking, ok := backing.(*vimtypes.VirtualDiskRawDiskMappingVer1BackingInfo)
		Expect(ok).To(BeTrue())
		Expect(rdmBacking.DiskMode).To(Equal(string(vimtypes.VirtualDiskModeIndependent_nonpersistent)))
		Expect(rdmBacking.Parent).To(Equal(orig))
		Expect(rdmBacking.Uuid).To(Equal("uuid-rdm"))
	})

	It("returns error for unsupported backing type", func() {
		orig := &vimtypes.VirtualDiskRawDiskVer2BackingInfo{
			DescriptorFileName: "desc.vmdk",
		}
		_, err := vmconfsnapshotdisk.CreateSnapshotDiskBacking(&vimtypes.VirtualDisk{VirtualDevice: vimtypes.VirtualDevice{Backing: orig}})
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("GetDiskBackingInfo", func() {
	It("returns nil for nil disk or nil backing", func() {
		Expect(vmconfsnapshotdisk.GetDiskBackingInfo(nil)).To(BeNil())
		Expect(vmconfsnapshotdisk.GetDiskBackingInfo(&vimtypes.VirtualDisk{})).To(BeNil())
	})

	It("returns nil for unsupported backing type", func() {
		disk := &vimtypes.VirtualDisk{
			VirtualDevice: vimtypes.VirtualDevice{
				Backing: &vimtypes.VirtualDiskRawDiskVer2BackingInfo{},
			},
		}
		Expect(vmconfsnapshotdisk.GetDiskBackingInfo(disk)).To(BeNil())
	})

	It("handles FlatVer2 backing", func() {
		orig := &vimtypes.VirtualDiskFlatVer2BackingInfo{
			VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: "[ds] flat.vmdk"},
			DiskMode:                     string(vimtypes.VirtualDiskModePersistent),
			Uuid:                         "flat-uuid",
			Parent: &vimtypes.VirtualDiskFlatVer2BackingInfo{
				VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: "[ds] parent.vmdk"},
			},
		}
		disk := &vimtypes.VirtualDisk{VirtualDevice: vimtypes.VirtualDevice{Backing: orig}}
		info := vmconfsnapshotdisk.GetDiskBackingInfo(disk)
		Expect(info).ToNot(BeNil())
		Expect(info.UUID).To(Equal("flat-uuid"))
		Expect(info.DiskMode).To(Equal(string(vimtypes.VirtualDiskModePersistent)))
		Expect(info.FileName).To(Equal("[ds] flat.vmdk"))
		Expect(info.HasParent).To(BeTrue())
		Expect(info.ParentFileName).To(Equal("[ds] parent.vmdk"))
		Expect(info.CreateChildBacking).ToNot(BeNil())

		child := info.CreateChildBacking()
		flatChild, ok := child.(*vimtypes.VirtualDiskFlatVer2BackingInfo)
		Expect(ok).To(BeTrue())
		Expect(flatChild.DiskMode).To(Equal(string(vimtypes.VirtualDiskModeIndependent_nonpersistent)))
		Expect(flatChild.Parent).To(Equal(orig))
		Expect(flatChild.Uuid).To(Equal("flat-uuid"))
	})

	It("handles SeSparse backing", func() {
		orig := &vimtypes.VirtualDiskSeSparseBackingInfo{
			VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: "[ds] sesparse.vmdk"},
			DiskMode:                     string(vimtypes.VirtualDiskModeIndependent_nonpersistent),
			Uuid:                         "sesparse-uuid",
		}
		disk := &vimtypes.VirtualDisk{VirtualDevice: vimtypes.VirtualDevice{Backing: orig}}
		info := vmconfsnapshotdisk.GetDiskBackingInfo(disk)
		Expect(info).ToNot(BeNil())
		Expect(info.UUID).To(Equal("sesparse-uuid"))
		Expect(info.DiskMode).To(Equal(string(vimtypes.VirtualDiskModeIndependent_nonpersistent)))
		Expect(info.FileName).To(Equal("[ds] sesparse.vmdk"))
		Expect(info.HasParent).To(BeFalse())
	})

	It("handles SparseVer2 backing", func() {
		orig := &vimtypes.VirtualDiskSparseVer2BackingInfo{
			VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: "[ds] sparse.vmdk"},
			DiskMode:                     string(vimtypes.VirtualDiskModePersistent),
			Uuid:                         "sparse-uuid",
		}
		disk := &vimtypes.VirtualDisk{VirtualDevice: vimtypes.VirtualDevice{Backing: orig}}
		info := vmconfsnapshotdisk.GetDiskBackingInfo(disk)
		Expect(info).ToNot(BeNil())
		Expect(info.UUID).To(Equal("sparse-uuid"))
		Expect(info.DiskMode).To(Equal(string(vimtypes.VirtualDiskModePersistent)))
		Expect(info.FileName).To(Equal("[ds] sparse.vmdk"))
		Expect(info.HasParent).To(BeFalse())
	})

	It("handles RawDiskMappingVer1 backing", func() {
		orig := &vimtypes.VirtualDiskRawDiskMappingVer1BackingInfo{
			VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: "[ds] rdm.vmdk"},
			DiskMode:                     string(vimtypes.VirtualDiskModePersistent),
			Uuid:                         "rdm-uuid",
		}
		disk := &vimtypes.VirtualDisk{VirtualDevice: vimtypes.VirtualDevice{Backing: orig}}
		info := vmconfsnapshotdisk.GetDiskBackingInfo(disk)
		Expect(info).ToNot(BeNil())
		Expect(info.UUID).To(Equal("rdm-uuid"))
		Expect(info.DiskMode).To(Equal(string(vimtypes.VirtualDiskModePersistent)))
		Expect(info.FileName).To(Equal("[ds] rdm.vmdk"))
		Expect(info.HasParent).To(BeFalse())
	})
})

var _ = Describe("GetVirtualDiskUUID", func() {
	It("returns empty string for nil disk or nil backing", func() {
		Expect(vmconfsnapshotdisk.GetVirtualDiskUUID(nil)).To(BeEmpty())
		Expect(vmconfsnapshotdisk.GetVirtualDiskUUID(&vimtypes.VirtualDisk{})).To(BeEmpty())
	})

	It("returns uuid for supported backing", func() {
		disk := &vimtypes.VirtualDisk{
			VirtualDevice: vimtypes.VirtualDevice{
				Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{Uuid: "test-uuid"},
			},
		}
		Expect(vmconfsnapshotdisk.GetVirtualDiskUUID(disk)).To(Equal("test-uuid"))
	})
})

var _ = Describe("IsDiskDerivedFromSnapshotDisk", func() {
	It("returns false for nil disk or nil targetDisk", func() {
		disk := &vimtypes.VirtualDisk{
			VirtualDevice: vimtypes.VirtualDevice{
				Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{Uuid: "disk-uuid"},
			},
		}
		Expect(vmconfsnapshotdisk.IsDiskDerivedFromSnapshotDisk(nil, nil)).To(BeFalse())
		Expect(vmconfsnapshotdisk.IsDiskDerivedFromSnapshotDisk(disk, nil)).To(BeFalse())
		Expect(vmconfsnapshotdisk.IsDiskDerivedFromSnapshotDisk(nil, disk)).To(BeFalse())
	})

	It("returns false when disk or targetDisk has no backing", func() {
		disk := &vimtypes.VirtualDisk{}
		targetDisk := &vimtypes.VirtualDisk{}
		Expect(vmconfsnapshotdisk.IsDiskDerivedFromSnapshotDisk(disk, targetDisk)).To(BeFalse())
	})

	It("returns false when UUIDs do not match", func() {
		disk := &vimtypes.VirtualDisk{
			VirtualDevice: vimtypes.VirtualDevice{
				Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
					Uuid:     "uuid-1",
					DiskMode: string(vimtypes.VirtualDiskModeIndependent_nonpersistent),
					Parent:   &vimtypes.VirtualDiskFlatVer2BackingInfo{},
				},
			},
		}
		targetDisk := &vimtypes.VirtualDisk{
			VirtualDevice: vimtypes.VirtualDevice{
				Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
					Uuid: "uuid-2",
				},
			},
		}
		Expect(vmconfsnapshotdisk.IsDiskDerivedFromSnapshotDisk(disk, targetDisk)).To(BeFalse())
	})

	It("returns false when disk mode is persistent (not independent_nonpersistent)", func() {
		disk := &vimtypes.VirtualDisk{
			VirtualDevice: vimtypes.VirtualDevice{
				Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
					Uuid:     "same-uuid",
					DiskMode: string(vimtypes.VirtualDiskModePersistent),
					Parent:   &vimtypes.VirtualDiskFlatVer2BackingInfo{},
				},
			},
		}
		targetDisk := &vimtypes.VirtualDisk{
			VirtualDevice: vimtypes.VirtualDevice{
				Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
					Uuid: "same-uuid",
				},
			},
		}
		Expect(vmconfsnapshotdisk.IsDiskDerivedFromSnapshotDisk(disk, targetDisk)).To(BeFalse())
	})

	It("returns true when parent file name matches target disk file name", func() {
		targetDisk := &vimtypes.VirtualDisk{
			VirtualDevice: vimtypes.VirtualDevice{
				Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
					VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: "[ds] vm/snap.vmdk"},
					Uuid:                         "same-uuid",
				},
			},
		}
		disk := &vimtypes.VirtualDisk{
			VirtualDevice: vimtypes.VirtualDevice{
				Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
					VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: "[ds] vm/child.vmdk"},
					Uuid:                         "same-uuid",
					DiskMode:                     string(vimtypes.VirtualDiskModeIndependent_nonpersistent),
					Parent: &vimtypes.VirtualDiskFlatVer2BackingInfo{
						VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: "[ds] vm/snap.vmdk"},
					},
				},
			},
		}
		Expect(vmconfsnapshotdisk.IsDiskDerivedFromSnapshotDisk(disk, targetDisk)).To(BeTrue())
	})

	It("returns false when parent file name does not match target disk file name", func() {
		targetDisk := &vimtypes.VirtualDisk{
			VirtualDevice: vimtypes.VirtualDevice{
				Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
					VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: "[ds] vm/snap-1.vmdk"},
					Uuid:                         "same-uuid",
				},
			},
		}
		disk := &vimtypes.VirtualDisk{
			VirtualDevice: vimtypes.VirtualDevice{
				Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
					VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: "[ds] vm/child.vmdk"},
					Uuid:                         "same-uuid",
					DiskMode:                     string(vimtypes.VirtualDiskModeIndependent_nonpersistent),
					Parent: &vimtypes.VirtualDiskFlatVer2BackingInfo{
						VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: "[ds] vm/different-snap.vmdk"},
					},
				},
			},
		}
		Expect(vmconfsnapshotdisk.IsDiskDerivedFromSnapshotDisk(disk, targetDisk)).To(BeFalse())
	})

	It("returns false when parent file name or target file name is empty", func() {
		targetDisk := &vimtypes.VirtualDisk{
			VirtualDevice: vimtypes.VirtualDevice{
				Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
					Uuid: "same-uuid",
				},
			},
		}
		disk := &vimtypes.VirtualDisk{
			VirtualDevice: vimtypes.VirtualDevice{
				Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
					Uuid:     "same-uuid",
					DiskMode: string(vimtypes.VirtualDiskModeIndependent_nonpersistent),
					Parent:   &vimtypes.VirtualDiskFlatVer2BackingInfo{},
				},
			},
		}
		Expect(vmconfsnapshotdisk.IsDiskDerivedFromSnapshotDisk(disk, targetDisk)).To(BeFalse())
	})

	It("returns false when disk has no parent and parent file name is empty", func() {
		targetDisk := &vimtypes.VirtualDisk{
			VirtualDevice: vimtypes.VirtualDevice{
				Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
					Uuid: "same-uuid",
				},
			},
		}
		disk := &vimtypes.VirtualDisk{
			VirtualDevice: vimtypes.VirtualDevice{
				Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
					Uuid:     "same-uuid",
					DiskMode: string(vimtypes.VirtualDiskModeIndependent_nonpersistent),
				},
			},
		}
		Expect(vmconfsnapshotdisk.IsDiskDerivedFromSnapshotDisk(disk, targetDisk)).To(BeFalse())
	})
})

var _ = Describe("IsSnapshotDiskAttached", func() {
	var (
		targetDisk *vimtypes.VirtualDisk
		moVM       mo.VirtualMachine
		configSpec *vimtypes.VirtualMachineConfigSpec
	)

	BeforeEach(func() {
		targetDisk = &vimtypes.VirtualDisk{
			VirtualDevice: vimtypes.VirtualDevice{
				Key: 1000,
				Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
					VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: "[ds] snap.vmdk"},
					Uuid:                         "disk-uuid-1",
				},
			},
		}
		configSpec = &vimtypes.VirtualMachineConfigSpec{}
	})

	It("returns false when targetDisk is nil or has no UUID", func() {
		Expect(vmconfsnapshotdisk.IsSnapshotDiskAttached(moVM, configSpec, nil)).To(BeFalse())
		Expect(vmconfsnapshotdisk.IsSnapshotDiskAttached(moVM, configSpec, &vimtypes.VirtualDisk{})).To(BeFalse())
	})

	It("returns false when VM has no matching devices", func() {
		moVM = mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{},
		}
		Expect(vmconfsnapshotdisk.IsSnapshotDiskAttached(moVM, configSpec, targetDisk)).To(BeFalse())
	})

	It("returns true when derived snapshot disk is attached to VM", func() {
		attachedDisk := &vimtypes.VirtualDisk{
			VirtualDevice: vimtypes.VirtualDevice{
				Key: 2000,
				Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
					VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: "[ds] child.vmdk"},
					Uuid:                         "disk-uuid-1",
					DiskMode:                     string(vimtypes.VirtualDiskModeIndependent_nonpersistent),
					Parent: &vimtypes.VirtualDiskFlatVer2BackingInfo{
						VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: "[ds] snap.vmdk"},
					},
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
		Expect(vmconfsnapshotdisk.IsSnapshotDiskAttached(moVM, configSpec, targetDisk)).To(BeTrue())
	})

	It("returns false when VM has same UUID disk but it is regular persistent disk (self-mount case)", func() {
		regularDisk := &vimtypes.VirtualDisk{
			VirtualDevice: vimtypes.VirtualDevice{
				Key: 2000,
				Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
					VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: "[ds] orig.vmdk"},
					Uuid:                         "disk-uuid-1",
					DiskMode:                     string(vimtypes.VirtualDiskModePersistent),
				},
			},
		}
		moVM = mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{
				Hardware: vimtypes.VirtualHardware{
					Device: []vimtypes.BaseVirtualDevice{regularDisk},
				},
			},
		}
		Expect(vmconfsnapshotdisk.IsSnapshotDiskAttached(moVM, configSpec, targetDisk)).To(BeFalse())
	})

	It("returns false when attached snapshot disk is scheduled for removal in configSpec", func() {
		attachedDisk := &vimtypes.VirtualDisk{
			VirtualDevice: vimtypes.VirtualDevice{
				Key: 2000,
				Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
					VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: "[ds] child.vmdk"},
					Uuid:                         "disk-uuid-1",
					DiskMode:                     string(vimtypes.VirtualDiskModeIndependent_nonpersistent),
					Parent: &vimtypes.VirtualDiskFlatVer2BackingInfo{
						VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: "[ds] snap.vmdk"},
					},
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
		configSpec.DeviceChange = append(configSpec.DeviceChange, &vimtypes.VirtualDeviceConfigSpec{
			Operation: vimtypes.VirtualDeviceConfigSpecOperationRemove,
			Device: &vimtypes.VirtualDisk{
				VirtualDevice: vimtypes.VirtualDevice{
					Key: 2000,
					Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
						Uuid: "disk-uuid-1",
					},
				},
			},
		})
		Expect(vmconfsnapshotdisk.IsSnapshotDiskAttached(moVM, configSpec, targetDisk)).To(BeFalse())
	})

	It("returns true when snapshot disk is scheduled for addition in configSpec", func() {
		moVM = mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{},
		}
		newDisk := &vimtypes.VirtualDisk{
			VirtualDevice: vimtypes.VirtualDevice{
				Key: -1,
				Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
					VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: "[ds] child.vmdk"},
					Uuid:                         "disk-uuid-1",
					DiskMode:                     string(vimtypes.VirtualDiskModeIndependent_nonpersistent),
					Parent: &vimtypes.VirtualDiskFlatVer2BackingInfo{
						VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: "[ds] snap.vmdk"},
					},
				},
			},
		}
		configSpec.DeviceChange = append(configSpec.DeviceChange, &vimtypes.VirtualDeviceConfigSpec{
			Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
			Device:    newDisk,
		})
		Expect(vmconfsnapshotdisk.IsSnapshotDiskAttached(moVM, configSpec, targetDisk)).To(BeTrue())
	})
})

var _ = Describe("FindControllerKeyForSnapshotDisk", func() {
	var (
		moVM       mo.VirtualMachine
		configSpec *vimtypes.VirtualMachineConfigSpec
		vol        vmopv1.VirtualMachineVolume
		targetDisk *vimtypes.VirtualDisk
	)

	BeforeEach(func() {
		bus0 := int32(0)
		bus1 := int32(1)
		moVM = mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{
				Hardware: vimtypes.VirtualHardware{
					Device: []vimtypes.BaseVirtualDevice{
						&vimtypes.VirtualLsiLogicSASController{
							VirtualSCSIController: vimtypes.VirtualSCSIController{
								VirtualController: vimtypes.VirtualController{
									VirtualDevice: vimtypes.VirtualDevice{Key: 200},
									BusNumber:     bus0,
								},
							},
						},
						&vimtypes.ParaVirtualSCSIController{
							VirtualSCSIController: vimtypes.VirtualSCSIController{
								VirtualController: vimtypes.VirtualController{
									VirtualDevice: vimtypes.VirtualDevice{Key: 201},
									BusNumber:     bus1,
								},
							},
						},
						&vimtypes.VirtualSATAController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{Key: 300},
								BusNumber:     bus0,
							},
						},
					},
				},
			},
		}
		configSpec = &vimtypes.VirtualMachineConfigSpec{}
		vol = vmopv1.VirtualMachineVolume{Name: "snap-vol"}
		targetDisk = &vimtypes.VirtualDisk{
			VirtualDevice: vimtypes.VirtualDevice{
				Key:           2000,
				ControllerKey: 201,
			},
		}
	})

	It("matches controller by explicit ControllerType and ControllerBusNumber in spec", func() {
		bus := int32(1)
		vol.ControllerType = vmopv1.VirtualControllerTypeSCSI
		vol.ControllerBusNumber = &bus
		key := vmconfsnapshotdisk.FindControllerKeyForSnapshotDisk(moVM, configSpec, vol, targetDisk)
		Expect(key).To(Equal(int32(201)))

		sataBus := int32(0)
		vol.ControllerType = vmopv1.VirtualControllerTypeSATA
		vol.ControllerBusNumber = &sataBus
		key = vmconfsnapshotdisk.FindControllerKeyForSnapshotDisk(moVM, configSpec, vol, targetDisk)
		Expect(key).To(Equal(int32(300)))
	})

	It("matches controller from targetDisk when volume spec does not specify controller", func() {
		key := vmconfsnapshotdisk.FindControllerKeyForSnapshotDisk(moVM, configSpec, vol, targetDisk)
		Expect(key).To(Equal(int32(201)))
	})

	It("defaults to first SCSI controller when targetDisk controller is not on the VM", func() {
		targetDisk.ControllerKey = 9999
		key := vmconfsnapshotdisk.FindControllerKeyForSnapshotDisk(moVM, configSpec, vol, targetDisk)
		Expect(key).To(Equal(int32(200)))
	})

	It("returns 0 when VM has no controllers", func() {
		moVM = mo.VirtualMachine{Config: &vimtypes.VirtualMachineConfigInfo{}}
		key := vmconfsnapshotdisk.FindControllerKeyForSnapshotDisk(moVM, configSpec, vol, targetDisk)
		Expect(key).To(Equal(int32(0)))
	})
})

var _ = Describe("EnsureSnapshotDiskAttached", func() {
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
		}
		moVM = mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{},
		}
		vol = vmopv1.VirtualMachineVolume{
			Name: "snap-vol",
			VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
				VirtualMachineSnapshot: &vmopv1.VirtualMachineSnapshotDiskSpec{
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
		added, err := vmconfsnapshotdisk.EnsureSnapshotDiskAttached(
			ctx, k8sClient, vimClient, vm, moVM, configSpec, vol, &newDeviceKey, cache)
		Expect(err).ToNot(HaveOccurred())
		Expect(added).To(BeTrue())
		Expect(configSpec.DeviceChange).To(HaveLen(1))
	})

	It("returns (false, nil) when snapshot disk is not found (business error)", func() {
		vol.VirtualMachineSnapshot.Name = "missing-snap"
		cache := make(map[string]vmconfsnapshotdisk.SnapshotFetchResult)
		added, err := vmconfsnapshotdisk.EnsureSnapshotDiskAttached(
			ctx, k8sClient, vimClient, vm, moVM, configSpec, vol, &newDeviceKey, cache)
		Expect(err).ToNot(HaveOccurred())
		Expect(added).To(BeFalse())
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
		added, err := vmconfsnapshotdisk.EnsureSnapshotDiskAttached(
			ctx, k8sClient, vimClient, vm, moVM, configSpec, vol, &newDeviceKey, cache)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("connection reset by peer"))
		Expect(added).To(BeFalse())
		Expect(vm.Status.Volumes[0].Error).To(ContainSubstring("connection reset by peer"))
	})
})
