// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package volumebatch_test

import (
	"context"
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	storagehelpers "k8s.io/component-helpers/storage/volume"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/log"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/test/builder"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachine/volumebatch"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/providers/fake"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

func unitTests() {
	Describe(
		"Volume Batch Controller",
		Label(
			testlabels.Controller,
			testlabels.API,
		),
		unitTestsReconcile,
	)
}

func unitTestsReconcile() {
	const (
		ns            = "dummy-ns"
		dummyBiosUUID = "dummy-bios-uuid"
	)
	var (
		reconciler     *volumebatch.Reconciler
		initObjects    []client.Object
		withFuncs      interceptor.Funcs
		ctx            *builder.UnitTestContextForController
		fakeVMProvider *providerfake.VMProvider
		volCtx         *pkgctx.VolumeContext

		vm               *vmopv1.VirtualMachine
		vmVol            *vmopv1.VirtualMachineVolume
		vmVolumeWithPVC1 *vmopv1.VirtualMachineVolume
		boundPVC1        *corev1.PersistentVolumeClaim
	)

	BeforeEach(func() {

		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-vm",
				Namespace: ns,
			},
			Status: vmopv1.VirtualMachineStatus{
				BiosUUID: dummyBiosUUID,
			},
		}

		vmVolumeWithPVC1 = &vmopv1.VirtualMachineVolume{
			Name: "cns-volume-1",
			VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
				PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
					PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: "pvc-volume-1",
					},
				},
			},
		}

		boundPVC1 = &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      vmVolumeWithPVC1.VirtualMachineVolumeSource.PersistentVolumeClaim.ClaimName,
				Namespace: ns,
			},
			Status: corev1.PersistentVolumeClaimStatus{
				Phase: corev1.ClaimBound,
			},
		}
	})

	JustBeforeEach(func() {

		ctx = suite.NewUnitTestContextForController()

		// Replace the fake client with our own that has the expected index.
		ctx.Client = fake.NewClientBuilder().
			WithScheme(ctx.Client.Scheme()).
			WithObjects(initObjects...).
			WithInterceptorFuncs(withFuncs).
			WithStatusSubresource(builder.KnownObjectTypes()...).
			Build()

		reconciler = volumebatch.NewReconciler(
			ctx,
			ctx.Client,
			ctx.Logger,
			ctx.Recorder,
			ctx.VMProvider,
		)
		fakeVMProvider = ctx.VMProvider.(*providerfake.VMProvider)

		volCtx = &pkgctx.VolumeContext{
			Context: ctx,
			Logger:  log.Log,
			VM:      vm,
		}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		initObjects = nil
		withFuncs = interceptor.Funcs{}
		reconciler = nil
		volCtx = nil
		vm = nil
		vmVol = nil
		vmVolumeWithPVC1 = nil
	})

	Context("ReconcileNormal", func() {
		When("VM does not have BiosUUID", func() {
			BeforeEach(func() {
				vmVol = vmVolumeWithPVC1
				vm.Spec.Volumes = append(vm.Spec.Volumes, *vmVol)
				vm.Status.BiosUUID = ""
			})

			It("returns success", func() {
				err := reconciler.ReconcileNormal(volCtx)
				Expect(err).ToNot(HaveOccurred())

				By("Did not create CnsNodeVmBatchAttachment", func() {
					Expect(getCNSBatchAttachmentForVolumeName(ctx, vm)).To(BeNil())
					Expect(vm.Status.Volumes).To(BeEmpty())
				})
			})
		})

		When("VM Spec.Volumes is empty", func() {
			It("returns success", func() {
				err := reconciler.ReconcileNormal(volCtx)
				Expect(err).ToNot(HaveOccurred())
				Expect(vm.Spec.Volumes).To(BeEmpty())
				Expect(vm.Status.Volumes).To(BeEmpty())
			})
		})

		When("VM Spec.Volumes is not empty", func() {
			BeforeEach(func() {
				initObjects = append(initObjects, boundPVC1)

				vmVol = vmVolumeWithPVC1
				vm.Spec.Volumes = append(vm.Spec.Volumes, *vmVol)
			})

			When("hardware failed to get VM hardware version", func() {
				JustBeforeEach(func() {
					fakeVMProvider.GetVirtualMachineHardwareVersionFn = func(_ context.Context, _ *vmopv1.VirtualMachine) (vimtypes.HardwareVersion, error) {
						return 0, errors.New("dummy-error")
					}
				})

				It("returns error", func() {
					err := reconciler.ReconcileNormal(volCtx)
					Expect(err).To(HaveOccurred())

					attachment := getCNSBatchAttachmentForVolumeName(ctx, vm)
					Expect(attachment).To(BeNil())
					Expect(vm.Status.Volumes).To(BeEmpty())
				})
			})

			When("VM hardware version is smaller than minimal requirement", func() {
				JustBeforeEach(func() {
					fakeVMProvider.GetVirtualMachineHardwareVersionFn = func(_ context.Context, _ *vmopv1.VirtualMachine) (vimtypes.HardwareVersion, error) {
						return vimtypes.VMX11, nil
					}
				})

				It("returns error", func() {
					err := reconciler.ReconcileNormal(volCtx)
					Expect(err).To(HaveOccurred())

					attachment := getCNSBatchAttachmentForVolumeName(ctx, vm)
					Expect(attachment).To(BeNil())
					Expect(vm.Status.Volumes).To(BeEmpty())
				})
			})

			When("failed to parse VM hardware version", func() {
				JustBeforeEach(func() {
					fakeVMProvider.GetVirtualMachineHardwareVersionFn = func(_ context.Context, _ *vmopv1.VirtualMachine) (vimtypes.HardwareVersion, error) {
						return 0, nil
					}
				})

				It("succeeds", func() {
					err := reconciler.ReconcileNormal(volCtx)
					Expect(err).ToNot(HaveOccurred())
				})
			})

			When("there is basic PVC", func() {
				It("should build batchAttachment and fill vm volume status", func() {
					err := reconciler.ReconcileNormal(volCtx)
					Expect(err).NotTo(HaveOccurred())

					attachment := getCNSBatchAttachmentForVolumeName(ctx, vm)

					Expect(attachment).NotTo(BeNil())
					Expect(attachment.Spec.Volumes).To(HaveLen(1))

					attVol1 := attachment.Spec.Volumes[0]
					Expect(attVol1.Name).To(Equal("cns-volume-1"))
					Expect(attVol1.PersistentVolumeClaim.ClaimName).To(Equal("pvc-volume-1"))
				})
			})

			When("volumes are already tracked by legacy CnsNodeVmAttachment", func() {
				var (
					vmVolumeWithPVC2 *vmopv1.VirtualMachineVolume
					boundPVC2        *corev1.PersistentVolumeClaim
					legacyAttachment *cnsv1alpha1.CnsNodeVmAttachment
				)

				BeforeEach(func() {
					// Create a second volume.  This one will be tracked by batch attach.
					vmVolumeWithPVC2 = &vmopv1.VirtualMachineVolume{
						Name: "cns-volume-2",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "pvc-volume-2",
								},
							},
						},
					}

					boundPVC2 = &corev1.PersistentVolumeClaim{
						ObjectMeta: metav1.ObjectMeta{
							Name:      vmVolumeWithPVC2.VirtualMachineVolumeSource.PersistentVolumeClaim.ClaimName,
							Namespace: ns,
						},
						Status: corev1.PersistentVolumeClaimStatus{
							Phase: corev1.ClaimBound,
						},
					}

					// Create a CnsNodeVmAttachment for the first volume to simulate brownfield attached volume.
					legacyAttachment = &cnsv1alpha1.CnsNodeVmAttachment{
						ObjectMeta: metav1.ObjectMeta{
							Name:      util.CNSAttachmentNameForVolume(vm.Name, vmVolumeWithPVC1.Name), // "dummy-vm-cns-volume-1"
							Namespace: ns,
						},
						Spec: cnsv1alpha1.CnsNodeVmAttachmentSpec{
							NodeUUID:   dummyBiosUUID,
							VolumeName: vmVolumeWithPVC1.VirtualMachineVolumeSource.PersistentVolumeClaim.ClaimName,
						},
						Status: cnsv1alpha1.CnsNodeVmAttachmentStatus{
							Attached: true,
						},
					}

					// Add both volumes to the VM spec
					vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{*vmVolumeWithPVC1, *vmVolumeWithPVC2}

					initObjects = append(initObjects, boundPVC2, legacyAttachment)
				})

				It("should exclude legacy-tracked volumes from batch attachment", func() {
					err := reconciler.ReconcileNormal(volCtx)
					Expect(err).NotTo(HaveOccurred())

					attachment := getCNSBatchAttachmentForVolumeName(ctx, vm)

					// Batch attachment should be created only for the greenfield volume
					Expect(attachment).NotTo(BeNil())
					Expect(attachment.Spec.Volumes).To(HaveLen(1))
					attVol := attachment.Spec.Volumes[0]
					Expect(attVol.Name).To(Equal("cns-volume-2"))
					Expect(attVol.PersistentVolumeClaim.ClaimName).To(Equal("pvc-volume-2"))

					// Verify the legacy attachment still exists.
					// TODO: AKP: This will be removed once we handle orphaned CnsNodeVmAttachments removal.
					legacyAttachmentKey := client.ObjectKey{
						Name:      util.CNSAttachmentNameForVolume(vm.Name, vmVolumeWithPVC1.Name),
						Namespace: ns,
					}
					existingLegacyAttachment := &cnsv1alpha1.CnsNodeVmAttachment{}
					err = ctx.Client.Get(ctx, legacyAttachmentKey, existingLegacyAttachment)
					Expect(err).NotTo(HaveOccurred())
					Expect(existingLegacyAttachment.Spec.VolumeName).To(Equal("pvc-volume-1"))
				})

				When("all volumes are tracked by legacy attachments", func() {
					BeforeEach(func() {
						// Create a legacy attachment for the second volume too
						legacyAttachment2 := &cnsv1alpha1.CnsNodeVmAttachment{
							ObjectMeta: metav1.ObjectMeta{
								Name:      util.CNSAttachmentNameForVolume(vm.Name, vmVolumeWithPVC2.Name), // "dummy-vm-cns-volume-2"
								Namespace: ns,
							},
							Spec: cnsv1alpha1.CnsNodeVmAttachmentSpec{
								NodeUUID:   dummyBiosUUID,
								VolumeName: vmVolumeWithPVC2.VirtualMachineVolumeSource.PersistentVolumeClaim.ClaimName,
							},
							Status: cnsv1alpha1.CnsNodeVmAttachmentStatus{
								Attached: true,
							},
						}
						initObjects = append(initObjects, legacyAttachment2)
					})

					It("should not create batch attachment when all volumes are handled by legacy attachments", func() {
						err := reconciler.ReconcileNormal(volCtx)
						Expect(err).NotTo(HaveOccurred())

						// No batch attachment should be created since all volumes are legacy-tracked
						attachment := getCNSBatchAttachmentForVolumeName(ctx, vm)
						Expect(attachment).To(BeNil())
					})
				})

				When("legacy attachment has different NodeUUID", func() {
					BeforeEach(func() {
						legacyAttachment.Spec.NodeUUID = "stale-bios-uuid"
					})

					It("should treat volume as greenfield and include in batch", func() {
						err := reconciler.ReconcileNormal(volCtx)
						Expect(err).NotTo(HaveOccurred())

						attachment := getCNSBatchAttachmentForVolumeName(ctx, vm)

						// Both volumes should be in the batch since
						// the legacy attachment's node UUID doesn't
						// match this VM's BIOS UUID.
						Expect(attachment).NotTo(BeNil())
						Expect(attachment.Spec.Volumes).To(HaveLen(2))

						volumeNames := make([]string, len(attachment.Spec.Volumes))
						for i, vol := range attachment.Spec.Volumes {
							volumeNames[i] = vol.Name
						}
						Expect(volumeNames).To(ConsistOf("cns-volume-1", "cns-volume-2"))
					})
				})

				When("legacy attachment has different PVC name", func() {
					BeforeEach(func() {
						// Make the legacy attachment point to a different PVC
						legacyAttachment.Spec.VolumeName = "different-pvc"
					})

					It("should treat volume as greenfield and include in batch", func() {
						err := reconciler.ReconcileNormal(volCtx)
						Expect(err).NotTo(HaveOccurred())

						attachment := getCNSBatchAttachmentForVolumeName(ctx, vm)

						// Both volumes should be in the batch since the legacy attachment points to different PVC
						Expect(attachment).NotTo(BeNil())
						Expect(attachment.Spec.Volumes).To(HaveLen(2), "Both volumes should be treated as greenfield when legacy attachment has different PVC")

						volumeNames := make([]string, len(attachment.Spec.Volumes))
						for i, vol := range attachment.Spec.Volumes {
							volumeNames[i] = vol.Name
						}
						Expect(volumeNames).To(ConsistOf("cns-volume-1", "cns-volume-2"))
					})
				})
			})

			When("there is a PVC with application type: Oracle RAC", func() {
				BeforeEach(func() {
					vm.Spec.Volumes[0].PersistentVolumeClaim.ApplicationType = vmopv1.VolumeApplicationTypeOracleRAC // This sets IndependentPersistent + MultiWriter
					vm.Spec.Volumes[0].PersistentVolumeClaim.DiskMode = vmopv1.VolumeDiskModePersistent              // Override to Persistent
					vm.Spec.Volumes[0].PersistentVolumeClaim.SharingMode = vmopv1.VolumeSharingModeNone              // Override to None
				})

				It("sets volme variables correctly", func() {
					err := reconciler.ReconcileNormal(volCtx)
					Expect(err).NotTo(HaveOccurred())

					attachment := getCNSBatchAttachmentForVolumeName(ctx, vm)

					Expect(attachment).NotTo(BeNil())
					Expect(attachment.Spec.Volumes).To(HaveLen(1))

					// Explicit settings should override application type presets
					attVol1 := attachment.Spec.Volumes[0]
					Expect(attVol1.PersistentVolumeClaim.DiskMode).To(Equal(cnsv1alpha1.Persistent))
					Expect(attVol1.PersistentVolumeClaim.SharingMode).To(Equal(cnsv1alpha1.SharingNone))
				})
			})

			When("controller type and bus number are set", func() {
				BeforeEach(func() {
					vm.Spec.Volumes[0].PersistentVolumeClaim.ControllerType = vmopv1.VirtualControllerTypeSCSI
					vm.Spec.Volumes[0].PersistentVolumeClaim.ControllerBusNumber = ptr.To[int32](1)
					vm.Spec.Volumes[0].PersistentVolumeClaim.UnitNumber = ptr.To[int32](5)
				})

				It("sets volme variables correctly", func() {
					err := reconciler.ReconcileNormal(volCtx)
					Expect(err).NotTo(HaveOccurred())

					attachment := getCNSBatchAttachmentForVolumeName(ctx, vm)

					Expect(attachment).NotTo(BeNil())
					Expect(attachment.Spec.Volumes).To(HaveLen(1))

					attVol1 := attachment.Spec.Volumes[0]
					Expect(attVol1.PersistentVolumeClaim.ControllerKey).To(Equal("SCSI:1"))
					Expect(attVol1.PersistentVolumeClaim.UnitNumber).To(Equal("5"))
				})
			})
		})

		When("VM Spec.Volumes has CNS volume that references WFFC StorageClass", func() {
			const zoneName = "my-zone"

			var storageClass *storagev1.StorageClass
			var wffcPVC *corev1.PersistentVolumeClaim

			BeforeEach(func() {
				storageClass = builder.DummyStorageClass()
				storageClass.VolumeBindingMode = ptr.To(storagev1.VolumeBindingWaitForFirstConsumer)
				initObjects = append(initObjects, storageClass)

				wffcPVC = boundPVC1.DeepCopy()
				wffcPVC.Spec.StorageClassName = &storageClass.Name
				wffcPVC.Status.Phase = corev1.ClaimPending
				initObjects = append(initObjects, wffcPVC)

				vmVol = vmVolumeWithPVC1
				vm.Spec.Volumes = append(vm.Spec.Volumes, *vmVol)
				vm.Status.Zone = zoneName
				vm.Namespace = ns
			})

			JustBeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.VMWaitForFirstConsumerPVC = true
				})
			})

			AfterEach(func() {
				storageClass = nil
				wffcPVC = nil
			})

			Context("Feature is disabled", func() {
				JustBeforeEach(func() {
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMWaitForFirstConsumerPVC = false
					})
				})

				It("returns error", func() {
					fmt.Println(initObjects)
					err := reconciler.ReconcileNormal(volCtx)
					Expect(err).To(MatchError(ContainSubstring("PVC with WFFC storage class support is not enabled")))
				})
			})

			When("PVC does not exist", func() {
				BeforeEach(func() {
					vm.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = "bogus"
				})

				It("returns error", func() {
					err := reconciler.ReconcileNormal(volCtx)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("cannot get PVC"))
				})
			})

			When("PVC StorageClassName is unset", func() {
				BeforeEach(func() {
					wffcPVC.Spec.StorageClassName = nil
				})

				It("returns error", func() {
					err := reconciler.ReconcileNormal(volCtx)
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(ContainSubstring("does not have StorageClassName set")))
				})
			})

			When("PVC has AnnSelectedNode annotation", func() {
				BeforeEach(func() {
					wffcPVC.Annotations = map[string]string{
						storagehelpers.AnnSelectedNode: "node1",
					}
				})

				It("returns success", func() {
					err := reconciler.ReconcileNormal(volCtx)
					Expect(err).ToNot(HaveOccurred())
				})
			})

			When("StorageClass does not exist", func() {
				BeforeEach(func() {
					wffcPVC.Spec.StorageClassName = ptr.To("bogus")
				})

				It("returns error", func() {
					err := reconciler.ReconcileNormal(volCtx)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("cannot get StorageClass for PVC"))
				})
			})

			When("VM does not have Zone assigned", func() {
				BeforeEach(func() {
					vm.Status.Zone = ""
				})

				It("returns error", func() {
					err := reconciler.ReconcileNormal(volCtx)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("VM does not have Zone set"))
				})
			})

			It("returns success", func() {
				err := reconciler.ReconcileNormal(volCtx)
				Expect(err).ToNot(HaveOccurred())

				By("Adds node-is-zone and selected-node annotation to PVC", func() {
					pvc := &corev1.PersistentVolumeClaim{}
					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(wffcPVC), pvc)).To(Succeed())
					Expect(pvc.Annotations).To(HaveKeyWithValue(constants.CNSSelectedNodeIsZoneAnnotationKey, "true"))
					Expect(pvc.Annotations).To(HaveKeyWithValue(storagehelpers.AnnSelectedNode, zoneName))
				})
			})

			When("There an existing CnsNodeVmBatchAttachment has the PVC in spec", func() {
				var batchAtt *cnsv1alpha1.CnsNodeVmBatchAttachment
				BeforeEach(func() {
					batchAtt = &cnsv1alpha1.CnsNodeVmBatchAttachment{
						ObjectMeta: metav1.ObjectMeta{
							Name:      util.CNSBatchAttachmentNameForVolume(vm.Name),
							Namespace: vm.Namespace,
						},
						Spec: cnsv1alpha1.CnsNodeVmBatchAttachmentSpec{
							Volumes: []cnsv1alpha1.VolumeSpec{
								{
									Name: "cns-volume-1",
									PersistentVolumeClaim: cnsv1alpha1.PersistentVolumeClaimSpec{
										ClaimName: "pvc-volume-1",
									},
								},
							},
						},
					}
				})

				AfterEach(func() {
					batchAtt = nil
				})

				JustBeforeEach(func() {
					initObjects = append(initObjects, batchAtt)
				})

				It("returns success", func() {
					err := reconciler.ReconcileNormal(volCtx)
					Expect(err).ToNot(HaveOccurred())

					By("Adds node-is-zone and selected-node annotation to PVC", func() {
						pvc := &corev1.PersistentVolumeClaim{}
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(wffcPVC), pvc)).To(Succeed())
						Expect(pvc.Annotations).To(HaveKeyWithValue(constants.CNSSelectedNodeIsZoneAnnotationKey, "true"))
						Expect(pvc.Annotations).To(HaveKeyWithValue(storagehelpers.AnnSelectedNode, zoneName))
					})
				})
			})
		})

		When("There is an existing CnsNodeVmBatchAttachment", func() {
			var batchAtt *cnsv1alpha1.CnsNodeVmBatchAttachment
			BeforeEach(func() {
				batchAtt = &cnsv1alpha1.CnsNodeVmBatchAttachment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      util.CNSBatchAttachmentNameForVolume(vm.Name),
						Namespace: vm.Namespace,
					},
				}
			})

			AfterEach(func() {
				batchAtt = nil
			})

			JustBeforeEach(func() {
				initObjects = append(initObjects, batchAtt)
			})

			When("there is no pvcs on the VM", func() {
				It("Should delete the batchAttachment", func() {
					err := reconciler.ReconcileNormal(volCtx)
					Expect(err).ToNot(HaveOccurred())
					attachment := getCNSBatchAttachmentForVolumeName(ctx, vm)

					Expect(attachment).To(BeNil())
					Expect(vm.Status.Volumes).To(BeEmpty())
				})
			})
		})
	})

	Context("ReconcileDelete", func() {
		When("VM is marked for deletion", func() {
			BeforeEach(func() {
				now := metav1.Now()
				vm.DeletionTimestamp = &now
			})

			It("returns success", func() {
				err := reconciler.ReconcileDelete(volCtx)
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})

}

func getCNSBatchAttachmentForVolumeName(ctx *builder.UnitTestContextForController, vm *vmopv1.VirtualMachine) *cnsv1alpha1.CnsNodeVmBatchAttachment {

	GinkgoHelper()

	objectKey := client.ObjectKey{Name: util.CNSBatchAttachmentNameForVolume(vm.Name), Namespace: vm.Namespace}
	attachment := &cnsv1alpha1.CnsNodeVmBatchAttachment{}

	err := ctx.Client.Get(ctx, objectKey, attachment)
	if err == nil {
		return attachment
	}

	if apierrors.IsNotFound(err) {
		return nil
	}

	Expect(err).ToNot(HaveOccurred())
	return nil
}
