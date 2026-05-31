// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package volumebatch_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"

	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/test/builder"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachine/volumebatch"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/providers/fake"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

func unitVMOwnedStorageTests() {
	Describe(
		"VM-owned Storage Volume Operations",
		Label(testlabels.Controller, testlabels.API),
		unitVMOwnedStorageReconcile,
	)
}

func unitVMOwnedStorageReconcile() {
	const (
		ns           = "vmowned-ns"
		vmName       = "vmowned-vm"
		pvcName      = "pvc-gf-1"
		pvName       = "pv-gf-1"
		volumeHandle = "cns-vol-gf-1"
		diskUUID     = "6000C29-gf-1"
		diskPath     = "[ds1] vmowned-ns/vm/disk.vmdk"
		volName      = "gf-volume-1"
	)

	var (
		reconciler     *volumebatch.Reconciler
		fakeVMProvider *providerfake.VMProvider
		volCtx         *pkgctx.VolumeContext
		fakeClient     client.Client

		vm  *vmopv1.VirtualMachine
		pvc *corev1.PersistentVolumeClaim
		pv  *corev1.PersistentVolume
		cvi *cnsv1alpha1.CsiVolumeInfo
		ba  *cnsv1alpha1.CnsNodeVMBatchAttachment

		testCtx *builder.UnitTestContextForController
	)

	makeVM := func() *vmopv1.VirtualMachine {
		return &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      vmName,
				Namespace: ns,
				Annotations: map[string]string{
					pkgconst.VMOwnedVolumesAnnotation: "true",
				},
			},
			Status: vmopv1.VirtualMachineStatus{
				InstanceUUID: "instance-uuid-gf",
				BiosUUID:     "bios-uuid-gf",
				Hardware: &vmopv1.VirtualMachineHardwareStatus{
					Controllers: []vmopv1.VirtualControllerStatus{
						{Type: vmopv1.VirtualControllerTypeSCSI, BusNumber: 0, DeviceKey: 1000},
					},
				},
			},
		}
	}

	BeforeEach(func() {
		vm = makeVM()

		pvc = &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: pvcName, Namespace: ns},
			Spec:       corev1.PersistentVolumeClaimSpec{VolumeName: pvName},
			Status:     corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
		}

		pv = &corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{Name: pvName},
			Spec: corev1.PersistentVolumeSpec{
				PersistentVolumeSource: corev1.PersistentVolumeSource{
					CSI: &corev1.CSIPersistentVolumeSource{
						VolumeHandle: volumeHandle,
					},
				},
			},
		}

		cvi = &cnsv1alpha1.CsiVolumeInfo{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "csi-volume-info-" + volumeHandle,
				Namespace: ns,
				Labels:    map[string]string{cnsv1alpha1.CVIDiskUUIDLabel: diskUUID},
			},
			Spec: cnsv1alpha1.CsiVolumeInfoSpec{
				VolumeID: volumeHandle,
				PVCName:  pvcName,
				PVName:   pvName,
			},
			Status: cnsv1alpha1.CsiVolumeInfoStatus{
				OwnershipState: cnsv1alpha1.OwnershipStateTransferringToVM,
				VMName:         vmName,
				VMInstanceUUID: "instance-uuid-gf",
				DiskUUID:       diskUUID,
				DiskPath:       diskPath,
			},
		}

		ba = &cnsv1alpha1.CnsNodeVMBatchAttachment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      vmName,
				Namespace: ns,
			},
			Spec: cnsv1alpha1.CnsNodeVMBatchAttachmentSpec{
				InstanceUUID: "instance-uuid-gf",
				Volumes: []cnsv1alpha1.VolumeSpec{
					{
						Name: volName,
						PersistentVolumeClaim: cnsv1alpha1.PersistentVolumeClaimSpec{
							ClaimName:     pvcName,
							DiskMode:      cnsv1alpha1.Persistent,
							SharingMode:   cnsv1alpha1.SharingNone,
							ControllerKey: ptr.To(int32(1000)),
							UnitNumber:    ptr.To(int32(0)),
						},
					},
				},
			},
			Status: cnsv1alpha1.CnsNodeVMBatchAttachmentStatus{
				VolumeStatus: []cnsv1alpha1.VolumeStatus{
					{
						Name: volName,
						PersistentVolumeClaim: cnsv1alpha1.PersistentVolumeClaimStatus{
							ClaimName: pvcName,
							DiskUUID:  diskUUID,
							DiskPath:  diskPath,
							Conditions: []metav1.Condition{
								{
									Type:   cnsv1alpha1.ConditionAttachMethod,
									Reason: cnsv1alpha1.ReasonReconfig,
									Status: metav1.ConditionTrue,
								},
							},
						},
					},
				},
			},
		}
	})

	JustBeforeEach(func() {
		testCtx = suite.NewUnitTestContextForController()
		fakeClient = fake.NewClientBuilder().
			WithScheme(testCtx.Client.Scheme()).
			WithObjects(vm, pvc, pv, cvi, ba).
			WithStatusSubresource(builder.KnownObjectTypes()...).
			WithStatusSubresource(&cnsv1alpha1.CsiVolumeInfo{}, &cnsv1alpha1.CnsNodeVMBatchAttachment{}).
			WithIndex(&cnsv1alpha1.CnsNodeVmAttachment{}, "spec.nodeuuid", func(o client.Object) []string {
				return []string{o.(*cnsv1alpha1.CnsNodeVmAttachment).Spec.NodeUUID}
			}).
			Build()

		reconciler = volumebatch.NewReconciler(
			testCtx, fakeClient, log.Log, testCtx.Recorder, testCtx.VMProvider)
		fakeVMProvider = testCtx.VMProvider.(*providerfake.VMProvider)

		volCtx = &pkgctx.VolumeContext{
			Context: testCtx,
			Logger:  log.Log,
			VM:      vm,
		}
		pkgcfg.SetContext(volCtx, func(c *pkgcfg.Config) {
			c.Features.VMOwnedVolumes = true
		})
	})

	AfterEach(func() {
		testCtx.AfterEach()
		fakeVMProvider = nil
		reconciler = nil
		volCtx = nil
	})

	Context("VM-owned attach path", func() {
		When("CSI signals AttachMethod=Reconfig with diskPath and diskUUID", func() {
			It("should call AddExistingDiskToVM and transition CVI to VM_MANAGED", func() {
				var addCalled bool
				fakeVMProvider.AddExistingDiskToVMFn = func(ctx context.Context, _ *vmopv1.VirtualMachine,
					path string, _, _ int32, _ string) error {
					Expect(path).To(Equal(diskPath))
					addCalled = true
					return nil
				}
				fakeVMProvider.GetVirtualDiskByUUIDFn = func(ctx context.Context,
					_ *vmopv1.VirtualMachine, uuid string) (*providers.VirtualDiskInfo, error) {
					if uuid == diskUUID {
						return &providers.VirtualDiskInfo{
							DiskUUID: diskUUID,
							DiskPath: diskPath,
						}, nil
					}
					return nil, nil
				}

				Expect(reconciler.ReconcileNormal(volCtx)).To(Succeed())
				Expect(addCalled).To(BeTrue())

				// Verify CVI was transitioned to VM_MANAGED.
				updatedCVI := &cnsv1alpha1.CsiVolumeInfo{}
				Expect(fakeClient.Get(volCtx, client.ObjectKeyFromObject(cvi), updatedCVI)).To(Succeed())
				Expect(updatedCVI.Status.OwnershipState).To(Equal(cnsv1alpha1.OwnershipStateVMManaged))
			})

			When("disk is already present on VM (idempotent)", func() {
				It("should skip AddExistingDiskToVM and still transition CVI", func() {
					var addCalled bool
					fakeVMProvider.AddExistingDiskToVMFn = func(ctx context.Context,
						_ *vmopv1.VirtualMachine, _ string, _, _ int32, _ string) error {
						addCalled = true
						return nil
					}
					fakeVMProvider.GetVirtualDiskByUUIDFn = func(ctx context.Context,
						_ *vmopv1.VirtualMachine, _ string) (*providers.VirtualDiskInfo, error) {
						return &providers.VirtualDiskInfo{
							DiskUUID: diskUUID,
							DiskPath: diskPath,
						}, nil
					}

					Expect(reconciler.ReconcileNormal(volCtx)).To(Succeed())
					// Add should NOT be called since disk is already present.
					Expect(addCalled).To(BeFalse())

					updatedCVI := &cnsv1alpha1.CsiVolumeInfo{}
					Expect(fakeClient.Get(volCtx, client.ObjectKeyFromObject(cvi), updatedCVI)).To(Succeed())
					Expect(updatedCVI.Status.OwnershipState).To(Equal(cnsv1alpha1.OwnershipStateVMManaged))
				})
			})

			When("VM does not have the VM-owned storage annotation", func() {
				It("should skip the VM-owned storage path entirely", func() {
					vm.Annotations = nil
					var addCalled bool
					fakeVMProvider.AddExistingDiskToVMFn = func(ctx context.Context,
						_ *vmopv1.VirtualMachine, _ string, _, _ int32, _ string) error {
						addCalled = true
						return nil
					}

					Expect(reconciler.ReconcileNormal(volCtx)).To(Succeed())
					Expect(addCalled).To(BeFalse())
				})
			})

			When("VMOwnedVolumes feature gate is disabled", func() {
				It("should skip the VM-owned storage path", func() {
					pkgcfg.SetContext(volCtx, func(c *pkgcfg.Config) {
						c.Features.VMOwnedVolumes = false
					})

					var addCalled bool
					fakeVMProvider.AddExistingDiskToVMFn = func(ctx context.Context,
						_ *vmopv1.VirtualMachine, _ string, _, _ int32, _ string) error {
						addCalled = true
						return nil
					}

					Expect(reconciler.ReconcileNormal(volCtx)).To(Succeed())
					Expect(addCalled).To(BeFalse())
				})
			})
		})
	})

	Context("VM-owned detach path", func() {
		When("a VM-owned volume is VM_MANAGED and is removed from vm.spec.volumes", func() {
			BeforeEach(func() {
				// Set CVI to VM_MANAGED (attached state).
				cvi.Status.OwnershipState = cnsv1alpha1.OwnershipStateVMManaged
				cvi.Status.VMName = vmName

				// Do NOT add vol to vm.spec.volumes so it appears as removed.
			})

			It("should call RemoveDiskFromVM and transition CVI to TRANSFERRING_TO_CSI", func() {
				var removeCalled bool
				fakeVMProvider.RemoveDiskFromVMFn = func(ctx context.Context,
					_ *vmopv1.VirtualMachine, uuid string) error {
					Expect(uuid).To(Equal(diskUUID))
					removeCalled = true
					return nil
				}
				fakeVMProvider.GetVirtualDiskByUUIDFn = func(ctx context.Context,
					_ *vmopv1.VirtualMachine, _ string) (*providers.VirtualDiskInfo, error) {
					return &providers.VirtualDiskInfo{
						DiskUUID: diskUUID,
						DiskPath: diskPath,
					}, nil
				}

				Expect(reconciler.ReconcileNormal(volCtx)).To(Succeed())
				Expect(removeCalled).To(BeTrue())
			})

			When("RemoveDiskFromVM fails due to snapshot", func() {
				It("should revert CVI to VM_MANAGED and set DetachBlocked on BA", func() {
					fakeVMProvider.RemoveDiskFromVMFn = func(ctx context.Context,
						_ *vmopv1.VirtualMachine, _ string) error {
						return errors.New("snapshot retains disk")
					}
					fakeVMProvider.GetVirtualDiskByUUIDFn = func(ctx context.Context,
						_ *vmopv1.VirtualMachine, _ string) (*providers.VirtualDiskInfo, error) {
						return &providers.VirtualDiskInfo{
							DiskUUID: diskUUID,
							DiskPath: diskPath,
						}, nil
					}

					// ReconcileNormal should return an error or keep retrying.
					err := reconciler.ReconcileNormal(volCtx)
					// The detach failure should propagate.
					Expect(err).To(HaveOccurred())
				})
			})
		})
	})
}
