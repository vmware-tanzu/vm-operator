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
	"k8s.io/apimachinery/pkg/api/resource"
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
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
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
		ns                = "dummy-ns"
		dummyBiosUUID     = "dummy-bios-uuid"
		dummyInstanceUUID = "dummy-instance-uuid"
		dummyDiskUUID     = "111-222-333-disk-uuid"
		claimName1        = "pvc-volume-1"
		claimName2        = "pvc-volume-2"
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
		vmVolumeWithPVC2 *vmopv1.VirtualMachineVolume
		boundPVC2        *corev1.PersistentVolumeClaim
	)

	BeforeEach(func() {

		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-vm",
				Namespace: ns,
			},
			Status: vmopv1.VirtualMachineStatus{
				BiosUUID:     dummyBiosUUID,
				InstanceUUID: dummyInstanceUUID,
				Hardware: &vmopv1.VirtualMachineHardwareStatus{
					Controllers: []vmopv1.VirtualControllerStatus{
						{
							Type:      "SCSI",
							BusNumber: 0,
							DeviceKey: 1000,
						},
					},
				},
			},
		}

		vmVolumeWithPVC1 = &vmopv1.VirtualMachineVolume{
			Name: "cns-volume-1",
			VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
				PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
					PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: claimName1,
					},
					ControllerType:      "SCSI",
					ControllerBusNumber: ptr.To(int32(0)),
					UnitNumber:          ptr.To(int32(0)),
					DiskMode:            vmopv1.VolumeDiskModePersistent,
					SharingMode:         vmopv1.VolumeSharingModeNone,
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

		vmVolumeWithPVC2 = &vmopv1.VirtualMachineVolume{
			Name: "cns-volume-2",
			VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
				PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
					PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: claimName2,
					},
					ControllerType:      "SCSI",
					ControllerBusNumber: ptr.To(int32(0)),
					UnitNumber:          ptr.To(int32(1)),
					DiskMode:            vmopv1.VolumeDiskModePersistent,
					SharingMode:         vmopv1.VolumeSharingModeNone,
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

	})

	JustBeforeEach(func() {

		ctx = suite.NewUnitTestContextForController()
		ctx.Context = cource.WithContext(ctx.Context)

		// Replace the fake client with our own that has the expected index.
		ctx.Client = fake.NewClientBuilder().
			WithScheme(ctx.Client.Scheme()).
			WithObjects(initObjects...).
			WithInterceptorFuncs(withFuncs).
			WithStatusSubresource(builder.KnownObjectTypes()...).
			WithIndex(
				&cnsv1alpha1.CnsNodeVmAttachment{},
				"spec.nodeuuid",
				func(rawObj client.Object) []string {
					attachment := rawObj.(*cnsv1alpha1.CnsNodeVmAttachment)
					return []string{attachment.Spec.NodeUUID}
				}).
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

	getCNSBatchAttachmentForVolumeName := func(ctx *builder.UnitTestContextForController, vm *vmopv1.VirtualMachine) *cnsv1alpha1.CnsNodeVMBatchAttachment {

		GinkgoHelper()

		objectKey := client.ObjectKey{Name: util.CNSBatchAttachmentNameForVM(vm.Name), Namespace: vm.Namespace}
		attachment := &cnsv1alpha1.CnsNodeVMBatchAttachment{}

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

	Context("ReconcileNormal", func() {
		When("VM does not have InstanceUUID", func() {
			BeforeEach(func() {
				vmVol = vmVolumeWithPVC1
				vm.Spec.Volumes = append(vm.Spec.Volumes, *vmVol)
				vm.Status.InstanceUUID = ""
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

		When("VM does not have BiosUUID", func() {
			BeforeEach(func() {
				vmVol = vmVolumeWithPVC1
				vm.Spec.Volumes = append(vm.Spec.Volumes, *vmVol)
				vm.Status.BiosUUID = ""
			})

			It("returns success", func() {
				err := reconciler.ReconcileNormal(volCtx)
				Expect(err).ToNot(HaveOccurred())

				By("Did not create CnsNodeVMBatchAttachment", func() {
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
					Expect(attVol1.PersistentVolumeClaim.ClaimName).To(Equal(claimName1))
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
								ControllerType:      vmopv1.VirtualControllerTypeSCSI,
								ControllerBusNumber: ptr.To(int32(0)),
								UnitNumber:          ptr.To(int32(1)),
								DiskMode:            vmopv1.VolumeDiskModePersistent,
								SharingMode:         vmopv1.VolumeSharingModeNone,
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
					vm.Spec.Volumes[0].PersistentVolumeClaim.ControllerBusNumber = ptr.To[int32](0)
					vm.Spec.Volumes[0].PersistentVolumeClaim.UnitNumber = ptr.To[int32](5)
				})

				It("sets volme variables correctly", func() {
					err := reconciler.ReconcileNormal(volCtx)
					Expect(err).NotTo(HaveOccurred())

					attachment := getCNSBatchAttachmentForVolumeName(ctx, vm)

					Expect(attachment).NotTo(BeNil())
					Expect(attachment.Spec.Volumes).To(HaveLen(1))

					attVol1 := attachment.Spec.Volumes[0]
					Expect(attVol1.PersistentVolumeClaim.ControllerKey).
						To(Equal(ptr.To(int32(1000))))
					Expect(attVol1.PersistentVolumeClaim.UnitNumber).
						To(Equal(ptr.To(int32(5))))
				})
			})
		})

		When("VM has legacy CnsNodeVmAttachment resources", func() {
			var (
				legacyAttachment1 *cnsv1alpha1.CnsNodeVmAttachment
				legacyAttachment2 *cnsv1alpha1.CnsNodeVmAttachment
			)

			BeforeEach(func() {
				// Create legacy attachments for testing
				legacyAttachment1 = &cnsv1alpha1.CnsNodeVmAttachment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      util.CNSAttachmentNameForVolume(vm.Name, "legacy-volume-1"),
						Namespace: ns,
					},
					Spec: cnsv1alpha1.CnsNodeVmAttachmentSpec{
						NodeUUID:   dummyBiosUUID,
						VolumeName: "legacy-pvc-1",
					},
					Status: cnsv1alpha1.CnsNodeVmAttachmentStatus{
						Attached: true,
					},
				}

				legacyAttachment2 = &cnsv1alpha1.CnsNodeVmAttachment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      util.CNSAttachmentNameForVolume(vm.Name, "legacy-volume-2"),
						Namespace: ns,
					},
					Spec: cnsv1alpha1.CnsNodeVmAttachmentSpec{
						NodeUUID:   dummyBiosUUID,
						VolumeName: "legacy-pvc-2",
					},
					Status: cnsv1alpha1.CnsNodeVmAttachmentStatus{
						Attached: false,
					},
				}
			})

			When("legacy attachments exist but volumes are not in VM spec", func() {
				BeforeEach(func() {
					// Add legacy attachments but no corresponding volumes in VM spec
					initObjects = append(initObjects, legacyAttachment1, legacyAttachment2)
				})

				It("should delete orphaned legacy attachments", func() {
					err := reconciler.ReconcileNormal(volCtx)
					Expect(err).ToNot(HaveOccurred())

					// Verify legacy attachments were deleted
					legacyKey1 := client.ObjectKey{Name: legacyAttachment1.Name, Namespace: ns}
					legacyKey2 := client.ObjectKey{Name: legacyAttachment2.Name, Namespace: ns}

					attachment1 := &cnsv1alpha1.CnsNodeVmAttachment{}
					attachment2 := &cnsv1alpha1.CnsNodeVmAttachment{}

					err1 := ctx.Client.Get(ctx, legacyKey1, attachment1)
					err2 := ctx.Client.Get(ctx, legacyKey2, attachment2)

					Expect(apierrors.IsNotFound(err1)).To(BeTrue(), "Legacy attachment 1 should be deleted")
					Expect(apierrors.IsNotFound(err2)).To(BeTrue(), "Legacy attachment 2 should be deleted")

					// No batch attachment should be created since no volumes in spec
					batchAttachment := getCNSBatchAttachmentForVolumeName(ctx, vm)
					Expect(batchAttachment).To(BeNil())
				})
			})

			When("legacy attachments exist and corresponding volumes are in VM spec", func() {
				BeforeEach(func() {
					// Add a volume that matches legacy attachment
					vmVolWithLegacy := &vmopv1.VirtualMachineVolume{
						Name: "legacy-volume-1",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "legacy-pvc-1",
								},
							},
						},
					}

					legacyPVC := &corev1.PersistentVolumeClaim{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "legacy-pvc-1",
							Namespace: ns,
						},
						Status: corev1.PersistentVolumeClaimStatus{
							Phase: corev1.ClaimBound,
						},
					}

					vm.Spec.Volumes = append(vm.Spec.Volumes, *vmVolWithLegacy)
					initObjects = append(initObjects, legacyAttachment1, legacyAttachment2, legacyPVC)
				})

				It("should keep matching legacy attachments and delete orphaned ones", func() {
					err := reconciler.ReconcileNormal(volCtx)
					Expect(err).ToNot(HaveOccurred())

					// Verify matching legacy attachment still exists
					legacyKey1 := client.ObjectKey{Name: legacyAttachment1.Name, Namespace: ns}
					attachment1 := &cnsv1alpha1.CnsNodeVmAttachment{}
					err1 := ctx.Client.Get(ctx, legacyKey1, attachment1)
					Expect(err1).ToNot(HaveOccurred(), "Matching legacy attachment should be preserved")

					// Verify orphaned legacy attachment was deleted
					legacyKey2 := client.ObjectKey{Name: legacyAttachment2.Name, Namespace: ns}
					attachment2 := &cnsv1alpha1.CnsNodeVmAttachment{}
					err2 := ctx.Client.Get(ctx, legacyKey2, attachment2)
					Expect(apierrors.IsNotFound(err2)).To(BeTrue(), "Orphaned legacy attachment should be deleted")

					// batch attachment should NOT be created since
					// the volume is handled by a matching legacy
					// attachment
					batchAttachment := getCNSBatchAttachmentForVolumeName(ctx, vm)
					Expect(batchAttachment).To(BeNil(), "Batch attachment should not be created when volume is tracked by matching legacy attachment")
				})
			})

			When("legacy attachment has different PVC name than VM spec", func() {
				BeforeEach(func() {
					// Add a volume with same name but different PVC
					vmVolWithDifferentPVC := &vmopv1.VirtualMachineVolume{
						Name: "legacy-volume-1",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "different-pvc", // Different from legacy-pvc-1
								},
								ControllerType:      vmopv1.VirtualControllerTypeSCSI,
								ControllerBusNumber: ptr.To(int32(0)),
								UnitNumber:          ptr.To(int32(0)),
								DiskMode:            vmopv1.VolumeDiskModePersistent,
								SharingMode:         vmopv1.VolumeSharingModeNone,
							},
						},
					}

					differentPVC := &corev1.PersistentVolumeClaim{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "different-pvc",
							Namespace: ns,
						},
						Status: corev1.PersistentVolumeClaimStatus{
							Phase: corev1.ClaimBound,
						},
					}

					vm.Spec.Volumes = append(vm.Spec.Volumes, *vmVolWithDifferentPVC)
					initObjects = append(initObjects, legacyAttachment1, legacyAttachment2, differentPVC)
				})

				It("should delete legacy attachment and create batch attachment", func() {
					err := reconciler.ReconcileNormal(volCtx)
					Expect(err).ToNot(HaveOccurred())

					// Verify legacy attachments were deleted (PVC mismatch makes them orphaned)
					legacyKey1 := client.ObjectKey{Name: legacyAttachment1.Name, Namespace: ns}
					legacyKey2 := client.ObjectKey{Name: legacyAttachment2.Name, Namespace: ns}

					attachment1 := &cnsv1alpha1.CnsNodeVmAttachment{}
					attachment2 := &cnsv1alpha1.CnsNodeVmAttachment{}

					err1 := ctx.Client.Get(ctx, legacyKey1, attachment1)
					err2 := ctx.Client.Get(ctx, legacyKey2, attachment2)

					Expect(apierrors.IsNotFound(err1)).To(BeTrue(), "Legacy attachment with wrong PVC should be deleted")
					Expect(apierrors.IsNotFound(err2)).To(BeTrue(), "Orphaned legacy attachment should be deleted")

					// Batch attachment should be created for the volume
					batchAttachment := getCNSBatchAttachmentForVolumeName(ctx, vm)
					Expect(batchAttachment).ToNot(BeNil())
					Expect(batchAttachment.Spec.Volumes).To(HaveLen(1))
					Expect(batchAttachment.Spec.Volumes[0].Name).To(Equal("legacy-volume-1"))
					Expect(batchAttachment.Spec.Volumes[0].PersistentVolumeClaim.ClaimName).To(Equal("different-pvc"))
				})
			})

			When("legacy attachment has different NodeUUID (stale)", func() {
				BeforeEach(func() {
					// Make legacy attachment have stale NodeUUID
					legacyAttachment1.Spec.NodeUUID = "stale-bios-uuid"

					vmVolWithLegacy := &vmopv1.VirtualMachineVolume{
						Name: "legacy-volume-1",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "legacy-pvc-1",
								},
								ControllerType:      vmopv1.VirtualControllerTypeSCSI,
								ControllerBusNumber: ptr.To(int32(0)),
								UnitNumber:          ptr.To(int32(0)),
								DiskMode:            vmopv1.VolumeDiskModePersistent,
								SharingMode:         vmopv1.VolumeSharingModeNone,
							},
						},
					}

					legacyPVC := &corev1.PersistentVolumeClaim{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "legacy-pvc-1",
							Namespace: ns,
						},
						Status: corev1.PersistentVolumeClaimStatus{
							Phase: corev1.ClaimBound,
						},
					}

					vm.Spec.Volumes = append(vm.Spec.Volumes, *vmVolWithLegacy)
					initObjects = append(initObjects, legacyAttachment1, legacyPVC)
				})

				It("should treat volume as greenfield and create batch attachment", func() {
					err := reconciler.ReconcileNormal(volCtx)
					Expect(err).ToNot(HaveOccurred())

					// Current behavior: stale legacy attachment is NOT automatically deleted
					// (only orphaned attachments not matching VM spec are deleted)
					legacyKey1 := client.ObjectKey{Name: legacyAttachment1.Name, Namespace: ns}
					attachment1 := &cnsv1alpha1.CnsNodeVmAttachment{}
					err1 := ctx.Client.Get(ctx, legacyKey1, attachment1)
					Expect(err1).ToNot(HaveOccurred(), "Stale legacy attachment still exists in current implementation")

					// Volume should be processed by batch controller regardless
					batchAttachment := getCNSBatchAttachmentForVolumeName(ctx, vm)
					Expect(batchAttachment).ToNot(BeNil())
					Expect(batchAttachment.Spec.Volumes).To(HaveLen(1))
					Expect(batchAttachment.Spec.Volumes[0].Name).To(Equal("legacy-volume-1"))
				})
			})

			When("error occurs while getting legacy attachments", func() {
				BeforeEach(func() {
					// Add a volume to trigger processing
					vmVol = vmVolumeWithPVC1
					vm.Spec.Volumes = append(vm.Spec.Volumes, *vmVol)
					initObjects = append(initObjects, boundPVC1)

					// Set up interceptor to simulate error when listing CnsNodeVmAttachment
					withFuncs.List = func(ctx context.Context, client client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
						if _, ok := list.(*cnsv1alpha1.CnsNodeVmAttachmentList); ok {
							return errors.New("simulated list error")
						}
						return client.List(ctx, list, opts...)
					}
				})

				It("should return error and not proceed with batch processing", func() {
					err := reconciler.ReconcileNormal(volCtx)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("simulated list error"))

					// No batch attachment should be created due to error
					batchAttachment := getCNSBatchAttachmentForVolumeName(ctx, vm)
					Expect(batchAttachment).To(BeNil())
				})
			})

			When("error occurs while deleting legacy attachments", func() {
				BeforeEach(func() {
					// Add orphaned legacy attachment
					initObjects = append(initObjects, legacyAttachment1)

					// Set up interceptor to simulate error when deleting CnsNodeVmAttachment
					withFuncs.Delete = func(ctx context.Context, client client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
						if _, ok := obj.(*cnsv1alpha1.CnsNodeVmAttachment); ok {
							return errors.New("simulated delete error")
						}
						return client.Delete(ctx, obj, opts...)
					}
				})

				It("should log error but continue with batch processing", func() {
					err := reconciler.ReconcileNormal(volCtx)
					Expect(err).To(Equal(errors.New("simulated delete error"))) // Should not fail, just log error

					// Legacy attachment should still exist due to delete error
					legacyKey := client.ObjectKey{Name: legacyAttachment1.Name, Namespace: ns}
					attachment := &cnsv1alpha1.CnsNodeVmAttachment{}
					err = ctx.Client.Get(ctx, legacyKey, attachment)
					Expect(err).ToNot(HaveOccurred(), "Legacy attachment should still exist due to delete error")

					// Batch processing should continue normally (no volumes to process in this case)
					batchAttachment := getCNSBatchAttachmentForVolumeName(ctx, vm)
					Expect(batchAttachment).To(BeNil())
				})
			})
		})

		When("VM Spec.Volumes has CNS volume with a SOAP error", func() {
			awfulErrMsg := `failed to attach cns volume: \"88854b48-2b1c-43f8-8889-de4b5ca2cab5\" to node vm: \"VirtualMachine:vm-42
[VirtualCenterHost: vc.vmware.com, UUID: 42080725-d6b0-c045-b24e-29c4dadca6f2, Datacenter: Datacenter
[Datacenter: Datacenter:datacenter, VirtualCenterHost: vc.vmware.com]]\".
fault: \"(*vimtypes.LocalizedMethodFault)(0xc003d9b9a0)({\\n DynamicData: (vimtypes.DynamicData)
{\\n },\\n Fault: (*vimtypes.ResourceInUse)(0xc002e69080)({\\n VimFault: (vimtypes.VimFault)
{\\n MethodFault: (vimtypes.MethodFault) {\\n FaultCause: (*vimtypes.LocalizedMethodFault)(\u003cnil\u003e),\\n
FaultMessage: ([]vimtypes.LocalizableMessage) \u003cnil\u003e\\n }\\n },\\n Type: (string) \\\"\\\",\\n Name:
(string) (len=6) \\\"volume\\\"\\n }),\\n LocalizedMessage: (string) (len=32)
\\\"The resource 'volume' is in use.\\\"\\n})\\n\". opId: \"67d69c68\""
`

			BeforeEach(func() {
				vmVol = vmVolumeWithPVC1
				vm.Spec.Volumes = append(vm.Spec.Volumes, *vmVol)
				initObjects = append(initObjects, boundPVC1)

				attachment := cnsBatchAttachmentForVMVolume(vm, []vmopv1.VirtualMachineVolume{*vmVol})
				attachment.Status.VolumeStatus = append(attachment.Status.VolumeStatus, cnsv1alpha1.VolumeStatus{
					Name: vmVol.Name,
					PersistentVolumeClaim: cnsv1alpha1.PersistentVolumeClaimStatus{
						Attached:  true,
						DiskUUID:  dummyDiskUUID,
						ClaimName: claimName1,
						Error:     awfulErrMsg,
					},
				})
				initObjects = append(initObjects, attachment)
			})

			It("returns success", func() {
				err := reconciler.ReconcileNormal(volCtx)
				Expect(err).ToNot(HaveOccurred())

				By("Expected VM Status.Volumes with sanitized error", func() {
					Expect(vm.Status.Volumes).To(HaveLen(1))

					attachment := getCNSBatchAttachmentForVolumeName(ctx, vm)
					Expect(attachment).ToNot(BeNil())
					assertBatchAttachmentSpec(vm, attachment)

					Expect(vm.Status.Volumes).To(HaveLen(1))
					Expect(attachment.Status.VolumeStatus).To(HaveLen(1))
					attachment.Status.VolumeStatus[0].PersistentVolumeClaim.Error = "failed to attach cns volume"
					assertVMVolStatusFromBatchAttachmentStatus(vm, attachment, 0, 0)
				})
			})
		})

		When("VM Status.Volumes is sorted as expected", func() {
			var vmVol1 vmopv1.VirtualMachineVolume
			var vmVol2 vmopv1.VirtualMachineVolume

			BeforeEach(func() {
				vmVol1 = *vmVolumeWithPVC1
				vmVol2 = *vmVolumeWithPVC2
				vm.Spec.Volumes = append(vm.Spec.Volumes, vmVol1, vmVol2)

				initObjects = append(initObjects, boundPVC1, boundPVC2)
			})

			// We sort by DiskUUID, but the volumes in CnsNodeVMBatchAttachment
			// haven't been "attached" yet, so expect the Spec.Volumes order.
			When("CnsNodeVMBatchAttachment do not have DiskUUID set", func() {
				It("returns success", func() {
					err := reconciler.ReconcileNormal(volCtx)
					Expect(err).ToNot(HaveOccurred())

					attachment := getCNSBatchAttachmentForVolumeName(ctx, vm)
					Expect(attachment).ToNot(BeNil())

					By("VM Status.Volumes are stable-sorted by Spec.Volumes order", func() {
						Expect(vm.Status.Volumes).To(HaveLen(2))
						Expect(attachment.Spec.Volumes).To(HaveLen(2))
						assertVMVolStatusFromBatchAttachmentSpec(vm, attachment, 0, 0)
						assertVMVolStatusFromBatchAttachmentSpec(vm, attachment, 1, 1)
					})
				})
			})

			When("CnsNodeVMBatchAttachment have DiskUUID set", func() {
				dummyDiskUUID1 := "z"
				dummyDiskUUID2 := "a"

				BeforeEach(func() {
					attachment := cnsBatchAttachmentForVMVolume(vm, []vmopv1.VirtualMachineVolume{vmVol1, vmVol2})
					attachment.Status.VolumeStatus = append(attachment.Status.VolumeStatus,
						cnsv1alpha1.VolumeStatus{
							Name: vmVol1.Name,
							PersistentVolumeClaim: cnsv1alpha1.PersistentVolumeClaimStatus{
								ClaimName: claimName1,
								Attached:  true,
								DiskUUID:  dummyDiskUUID1,
							},
						},
						cnsv1alpha1.VolumeStatus{
							Name: vmVol2.Name,
							PersistentVolumeClaim: cnsv1alpha1.PersistentVolumeClaimStatus{
								ClaimName: claimName2,
								Attached:  true,
								DiskUUID:  dummyDiskUUID2,
							},
						},
					)

					initObjects = append(initObjects, attachment)
				})

				It("returns success", func() {

					err := reconciler.ReconcileNormal(volCtx)
					Expect(err).ToNot(HaveOccurred())

					attachment := getCNSBatchAttachmentForVolumeName(ctx, vm)
					Expect(attachment).ToNot(BeNil())

					By("VM Status.Volumes are stable-sorted by Spec.Volumes order", func() {
						Expect(vm.Status.Volumes).To(HaveLen(2))
						Expect(attachment.Status.VolumeStatus).To(HaveLen(2))
						assertVMVolStatusFromBatchAttachmentStatus(vm, attachment, 1, 0)
						assertVMVolStatusFromBatchAttachmentStatus(vm, attachment, 0, 1)
					})
				})

				When("Collecting limit and usage information", func() {

					classicDisk1 := func() vmopv1.VirtualMachineVolumeStatus {
						return vmopv1.VirtualMachineVolumeStatus{
							Name:     "my-disk-0",
							Type:     vmopv1.VolumeTypeClassic,
							Limit:    ptr.To(resource.MustParse("10Gi")),
							Used:     ptr.To(resource.MustParse("91Gi")),
							Attached: true,
							DiskUUID: "100",
						}
					}

					classicDisk2 := func() vmopv1.VirtualMachineVolumeStatus {
						return vmopv1.VirtualMachineVolumeStatus{
							Name:     "my-disk-1",
							Type:     vmopv1.VolumeTypeClassic,
							Limit:    ptr.To(resource.MustParse("15Gi")),
							Used:     ptr.To(resource.MustParse("5Gi")),
							Attached: true,
							DiskUUID: "101",
						}
					}

					BeforeEach(func() {
						vm.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{
							classicDisk1(),
							classicDisk2(),
						}
					})
					AfterEach(func() {
						vm.Status.Volumes = nil
					})

					assertBaselineVolStatus := func() {

						GinkgoHelper()

						err := reconciler.ReconcileNormal(volCtx)
						Expect(err).ToNot(HaveOccurred())

						attachment := getCNSBatchAttachmentForVolumeName(ctx, vm)
						Expect(attachment).ToNot(BeNil())

						By("VM Status.Volumes are sorted by DiskUUID", func() {
							Expect(vm.Status.Volumes).To(HaveLen(4))
							Expect(attachment.Status.VolumeStatus).To(HaveLen(2))

							Expect(vm.Status.Volumes[0]).To(Equal(classicDisk1()))
							Expect(vm.Status.Volumes[1]).To(Equal(classicDisk2()))

							assertVMVolStatusFromBatchAttachmentStatus(vm, attachment, 2, 1)
							assertVMVolStatusFromBatchAttachmentStatus(vm, attachment, 3, 0)
						})
					}

					It("does not remove any existing classic disks", func() {
						assertBaselineVolStatus()
					})

					When("Existing status has usage info for a PVC", func() {
						BeforeEach(func() {
							vm.Status.Volumes = append(vm.Status.Volumes,
								vmopv1.VirtualMachineVolumeStatus{
									Name: vmVol1.Name,
									Type: vmopv1.VolumeTypeManaged,
									Used: ptr.To(resource.MustParse("1Gi")),
								},
							)
						})

						assertPVCHasUsage := func() {

							GinkgoHelper()

							Expect(vm.Status.Volumes[3].Used).To(Equal(ptr.To(resource.MustParse("1Gi"))))
						}

						It("includes the PVC usage in the result", func() {
							assertBaselineVolStatus()
							assertPVCHasUsage()
						})

						When("Existing status has stale PVC", func() {
							BeforeEach(func() {
								vm.Status.Volumes = append(vm.Status.Volumes,
									vmopv1.VirtualMachineVolumeStatus{
										Name: "non-existing-pvc",
										Type: vmopv1.VolumeTypeManaged,
										Used: ptr.To(resource.MustParse("1Gi")),
									},
								)
							})

							It("should be removed from the result", func() {
								assertBaselineVolStatus()
								assertPVCHasUsage()
							})
						})

						When("PVC resource exists with limit or request", func() {

							assertPVCHasLimit := func() {

								GinkgoHelper()

								Expect(vm.Status.Volumes[3].Limit).To(Equal(ptr.To(resource.MustParse("20Gi"))))
							}

							When("PVC has limit", func() {
								It("should report its limit", func() {
									boundPVC1.Spec.Resources.Limits = corev1.ResourceList{
										corev1.ResourceStorage: resource.MustParse("20Gi"),
									}
									Expect(ctx.Client.Update(ctx, boundPVC1)).To(Succeed())

									assertBaselineVolStatus()
									assertPVCHasUsage()
									assertPVCHasLimit()
								})
							})

							When("PVC has request", func() {
								It("should report its request", func() {
									boundPVC1.Spec.Resources.Requests = corev1.ResourceList{
										corev1.ResourceStorage: resource.MustParse("20Gi"),
									}
									Expect(ctx.Client.Update(ctx, boundPVC1)).To(Succeed())

									assertBaselineVolStatus()
									assertPVCHasUsage()
									assertPVCHasLimit()
								})
							})

							When("PVC has limit and request", func() {
								It("should report its limit", func() {
									boundPVC1.Spec.Resources.Limits = corev1.ResourceList{
										corev1.ResourceStorage: resource.MustParse("20Gi"),
									}
									boundPVC1.Spec.Resources.Requests = corev1.ResourceList{
										corev1.ResourceStorage: resource.MustParse("10Gi"),
									}
									Expect(ctx.Client.Update(ctx, boundPVC1)).To(Succeed())

									assertBaselineVolStatus()
									assertPVCHasUsage()
									assertPVCHasLimit()
								})
							})
						})
					})

					When("Existing status has crypto info for a PVC", func() {

						newCryptoStatus := func() *vmopv1.VirtualMachineVolumeCryptoStatus {
							return &vmopv1.VirtualMachineVolumeCryptoStatus{
								ProviderID: "my-provider-id",
								KeyID:      "my-key-id",
							}
						}

						assertPVCHasCrypto := func() {

							GinkgoHelper()

							Expect(vm.Status.Volumes[3].Crypto).To(Equal(newCryptoStatus()))
						}

						BeforeEach(func() {
							vm.Status.Volumes = append(vm.Status.Volumes,
								vmopv1.VirtualMachineVolumeStatus{
									Name:   vmVol1.Name,
									Type:   vmopv1.VolumeTypeManaged,
									Crypto: newCryptoStatus(),
								},
							)
						})

						It("includes the PVC crypto in the result", func() {
							assertBaselineVolStatus()
							assertPVCHasCrypto()
						})
					})
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

			When("There an existing CnsNodeVMBatchAttachment has the PVC in spec", func() {
				var batchAtt *cnsv1alpha1.CnsNodeVMBatchAttachment
				BeforeEach(func() {
					batchAtt = &cnsv1alpha1.CnsNodeVMBatchAttachment{
						ObjectMeta: metav1.ObjectMeta{
							Name:      util.CNSBatchAttachmentNameForVM(vm.Name),
							Namespace: vm.Namespace,
						},
						Spec: cnsv1alpha1.CnsNodeVMBatchAttachmentSpec{
							Volumes: []cnsv1alpha1.VolumeSpec{
								{
									Name: "cns-volume-1",
									PersistentVolumeClaim: cnsv1alpha1.PersistentVolumeClaimSpec{
										ClaimName: claimName1,
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

		When("There is an existing CnsNodeVMBatchAttachment", func() {
			var attachment *cnsv1alpha1.CnsNodeVMBatchAttachment
			BeforeEach(func() {
				attachment = cnsBatchAttachmentForVMVolume(vm, []vmopv1.VirtualMachineVolume{})
			})

			AfterEach(func() {
				attachment = nil
			})

			JustBeforeEach(func() {
				initObjects = append(initObjects, attachment)
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

			When("VM Spec.Volumes has changed PVC", func() {
				BeforeEach(func() {
					vmVol1 := *vmVolumeWithPVC1
					vmVolWithDifferentPVC := *vmVolumeWithPVC1
					vmVolWithDifferentPVC.PersistentVolumeClaim.ClaimName = boundPVC2.Name
					// VM Spec has Volume with new PVC.
					vm.Spec.Volumes = append(vm.Spec.Volumes, vmVolWithDifferentPVC)
					initObjects = append(initObjects, boundPVC2)

					// Old attachment still points to vmVol1 with old PVC.
					attachment = cnsBatchAttachmentForVMVolume(vm, []vmopv1.VirtualMachineVolume{vmVol1})
					attachment.Status.VolumeStatus = append(attachment.Status.VolumeStatus,
						cnsv1alpha1.VolumeStatus{
							Name: vmVol1.Name,
							PersistentVolumeClaim: cnsv1alpha1.PersistentVolumeClaimStatus{
								ClaimName: claimName1,
								Attached:  true,
							},
						},
					)

					vm.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{
						{
							Name:     "cns-volume1-1",
							Attached: true,
						},
					}

					initObjects = append(initObjects, attachment)
				})

				It("returns success and refresh the vm volume status", func() {
					err := reconciler.ReconcileNormal(volCtx)
					Expect(err).ToNot(HaveOccurred())

					attachment := getCNSBatchAttachmentForVolumeName(ctx, vm)
					Expect(attachment).ToNot(BeNil())

					By("VM Status.Volumes are stable-sorted by Spec.Volumes order", func() {
						Expect(vm.Status.Volumes).To(HaveLen(1))
						Expect(attachment.Status.VolumeStatus).To(HaveLen(1))
						assertVMVolStatusFromBatchAttachmentSpec(vm, attachment, 0, 0)
					})
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

func assertBatchAttachmentSpec(
	vm *vmopv1.VirtualMachine,
	attachment *cnsv1alpha1.CnsNodeVMBatchAttachment) {

	GinkgoHelper()

	Expect(attachment.Spec.NodeUUID).To(Equal(vm.Status.InstanceUUID))

	ownerRefs := attachment.GetOwnerReferences()
	Expect(ownerRefs).To(HaveLen(1))
	ownerRef := ownerRefs[0]
	Expect(ownerRef.Name).To(Equal(vm.Name))
	Expect(ownerRef.Controller).ToNot(BeNil())
	Expect(*ownerRef.Controller).To(BeTrue())
}

func cnsBatchAttachmentForVMVolume(
	vm *vmopv1.VirtualMachine,
	vmVols []vmopv1.VirtualMachineVolume) *cnsv1alpha1.CnsNodeVMBatchAttachment {
	t := true
	batchAttachment := &cnsv1alpha1.CnsNodeVMBatchAttachment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      util.CNSBatchAttachmentNameForVM(vm.Name),
			Namespace: vm.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         "vmoperator.vmware.com/v1alpha1",
					Kind:               "VirtualMachine",
					Name:               vm.Name,
					UID:                vm.UID,
					Controller:         &t,
					BlockOwnerDeletion: &t,
				},
			},
		},
		Spec: cnsv1alpha1.CnsNodeVMBatchAttachmentSpec{
			NodeUUID: vm.Status.InstanceUUID,
			Volumes:  []cnsv1alpha1.VolumeSpec{},
		},
		Status: cnsv1alpha1.CnsNodeVMBatchAttachmentStatus{
			VolumeStatus: []cnsv1alpha1.VolumeStatus{},
		},
	}

	for _, vmVol := range vmVols {
		batchAttachment.Spec.Volumes = append(batchAttachment.Spec.Volumes,
			cnsv1alpha1.VolumeSpec{
				Name: vmVol.PersistentVolumeClaim.ClaimName,
			})
	}

	return batchAttachment
}

func assertVMVolStatusFromBatchAttachmentStatus(
	vm *vmopv1.VirtualMachine,
	attachment *cnsv1alpha1.CnsNodeVMBatchAttachment,
	vmVolStatusIndex,
	attachmentStatusIndex int) {

	GinkgoHelper()

	vmVolStatus := vm.Status.Volumes[vmVolStatusIndex]
	attachmentVolStatus := attachment.Status.VolumeStatus[attachmentStatusIndex]

	Expect(vmVolStatus.Type).To(Equal(vmopv1.VolumeTypeManaged), "type should match")
	Expect(vmVolStatus.Name).To(Equal(attachmentVolStatus.Name), "volume name should match")
	Expect(vmVolStatus.Attached).To(Equal(attachmentVolStatus.PersistentVolumeClaim.Attached), "attached should match")
	Expect(vmVolStatus.DiskUUID).To(Equal(attachmentVolStatus.PersistentVolumeClaim.DiskUUID), "diskuuid should match")
	Expect(vmVolStatus.Error).To(Equal(attachmentVolStatus.PersistentVolumeClaim.Error), "error shouuld match")
}

func assertVMVolStatusFromBatchAttachmentSpec(
	vm *vmopv1.VirtualMachine,
	attachment *cnsv1alpha1.CnsNodeVMBatchAttachment,
	vmVolStatusIndex,
	attachmentStatusIndex int) {

	GinkgoHelper()

	vmVolStatus := vm.Status.Volumes[vmVolStatusIndex]
	attachmentVolSpec := attachment.Spec.Volumes[attachmentStatusIndex]

	Expect(vmVolStatus.Type).To(Equal(vmopv1.VolumeTypeManaged), "type should match")
	Expect(vmVolStatus.Name).To(Equal(attachmentVolSpec.Name), "volume name should match")
	Expect(vmVolStatus.Attached).To(BeFalse(), "attached should be set to default")
}
