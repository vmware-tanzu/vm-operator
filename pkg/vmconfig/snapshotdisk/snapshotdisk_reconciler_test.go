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
				_ = r.Reconcile(nil, k8sClient, vimClient, vm, moVM, configSpec)
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
			Expect(err).ToNot(HaveOccurred())

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

var _ = Describe("RemoveDetachedSnapshotDisks", func() {
	It("adds remove DeviceChange for detached snapshot disks", func() {
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

		snapshotVolumes := []vmopv1.VirtualMachineVolume{}
		configSpec := &vimtypes.VirtualMachineConfigSpec{}
		vm := &vmopv1.VirtualMachine{}

		vmconfsnapshotdisk.RemoveDetachedSnapshotDisks(moVM, vm, snapshotVolumes, configSpec)

		Expect(configSpec.DeviceChange).To(HaveLen(1))
		change := configSpec.DeviceChange[0].GetVirtualDeviceConfigSpec()
		Expect(change.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationRemove))
		Expect(change.Device.GetVirtualDevice().Key).To(Equal(int32(2000)))
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
