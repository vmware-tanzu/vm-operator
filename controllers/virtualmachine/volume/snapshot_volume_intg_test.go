// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package volume_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachine/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachine/volume"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/test/builder"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	"github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/watcher"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ovfcache"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

func snapshotVolumeIntgTests() {
	var (
		ctx       context.Context
		vcSimCtx  *builder.IntegrationTestContextForVCSim
		initEnvFn builder.InitVCSimEnvFn
		vm        *object.VirtualMachine
		obj       *vmopv1.VirtualMachine
		objKey    client.ObjectKey
		vmName    = "my-vm-snapshot-test"
		snapVolName = "snap-vol"
	)

	BeforeEach(func() {
		virtualmachine.SkipNameValidation = ptr.To(true)
		volume.SkipNameValidation = ptr.To(true)
		ctx = context.Background()
		cfg := pkgcfg.Default()
		cfg.MaxDeployThreadsOnProvider = 1
		cfg.AsyncCreateEnabled = false
		cfg.AsyncSignalEnabled = false
		ctx = pkgcfg.WithContext(ctx, cfg)
		ctx = cource.WithContext(ctx)
		ctx = watcher.WithContext(ctx)
		ctx = ovfcache.WithContext(ctx)
		obj = &vmopv1.VirtualMachine{}
	})

	JustBeforeEach(func() {
		vcSimCtx = builder.NewIntegrationTestContextForVCSim(
			ctx,
			builder.VCSimTestConfig{
				WithContentLibrary: true,
			},
			func(ctx *pkgctx.ControllerManagerContext, mgr ctrlmgr.Manager) error {
				if err := virtualmachine.AddToManager(ctx, mgr); err != nil {
					return err
				}
				return volume.AddToManager(ctx, mgr)
			},
			func(ctx *pkgctx.ControllerManagerContext, mgr ctrlmgr.Manager) error {
				ctx.VMProvider = vsphere.NewVSphereVMProviderFromClient(ctx, mgr.GetClient(), vcSimCtx.Recorder)
				return nil
			},
			initEnvFn)
		Expect(vcSimCtx).ToNot(BeNil())

		vcSimCtx.BeforeEach()

		// Clean up the dummy AZ created by TestSuite.BeforeSuite() to avoid
		// computeCPUMinFrequency failing when it tries to query moID "cluster" in vcsim.
		dummyAZ := builder.DummyAvailabilityZone()
		dummyAZ.Namespace = ""
		_ = vcSimCtx.Client.Delete(ctx, dummyAZ)

		objKey = client.ObjectKey{
			Namespace: vcSimCtx.NSInfo.Namespace,
			Name:      vmName,
		}

		ctx = vcSimCtx
	})

	BeforeEach(func() {
		initEnvFn = func(ctx *builder.IntegrationTestContextForVCSim) {
			vmClass := builder.DummyVirtualMachineClassGenName()
			vmClass.Namespace = ctx.NSInfo.Namespace
			Expect(ctx.Client.Create(ctx, vmClass)).To(Succeed())

			clusterVMI1 := &vmopv1.ClusterVirtualMachineImage{}
			Expect(ctx.Client.Get(
				ctx, client.ObjectKey{Name: ctx.ContentLibraryItem1Name},
				clusterVMI1)).To(Succeed())

			By("creating vm in k8s", func() {
				obj = builder.DummyBasicVirtualMachine(
					vmName,
					ctx.NSInfo.Namespace)
				obj.Spec.ClassName = vmClass.Name
				obj.Spec.ImageName = clusterVMI1.Name
				obj.Spec.Image = &vmopv1.VirtualMachineImageRef{
					Kind: "ClusterVirtualMachineImage",
					Name: clusterVMI1.Name,
				}
				obj.Spec.StorageClass = ctx.StorageClassName
				Expect(ctx.Client.Create(ctx, obj)).To(Succeed())
			})
		}
	})

	AfterEach(func() {
		vcSimCtx.AfterEach()
		vcSimCtx = nil
	})

	Context("VirtualMachineSnapshot volume source", func() {
		var (
			snapCR   *vmopv1.VirtualMachineSnapshot
			diskUUID string
		)

		JustBeforeEach(func() {
			By("waiting for vm to be created and powered on in vcsim", func() {
				Eventually(func(g Gomega) {
					g.Expect(vcSimCtx.Client.Get(ctx, objKey, obj)).To(Succeed())
					g.Expect(obj.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))
					g.Expect(obj.Status.UniqueID).ToNot(BeEmpty())
				}, "30s", "1s").Should(Succeed())

				var err error
				vm, err = vcSimCtx.Finder.VirtualMachine(ctx, obj.Status.UniqueID)
				Expect(err).ToNot(HaveOccurred())
				Expect(vm).ToNot(BeNil())
			})

			By("creating a snapshot in vcsim", func() {
				// Add a dummy disk to the VM so the snapshot has a disk
				dummyDisk := &vimtypes.VirtualDisk{
					VirtualDevice: vimtypes.VirtualDevice{
						Key: -100,
						ControllerKey: 200,
						UnitNumber: ptr.To(int32(0)),
						Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
							VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
								FileName: "[LocalDS_0]",
							},
							Uuid: "dummy-uuid-for-vcsim",
							DiskMode: string(vimtypes.VirtualDiskModePersistent),
						},
					},
					CapacityInBytes: 1024 * 1024,
				}
				
				spec := vimtypes.VirtualMachineConfigSpec{
					DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
						&vimtypes.VirtualDeviceConfigSpec{
							Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
							FileOperation: vimtypes.VirtualDeviceConfigSpecFileOperationCreate,
							Device:    dummyDisk,
						},
					},
				}
				
				task, err := vm.Reconfigure(ctx, spec)
				Expect(err).ToNot(HaveOccurred())
				Expect(task.Wait(ctx)).To(Succeed())

				task, err = vm.CreateSnapshot(ctx, "my-snapshot", "test snapshot", false, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(task.Wait(ctx)).To(Succeed())

				var moVM mo.VirtualMachine
				Eventually(func(g Gomega) {
					g.Expect(vm.Properties(ctx, vm.Reference(), []string{"snapshot", "config.hardware.device"}, &moVM)).To(Succeed())
					g.Expect(moVM.Snapshot).ToNot(BeNil())
					g.Expect(moVM.Snapshot.CurrentSnapshot).ToNot(BeNil())
				}).Should(Succeed())
				snapID := moVM.Snapshot.CurrentSnapshot.Value

				var moSnap mo.VirtualMachineSnapshot
				Expect(vm.Properties(ctx, *moVM.Snapshot.CurrentSnapshot, []string{"config.hardware.device"}, &moSnap)).To(Succeed())
				
				diskUUID = "dummy-uuid-for-vcsim"

				snapCR = &vmopv1.VirtualMachineSnapshot{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-snapshot",
						Namespace: vcSimCtx.NSInfo.Namespace,
					},
				}
				Expect(vcSimCtx.Client.Create(ctx, snapCR)).To(Succeed())

				snapCR.Status.UniqueID = snapID
				snapCR.Status.Disks = []vmopv1.VirtualMachineSnapshotDiskStatus{
					{
						ID: diskUUID,
					},
				}
				conditions.MarkTrue(snapCR, vmopv1.VirtualMachineSnapshotReadyCondition)
				Expect(vcSimCtx.Client.Status().Update(ctx, snapCR)).To(Succeed())
			})
		})

		It("Happy path: attaches snapshot disk successfully", func() {
			By("updating VM to reference the snapshot volume", func() {
				Eventually(func(g Gomega) {
					g.Expect(vcSimCtx.Client.Get(ctx, objKey, obj)).To(Succeed())
					obj.Spec.Volumes = append(obj.Spec.Volumes, vmopv1.VirtualMachineVolume{
						Name:      snapVolName,
						DiskMode:  vmopv1.VolumeDiskModeIndependentNonPersistent,
						Removable: ptr.To(true),
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							VirtualMachineSnapshot: &vmopv1.VirtualMachineSnapshotDiskSpec{
								Name:   "my-snapshot",
								DiskID: diskUUID,
							},
						},
					})
					g.Expect(vcSimCtx.Client.Update(ctx, obj)).To(Succeed())
				}).Should(Succeed())
			})

			By("waiting for volume to be attached", func() {
				Eventually(func(g Gomega) {
					g.Expect(vcSimCtx.Client.Get(ctx, objKey, obj)).To(Succeed())
					var snapVolStatus *vmopv1.VirtualMachineVolumeStatus
					for i, v := range obj.Status.Volumes {
						if v.Name == snapVolName {
							snapVolStatus = &obj.Status.Volumes[i]
							break
						}
					}
					g.Expect(snapVolStatus).ToNot(BeNil(), "Volume status for snap-vol not found in VM status: %v, spec: %v", obj.Status.Volumes, obj.Spec.Volumes)
					g.Expect(snapVolStatus.Attached).To(BeTrue())
					g.Expect(snapVolStatus.DiskUUID).To(Equal(diskUUID))
					g.Expect(snapVolStatus.Error).To(BeEmpty())
				}).Should(Succeed())
			})
		})

		It("Detach / removal: removes snapshot volume successfully", func() {
			By("updating VM to reference the snapshot volume", func() {
				Eventually(func(g Gomega) {
					g.Expect(vcSimCtx.Client.Get(ctx, objKey, obj)).To(Succeed())
					obj.Spec.Volumes = append(obj.Spec.Volumes, vmopv1.VirtualMachineVolume{
						Name:      snapVolName,
						DiskMode:  vmopv1.VolumeDiskModeIndependentNonPersistent,
						Removable: ptr.To(true),
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							VirtualMachineSnapshot: &vmopv1.VirtualMachineSnapshotDiskSpec{
								Name:   "my-snapshot",
								DiskID: diskUUID,
							},
						},
					})
					g.Expect(vcSimCtx.Client.Update(ctx, obj)).To(Succeed())
				}).Should(Succeed())
			})

			By("waiting for volume to be attached", func() {
				Eventually(func(g Gomega) {
					g.Expect(vcSimCtx.Client.Get(ctx, objKey, obj)).To(Succeed())
					var snapVolStatus *vmopv1.VirtualMachineVolumeStatus
					for _, v := range obj.Status.Volumes {
						if v.Name == snapVolName {
							snapVolStatus = &v
							break
						}
					}
					g.Expect(snapVolStatus).ToNot(BeNil(), "Volume status for snap-vol not found in VM status: %v, spec: %v", obj.Status.Volumes, obj.Spec.Volumes)
					g.Expect(snapVolStatus.Attached).To(BeTrue())
				}).Should(Succeed())
			})

			By("removing the snapshot volume from VM spec", func() {
				Eventually(func(g Gomega) {
					g.Expect(vcSimCtx.Client.Get(ctx, objKey, obj)).To(Succeed())
					var newVolumes []vmopv1.VirtualMachineVolume
					for _, v := range obj.Spec.Volumes {
						if v.Name != snapVolName {
							newVolumes = append(newVolumes, v)
						}
					}
					obj.Spec.Volumes = newVolumes
					g.Expect(vcSimCtx.Client.Update(ctx, obj)).To(Succeed())
				}).Should(Succeed())
			})

			By("waiting for volume to be detached and removed from status", func() {
				Eventually(func(g Gomega) {
					g.Expect(vcSimCtx.Client.Get(ctx, objKey, obj)).To(Succeed())
					for _, v := range obj.Status.Volumes {
						g.Expect(v.Name).ToNot(Equal(snapVolName))
					}
				}).Should(Succeed())
			})
		})

		It("Snapshot not found: sets error in status", func() {
			By("updating VM to reference a non-existent snapshot", func() {
				Eventually(func(g Gomega) {
					g.Expect(vcSimCtx.Client.Get(ctx, objKey, obj)).To(Succeed())
					obj.Spec.Volumes = append(obj.Spec.Volumes, vmopv1.VirtualMachineVolume{
						Name:      "snap-vol-not-found",
						DiskMode:  vmopv1.VolumeDiskModeIndependentNonPersistent,
						Removable: ptr.To(true),
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							VirtualMachineSnapshot: &vmopv1.VirtualMachineSnapshotDiskSpec{
								Name:   "missing-snapshot",
								DiskID: diskUUID,
							},
						},
					})
					g.Expect(vcSimCtx.Client.Update(ctx, obj)).To(Succeed())
				}).Should(Succeed())
			})

			By("waiting for error in volume status", func() {
				Eventually(func(g Gomega) {
					g.Expect(vcSimCtx.Client.Get(ctx, objKey, obj)).To(Succeed())
					var snapVolStatus *vmopv1.VirtualMachineVolumeStatus
					for i, v := range obj.Status.Volumes {
						if v.Name == "snap-vol-not-found" {
							snapVolStatus = &obj.Status.Volumes[i]
							break
						}
					}
					g.Expect(snapVolStatus).ToNot(BeNil(), "Volume status for snap-vol-not-found not found in VM status: %v, spec: %v", obj.Status.Volumes, obj.Spec.Volumes)
					g.Expect(snapVolStatus.Attached).To(BeFalse())
					g.Expect(snapVolStatus.Error).To(ContainSubstring("not found"))
				}).Should(Succeed())
			})
		})

		It("Snapshot not yet ready: sets error in status", func() {
			By("marking snapshot as not ready", func() {
				Eventually(func(g Gomega) {
					g.Expect(vcSimCtx.Client.Get(ctx, client.ObjectKeyFromObject(snapCR), snapCR)).To(Succeed())
					conditions.MarkFalse(snapCR, vmopv1.VirtualMachineSnapshotReadyCondition, "NotReady", "Not ready")
					g.Expect(vcSimCtx.Client.Status().Update(ctx, snapCR)).To(Succeed())
				}).Should(Succeed())
			})

			By("updating VM to reference the unready snapshot", func() {
				Eventually(func(g Gomega) {
					g.Expect(vcSimCtx.Client.Get(ctx, objKey, obj)).To(Succeed())
					obj.Spec.Volumes = append(obj.Spec.Volumes, vmopv1.VirtualMachineVolume{
						Name:      "snap-vol-unready",
						DiskMode:  vmopv1.VolumeDiskModeIndependentNonPersistent,
						Removable: ptr.To(true),
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							VirtualMachineSnapshot: &vmopv1.VirtualMachineSnapshotDiskSpec{
								Name:   "my-snapshot",
								DiskID: diskUUID,
							},
						},
					})
					g.Expect(vcSimCtx.Client.Update(ctx, obj)).To(Succeed())
				}).Should(Succeed())
			})

			By("waiting for error in volume status", func() {
				Eventually(func(g Gomega) {
					g.Expect(vcSimCtx.Client.Get(ctx, objKey, obj)).To(Succeed())
					var snapVolStatus *vmopv1.VirtualMachineVolumeStatus
					for i, v := range obj.Status.Volumes {
						if v.Name == "snap-vol-unready" {
							snapVolStatus = &obj.Status.Volumes[i]
							break
						}
					}
					g.Expect(snapVolStatus).ToNot(BeNil(), "Volume status for snap-vol-unready not found in VM status: %v, spec: %v", obj.Status.Volumes, obj.Spec.Volumes)
					g.Expect(snapVolStatus.Attached).To(BeFalse())
					g.Expect(snapVolStatus.Error).To(ContainSubstring("is not ready"))
				}).Should(Succeed())
			})
		})

		It("Unknown diskID: sets error in status", func() {
			By("updating VM to reference an unknown diskID", func() {
				Eventually(func(g Gomega) {
					g.Expect(vcSimCtx.Client.Get(ctx, objKey, obj)).To(Succeed())
					obj.Spec.Volumes = append(obj.Spec.Volumes, vmopv1.VirtualMachineVolume{
						Name:      "snap-vol-unknown-disk",
						DiskMode:  vmopv1.VolumeDiskModeIndependentNonPersistent,
						Removable: ptr.To(true),
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							VirtualMachineSnapshot: &vmopv1.VirtualMachineSnapshotDiskSpec{
								Name:   "my-snapshot",
								DiskID: "unknown-disk-id",
							},
						},
					})
					g.Expect(vcSimCtx.Client.Update(ctx, obj)).To(Succeed())
				}).Should(Succeed())
			})

			By("waiting for error in volume status", func() {
				Eventually(func(g Gomega) {
					g.Expect(vcSimCtx.Client.Get(ctx, objKey, obj)).To(Succeed())
					var snapVolStatus *vmopv1.VirtualMachineVolumeStatus
					for i, v := range obj.Status.Volumes {
						if v.Name == "snap-vol-unknown-disk" {
							snapVolStatus = &obj.Status.Volumes[i]
							break
						}
					}
					g.Expect(snapVolStatus).ToNot(BeNil(), "Volume status for snap-vol-unknown-disk not found in VM status: %v, spec: %v", obj.Status.Volumes, obj.Spec.Volumes)
					g.Expect(snapVolStatus.Attached).To(BeFalse())
					g.Expect(snapVolStatus.Error).To(ContainSubstring("not found in VirtualMachineSnapshot"))
				}).Should(Succeed())
			})
		})
	})
}
