// Copyright (c) 2020-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha2_test

import (
	goctx "context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	k8sapierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8serrors "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/pkg/syncer/cnsoperator/apis/cnsnodevmattachment/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	volume "github.com/vmware-tanzu/vm-operator/controllers/volume/v1alpha2"
	volContext "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/fake"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func unitTests() {
	Describe("Invoking Reconcile", unitTestsReconcile)
}

type testFailClient struct {
	client.Client
}

// This is used for returning an error while unit testing.
func (f *testFailClient) Create(ctx goctx.Context, obj client.Object, opts ...client.CreateOption) error {
	return k8sapierrors.NewForbidden(schema.GroupResource{}, "", errors.New("insufficient quota for creating PVC"))
}

func unitTestsReconcile() {
	const (
		dummyBiosUUID                 = "dummy-bios-uuid"
		dummyDiskUUID                 = "111-222-333-disk-uuid"
		dummyInstanceStorageClassName = "dummy-instance-storage-class"
	)

	var (
		initObjects []client.Object
		ctx         *builder.UnitTestContextForController

		reconciler     *volume.Reconciler
		fakeVMProvider *providerfake.VMProviderA2
		volCtx         *volContext.VolumeContextA2
		vm             *vmopv1.VirtualMachine

		vmVol            vmopv1.VirtualMachineVolume
		vmVolumeWithPVC1 *vmopv1.VirtualMachineVolume
		vmVolumeWithPVC2 *vmopv1.VirtualMachineVolume

		vmVolForInstPVC1 *vmopv1.VirtualMachineVolume
	)

	BeforeEach(func() {
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

		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-vm",
				Namespace: "dummy-ns",
			},
			Status: vmopv1.VirtualMachineStatus{
				BiosUUID: dummyBiosUUID,
			},
		}

		vmVolForInstPVC1 = &vmopv1.VirtualMachineVolume{
			Name: "instance-pvc-1",
			VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
				PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
					PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: "instance-pvc-1",
					},
					InstanceVolumeClaim: &vmopv1.InstanceVolumeClaimVolumeSource{
						StorageClass: dummyInstanceStorageClassName,
						Size:         resource.MustParse("256Gi"),
					},
				},
			},
		}
	})

	JustBeforeEach(func() {
		ctx = suite.NewUnitTestContextForController(initObjects...)
		reconciler = volume.NewReconciler(
			ctx.Client,
			ctx.Logger,
			ctx.Recorder,
			ctx.VMProviderA2,
		)
		fakeVMProvider = ctx.VMProviderA2.(*providerfake.VMProviderA2)

		volCtx = &volContext.VolumeContextA2{
			Context: ctx,
			Logger:  ctx.Logger,
			VM:      vm,
		}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		initObjects = nil
		volCtx = nil
		reconciler = nil
	})

	getCNSAttachmentForVolumeName := func(vm *vmopv1.VirtualMachine, volumeName string) *cnsv1alpha1.CnsNodeVmAttachment {
		objectKey := client.ObjectKey{Name: volume.CNSAttachmentNameForVolume(vm.Name, volumeName), Namespace: vm.Namespace}
		attachment := &cnsv1alpha1.CnsNodeVmAttachment{}

		err := ctx.Client.Get(ctx, objectKey, attachment)
		if err == nil {
			return attachment
		}

		if k8sapierrors.IsNotFound(err) {
			return nil
		}

		ExpectWithOffset(1, err).ToNot(HaveOccurred())
		return nil
	}

	Context("ReconcileNormal", func() {

		When("Instance storage is configured on VM", func() {
			BeforeEach(func() {
				vmVol = *vmVolForInstPVC1
				vm.Spec.Volumes = append(vm.Spec.Volumes, vmVol)
				vm.Annotations = make(map[string]string)
				vm.Annotations[constants.InstanceStorageSelectedNodeAnnotationKey] = "selected-node.domain.com"
				vm.Annotations[constants.InstanceStorageSelectedNodeMOIDAnnotationKey] = "host-88"
			})

			JustBeforeEach(func() {
				volCtx.InstanceStorageFSSEnabled = true
			})

			It("selected-node annotation not set - no PVCs created", func() {
				delete(vm.Annotations, constants.InstanceStorageSelectedNodeAnnotationKey)
				delete(vm.Annotations, constants.InstanceStorageSelectedNodeMOIDAnnotationKey)
				err := reconciler.ReconcileNormal(volCtx)
				Expect(err).ToNot(HaveOccurred())
				expectPVCsStatus(volCtx, ctx, false, false, 0)
			})

			It("PVCs are created but not bound after selected-node annotation set", func() {
				err := reconciler.ReconcileNormal(volCtx)
				Expect(err).ToNot(HaveOccurred())
				expectPVCsStatus(volCtx, ctx, true, false, len(vm.Spec.Volumes))

				By("Multiple reconciles", func() {
					err := reconciler.ReconcileNormal(volCtx)
					Expect(err).ToNot(HaveOccurred())
					expectPVCsStatus(volCtx, ctx, true, false, len(vm.Spec.Volumes))
				})
			})

			It("PVCs are created and placement is failed - remove all error PVCs", func() {
				By("create PVCs and not realized", func() {
					Expect(reconciler.ReconcileNormal(volCtx)).To(Succeed())
					expectPVCsStatus(volCtx, ctx, true, false, len(vm.Spec.Volumes))
				})

				By("Adjust PVC CreationTimestamp", func() {
					adjustPVCCreationTimestamp(volCtx, ctx)
				})

				By("PVCs realization turned into error - remove all PVCs", func() {
					patchInstanceStoragePVCs(volCtx, ctx, false, true)
					Expect(reconciler.ReconcileNormal(volCtx)).To(Succeed())
					expectPVCsStatus(volCtx, ctx, false, false, 0)
				})
			})

			It("PVCs are created and realized", func() {
				By("create PVCs and not realized", func() {
					Expect(reconciler.ReconcileNormal(volCtx)).To(Succeed())
					expectPVCsStatus(volCtx, ctx, true, false, len(vm.Spec.Volumes))
				})

				By("PVCs are bound", func() {
					patchInstanceStoragePVCs(volCtx, ctx, true, false)
					Expect(reconciler.ReconcileNormal(volCtx)).To(Succeed())
					expectPVCsStatus(volCtx, ctx, true, true, len(vm.Spec.Volumes))
				})
			})

			It("Storage policy quota is insufficient - PVCs should not create", func() {
				tfc := testFailClient{reconciler.Client}
				reconciler.Client = &tfc
				err := reconciler.ReconcileNormal(volCtx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("insufficient quota"))
				expectPVCsStatus(volCtx, ctx, true, false, 0)
			})
		})

		When("VM does not have BiosUUID", func() {
			BeforeEach(func() {
				vmVol = *vmVolumeWithPVC1
				vm.Spec.Volumes = append(vm.Spec.Volumes, vmVol)
				vm.Status.BiosUUID = ""
			})

			It("returns success", func() {
				err := reconciler.ReconcileNormal(volCtx)
				Expect(err).ToNot(HaveOccurred())

				By("Did not create CnsNodeVmAttachment", func() {
					Expect(getCNSAttachmentForVolumeName(vm, vmVol.Name)).To(BeNil())
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

		When("CnsNodeVmAttachment exists for a different VM", func() {
			BeforeEach(func() {
				vmVol = *vmVolumeWithPVC1
				vm.Spec.Volumes = append(vm.Spec.Volumes, vmVol)
				attachment := cnsAttachmentForVMVolume(vm, vmVol)

				otherAttachment := cnsAttachmentForVMVolume(vm, *vmVolumeWithPVC2)
				otherAttachment.Spec.NodeUUID = "some-other-uuid"
				otherAttachment.Status.Attached = true
				otherAttachment.Status.AttachmentMetadata = map[string]string{
					volume.AttributeFirstClassDiskUUID: dummyDiskUUID,
				}
				initObjects = append(initObjects, attachment, otherAttachment)
			})

			It("returns success", func() {
				err := reconciler.ReconcileNormal(volCtx)
				Expect(err).ToNot(HaveOccurred())

				By("Ignores the CnsNodeVmAttachment for other VM", func() {
					Expect(vm.Status.Volumes).To(HaveLen(1))

					attachment := getCNSAttachmentForVolumeName(vm, vmVol.Name)
					Expect(attachment).ToNot(BeNil())
					assertVMVolStatusFromAttachment(vmVol, attachment, vm.Status.Volumes[0])
				})
			})
		})

		When("VM Spec.Volumes contains CNS volumes and VM isn't powered on", func() {
			var vmVol1, vmVol2 vmopv1.VirtualMachineVolume

			BeforeEach(func() {
				vmVol1 = *vmVolumeWithPVC1
				vmVol2 = *vmVolumeWithPVC2
				vm.Spec.Volumes = append(vm.Spec.Volumes, vmVol1, vmVol2)

				vm.Status.PowerState = vmopv1.VirtualMachinePowerStateOff
			})

			It("only allows one pending attachment at a time", func() {
				Expect(reconciler.ReconcileNormal(volCtx)).To(Succeed())

				By("Created first CnsNodeVmAttachment", func() {
					attachments := &cnsv1alpha1.CnsNodeVmAttachmentList{}
					Expect(ctx.Client.List(ctx, attachments, client.InNamespace(vm.Namespace))).To(Succeed())
					Expect(attachments.Items).To(HaveLen(1))

					attachment := getCNSAttachmentForVolumeName(vm, vmVol1.Name)
					Expect(attachment).ToNot(BeNil())
					assertAttachmentSpecFromVMVol(vm, vmVol1, attachment)

					By("Expected VM Status.Volumes", func() {
						Expect(vm.Status.Volumes).To(HaveLen(1))
						assertVMVolStatusFromAttachment(vmVol1, attachment, vm.Status.Volumes[0])
					})

					// Mark as attached to let next volume proceed.
					attachment.Status.Attached = true
					Expect(ctx.Client.Status().Update(ctx, attachment)).To(Succeed())
				})

				Expect(reconciler.ReconcileNormal(volCtx)).To(Succeed())

				By("First Volume is marked as attached", func() {
					Expect(vm.Status.Volumes).To(HaveLen(2))

					attachment := getCNSAttachmentForVolumeName(vm, vmVol1.Name)
					Expect(attachment).ToNot(BeNil())
					Expect(attachment.Status.Attached).To(BeTrue())
					assertVMVolStatusFromAttachment(vmVol1, attachment, vm.Status.Volumes[0])
				})

				By("Created second CnsNodeVmAttachment", func() {
					attachments := &cnsv1alpha1.CnsNodeVmAttachmentList{}
					Expect(ctx.Client.List(ctx, attachments, client.InNamespace(vm.Namespace))).To(Succeed())
					Expect(attachments.Items).To(HaveLen(2))

					Expect(getCNSAttachmentForVolumeName(vm, vmVol1.Name)).ToNot(BeNil())
					attachment := getCNSAttachmentForVolumeName(vm, vmVol2.Name)
					Expect(attachment).ToNot(BeNil())
					assertAttachmentSpecFromVMVol(vm, vmVol2, attachment)

					By("Expected VM Status.Volumes", func() {
						Expect(vm.Status.Volumes).To(HaveLen(2))
						assertVMVolStatusFromAttachment(vmVol2, attachment, vm.Status.Volumes[1])
					})

					attachment.Status.Attached = true
					Expect(ctx.Client.Status().Update(ctx, attachment)).To(Succeed())
				})

				Expect(reconciler.ReconcileNormal(volCtx)).To(Succeed())

				By("Second Volume is mark as attached", func() {
					Expect(vm.Status.Volumes).To(HaveLen(2))

					attachment := getCNSAttachmentForVolumeName(vm, vmVol2.Name)
					Expect(attachment).ToNot(BeNil())
					Expect(attachment.Status.Attached).To(BeTrue())
					assertVMVolStatusFromAttachment(vmVol2, attachment, vm.Status.Volumes[1])
				})
			})

			It("only allows one pending attachment at a time when attachment has an error", func() {
				Expect(reconciler.ReconcileNormal(volCtx)).To(Succeed())

				By("Created first CnsNodeVmAttachment", func() {
					attachments := &cnsv1alpha1.CnsNodeVmAttachmentList{}
					Expect(ctx.Client.List(ctx, attachments, client.InNamespace(vm.Namespace))).To(Succeed())
					Expect(attachments.Items).To(HaveLen(1))

					attachment := getCNSAttachmentForVolumeName(vm, vmVol1.Name)
					Expect(attachment).ToNot(BeNil())
					assertAttachmentSpecFromVMVol(vm, vmVol1, attachment)

					By("Expected VM Status.Volumes", func() {
						Expect(vm.Status.Volumes).To(HaveLen(1))
						assertVMVolStatusFromAttachment(vmVol1, attachment, vm.Status.Volumes[0])
					})

					// Mark as failure and ensure the second volume isn't created.
					attachment.Status.Error = "failure"
					Expect(ctx.Client.Status().Update(ctx, attachment)).To(Succeed())
				})

				Expect(reconciler.ReconcileNormal(volCtx)).To(Succeed())

				By("First Volume is marked with error", func() {
					Expect(vm.Status.Volumes).To(HaveLen(1))

					attachment := getCNSAttachmentForVolumeName(vm, vmVol1.Name)
					Expect(attachment).ToNot(BeNil())
					assertVMVolStatusFromAttachment(vmVol1, attachment, vm.Status.Volumes[0])
				})

				By("Does not create second CnsNodeVmAttachment", func() {
					attachments := &cnsv1alpha1.CnsNodeVmAttachmentList{}
					Expect(ctx.Client.List(ctx, attachments, client.InNamespace(vm.Namespace))).To(Succeed())
					Expect(attachments.Items).To(HaveLen(1))

					Expect(getCNSAttachmentForVolumeName(vm, vmVol1.Name)).ToNot(BeNil())
					Expect(getCNSAttachmentForVolumeName(vm, vmVol2.Name)).To(BeNil())
				})

				By("Simulate attach of first volume", func() {
					attachment := getCNSAttachmentForVolumeName(vm, vmVol1.Name)
					Expect(attachment).ToNot(BeNil())
					attachment.Status.Attached = true
					attachment.Status.Error = ""
					Expect(ctx.Client.Status().Update(ctx, attachment)).To(Succeed())
				})

				Expect(reconciler.ReconcileNormal(volCtx)).To(Succeed())

				By("First Volume is marked as attached", func() {
					Expect(vm.Status.Volumes).To(HaveLen(2))

					attachment := getCNSAttachmentForVolumeName(vm, vmVol1.Name)
					Expect(attachment).ToNot(BeNil())
					Expect(attachment.Status.Attached).To(BeTrue())
					assertVMVolStatusFromAttachment(vmVol1, attachment, vm.Status.Volumes[0])
				})

				By("Created second CnsNodeVmAttachment", func() {
					attachments := &cnsv1alpha1.CnsNodeVmAttachmentList{}
					Expect(ctx.Client.List(ctx, attachments, client.InNamespace(vm.Namespace))).To(Succeed())
					Expect(attachments.Items).To(HaveLen(2))

					attachment := getCNSAttachmentForVolumeName(vm, vmVol2.Name)
					Expect(attachment).ToNot(BeNil())
					assertAttachmentSpecFromVMVol(vm, vmVol2, attachment)

					By("Expected VM Status.Volumes", func() {
						Expect(vm.Status.Volumes).To(HaveLen(2))
						assertVMVolStatusFromAttachment(vmVol2, attachment, vm.Status.Volumes[1])
					})
				})

				Expect(reconciler.ReconcileNormal(volCtx)).To(Succeed())

				By("Expected VM Status.Volumes", func() {
					Expect(vm.Status.Volumes).To(HaveLen(2))

					attachment := getCNSAttachmentForVolumeName(vm, vmVol1.Name)
					Expect(attachment).ToNot(BeNil())
					assertVMVolStatusFromAttachment(vmVol1, attachment, vm.Status.Volumes[0])

					attachment = getCNSAttachmentForVolumeName(vm, vmVol2.Name)
					Expect(attachment).ToNot(BeNil())
					assertVMVolStatusFromAttachment(vmVol2, attachment, vm.Status.Volumes[1])
				})
			})
		})

		When("VM Spec.Volumes contains CNS volumes and need to get VM's hardware version", func() {
			BeforeEach(func() {
				vmVol = *vmVolumeWithPVC1
				vm.Spec.Volumes = append(vm.Spec.Volumes, vmVol)
			})

			It("returns error when failed to get VM hardware version", func() {
				fakeVMProvider.Lock()
				fakeVMProvider.GetVirtualMachineHardwareVersionFn = func(_ goctx.Context, _ *vmopv1.VirtualMachine) (int32, error) {
					return 0, errors.New("dummy-error")
				}
				fakeVMProvider.Unlock()

				err := reconciler.ReconcileNormal(volCtx)
				Expect(err).To(HaveOccurred())

				attachment := getCNSAttachmentForVolumeName(vm, vmVol.Name)

				Expect(attachment).To(BeNil())
				Expect(vm.Status.Volumes).To(BeEmpty())
			})

			It("returns error when VM hardware version is smaller than minimal requirement", func() {
				fakeVMProvider.Lock()
				fakeVMProvider.GetVirtualMachineHardwareVersionFn = func(_ goctx.Context, _ *vmopv1.VirtualMachine) (int32, error) {
					return 11, nil
				}
				fakeVMProvider.Unlock()

				err := reconciler.ReconcileNormal(volCtx)
				Expect(err).To(HaveOccurred())

				attachment := getCNSAttachmentForVolumeName(vm, vmVol.Name)

				Expect(attachment).To(BeNil())
				Expect(vm.Status.Volumes).To(BeEmpty())
			})

			It("returns success when failed to parse VM hardware version", func() {
				fakeVMProvider.Lock()
				fakeVMProvider.GetVirtualMachineHardwareVersionFn = func(_ goctx.Context, _ *vmopv1.VirtualMachine) (int32, error) {
					return 0, nil
				}
				fakeVMProvider.Unlock()

				err := reconciler.ReconcileNormal(volCtx)
				Expect(err).ToNot(HaveOccurred())

				attachment := getCNSAttachmentForVolumeName(vm, vmVol.Name)

				By("Created expected CnsNodeVmAttachment", func() {
					Expect(attachment).ToNot(BeNil())
					assertAttachmentSpecFromVMVol(vm, vmVol, attachment)
				})

				By("Expected VM Status.Volumes", func() {
					Expect(vm.Status.Volumes).To(HaveLen(1))
					assertVMVolStatusFromAttachment(vmVol, attachment, vm.Status.Volumes[0])
				})
			})
		})

		When("VM Spec.Volumes has CNS volume", func() {
			BeforeEach(func() {
				vmVol = *vmVolumeWithPVC1
				vm.Spec.Volumes = append(vm.Spec.Volumes, vmVol)
			})

			It("returns success", func() {
				err := reconciler.ReconcileNormal(volCtx)
				Expect(err).ToNot(HaveOccurred())

				attachment := getCNSAttachmentForVolumeName(vm, vmVol.Name)

				By("Created expected CnsNodeVmAttachment", func() {
					Expect(attachment).ToNot(BeNil())
					assertAttachmentSpecFromVMVol(vm, vmVol, attachment)
				})

				By("Expected VM Status.Volumes", func() {
					Expect(vm.Status.Volumes).To(HaveLen(1))
					assertVMVolStatusFromAttachment(vmVol, attachment, vm.Status.Volumes[0])
				})
			})
		})

		When("VM Spec.Volumes has CNS volume with existing CnsNodeVmAttachment", func() {
			dummyErrMsg := "vmware foobar 42"

			BeforeEach(func() {
				vmVol = *vmVolumeWithPVC1
				vm.Spec.Volumes = append(vm.Spec.Volumes, vmVol)

				attachment := cnsAttachmentForVMVolume(vm, vmVol)
				attachment.Status.Attached = true
				attachment.Status.AttachmentMetadata = map[string]string{
					volume.AttributeFirstClassDiskUUID: dummyDiskUUID,
				}
				attachment.Status.Error = dummyErrMsg
				initObjects = append(initObjects, attachment)
			})

			It("returns success", func() {
				err := reconciler.ReconcileNormal(volCtx)
				Expect(err).ToNot(HaveOccurred())

				By("Expected VM Status.Volumes", func() {
					Expect(vm.Status.Volumes).To(HaveLen(1))

					attachment := getCNSAttachmentForVolumeName(vm, vmVol.Name)
					Expect(attachment).ToNot(BeNil())
					assertVMVolStatusFromAttachment(vmVol, attachment, vm.Status.Volumes[0])
				})
			})
		})

		When("VM Spec.Volumes has CNS volume with an existing CnsNodeVmAttachment for a different VM", func() {

			When("CnsNodeVmAttachment has OwnerRef of different VM", func() {

				BeforeEach(func() {
					vmVol = *vmVolumeWithPVC1
					vm.Spec.Volumes = append(vm.Spec.Volumes, vmVol)

					attachment := cnsAttachmentForVMVolume(vm, vmVol)
					attachment.ObjectMeta.OwnerReferences[0].UID = "some-other-uuid"
					attachment.Spec.NodeUUID = "some-other-bios-uuid"
					attachment.Status.Attached = true
					attachment.Status.AttachmentMetadata = map[string]string{
						volume.AttributeFirstClassDiskUUID: dummyDiskUUID,
					}
					initObjects = append(initObjects, attachment)
				})

				It("returns expected error", func() {
					err := reconciler.ReconcileNormal(volCtx)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring(" has a different controlling owner"))

					By("Expected VM Status.Volumes", func() {
						Expect(vm.Status.Volumes).To(BeEmpty())
					})

					By("attachment still exists", func() {
						Expect(getCNSAttachmentForVolumeName(vm, vmVol.Name)).ToNot(BeNil())
					})
				})
			})

			When("CnsNodeVmAttachment has OwnerRef of this VM but different NodeUUID", func() {

				BeforeEach(func() {
					vmVol = *vmVolumeWithPVC1
					vm.Spec.Volumes = append(vm.Spec.Volumes, vmVol)

					attachment := cnsAttachmentForVMVolume(vm, vmVol)
					attachment.Spec.NodeUUID = "some-old-bios-uuid"
					attachment.Status.Attached = true
					attachment.Status.AttachmentMetadata = map[string]string{
						volume.AttributeFirstClassDiskUUID: dummyDiskUUID,
					}
					initObjects = append(initObjects, attachment)
				})

				It("returns expected error", func() {
					Expect(getCNSAttachmentForVolumeName(vm, vmVol.Name)).ToNot(BeNil())

					err := reconciler.ReconcileNormal(volCtx)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("deleted stale CnsNodeVmAttachment"))

					By("old attachment was deleted", func() {
						attachment := getCNSAttachmentForVolumeName(vm, vmVol.Name)
						Expect(attachment).To(BeNil())
					})

					By("returns success on next reconcile", func() {
						err := reconciler.ReconcileNormal(volCtx)
						Expect(err).ToNot(HaveOccurred())

						By("Expected VM Status.Volumes", func() {
							Expect(vm.Status.Volumes).To(HaveLen(1))

							attachment := getCNSAttachmentForVolumeName(vm, vmVol.Name)
							Expect(attachment).ToNot(BeNil())
							assertVMVolStatusFromAttachment(vmVol, attachment, vm.Status.Volumes[0])
						})
					})
				})
			})
		})

		When("VM Spec.Volumes has CNS volume with a SOAP error", func() {
			awfulErrMsg := `failed to attach cns volume: \"88854b48-2b1c-43f8-8889-de4b5ca2cab5\" to node vm: \"VirtualMachine:vm-42
[VirtualCenterHost: vc.vmware.com, UUID: 42080725-d6b0-c045-b24e-29c4dadca6f2, Datacenter: Datacenter
[Datacenter: Datacenter:datacenter, VirtualCenterHost: vc.vmware.com]]\".
fault: \"(*types.LocalizedMethodFault)(0xc003d9b9a0)({\\n DynamicData: (types.DynamicData)
{\\n },\\n Fault: (*types.ResourceInUse)(0xc002e69080)({\\n VimFault: (types.VimFault)
{\\n MethodFault: (types.MethodFault) {\\n FaultCause: (*types.LocalizedMethodFault)(\u003cnil\u003e),\\n
FaultMessage: ([]types.LocalizableMessage) \u003cnil\u003e\\n }\\n },\\n Type: (string) \\\"\\\",\\n Name:
(string) (len=6) \\\"volume\\\"\\n }),\\n LocalizedMessage: (string) (len=32)
\\\"The resource 'volume' is in use.\\\"\\n})\\n\". opId: \"67d69c68\""
`

			BeforeEach(func() {
				vmVol = *vmVolumeWithPVC1
				vm.Spec.Volumes = append(vm.Spec.Volumes, vmVol)

				attachment := cnsAttachmentForVMVolume(vm, vmVol)
				attachment.Status.Attached = true
				attachment.Status.AttachmentMetadata = map[string]string{
					volume.AttributeFirstClassDiskUUID: dummyDiskUUID,
				}
				attachment.Status.Error = awfulErrMsg
				initObjects = append(initObjects, attachment)
			})

			It("returns success", func() {
				err := reconciler.ReconcileNormal(volCtx)
				Expect(err).ToNot(HaveOccurred())

				By("Expected VM Status.Volumes with sanitized error", func() {
					Expect(vm.Status.Volumes).To(HaveLen(1))

					attachment := getCNSAttachmentForVolumeName(vm, vmVol.Name)
					Expect(attachment).ToNot(BeNil())
					attachment.Status.Error = "failed to attach cns volume"
					assertVMVolStatusFromAttachment(vmVol, attachment, vm.Status.Volumes[0])
				})
			})
		})

		When("VM has orphaned CNS volume in Status.Volumes", func() {
			var attachment *cnsv1alpha1.CnsNodeVmAttachment

			BeforeEach(func() {
				vmVol = *vmVolumeWithPVC1
				vm.Status.Volumes = append(vm.Status.Volumes, vmopv1.VirtualMachineVolumeStatus{
					Name:     vmVol.Name,
					DiskUUID: dummyDiskUUID,
				})

				attachment = cnsAttachmentForVMVolume(vm, vmVol)
				attachment.Status.Attached = true
				attachment.Status.AttachmentMetadata = map[string]string{
					volume.AttributeFirstClassDiskUUID: dummyDiskUUID,
				}
				initObjects = append(initObjects, attachment)
			})

			It("returns success", func() {
				err := reconciler.ReconcileNormal(volCtx)
				Expect(err).ToNot(HaveOccurred())
				Expect(vm.Spec.Volumes).To(BeEmpty())

				By("Orphaned CNS volume preserved in Status.Volumes", func() {
					Expect(vm.Status.Volumes).To(HaveLen(1))
					assertVMVolStatusFromAttachment(vmVol, attachment, vm.Status.Volumes[0])

					// Not in VM Spec so should be deleted.
					Expect(getCNSAttachmentForVolumeName(vm, vmVol.Name)).To(BeNil())
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
				vm.Status.PowerState = vmopv1.VirtualMachinePowerStateOn
			})

			// We sort by DiskUUID, but the CnsNodeVmAttachment haven't been "attached" yet,
			// so expect the Spec.Volumes order.
			When("CnsNodeVmAttachments do not have DiskUUID set", func() {
				It("returns success", func() {
					err := reconciler.ReconcileNormal(volCtx)
					Expect(err).ToNot(HaveOccurred())

					attachment1 := getCNSAttachmentForVolumeName(vm, vmVol1.Name)
					Expect(attachment1).ToNot(BeNil())
					assertAttachmentSpecFromVMVol(vm, vmVol1, attachment1)

					attachment2 := getCNSAttachmentForVolumeName(vm, vmVol2.Name)
					Expect(attachment2).ToNot(BeNil())
					assertAttachmentSpecFromVMVol(vm, vmVol2, attachment2)

					By("VM Status.Volumes are stable-sorted by Spec.Volumes order", func() {
						Expect(vm.Status.Volumes).To(HaveLen(2))
						assertVMVolStatusFromAttachment(vmVol1, attachment1, vm.Status.Volumes[0])
						assertVMVolStatusFromAttachment(vmVol2, attachment2, vm.Status.Volumes[1])
					})
				})
			})

			When("CnsNodeVmAttachments have DiskUUID set", func() {
				dummyDiskUUID1 := "z"
				dummyDiskUUID2 := "a"

				BeforeEach(func() {
					attachment1 := cnsAttachmentForVMVolume(vm, vmVol1)
					attachment1.Status.Attached = true
					attachment1.Status.AttachmentMetadata = map[string]string{
						volume.AttributeFirstClassDiskUUID: dummyDiskUUID1,
					}

					attachment2 := cnsAttachmentForVMVolume(vm, vmVol2)
					attachment2.Status.Attached = true
					attachment2.Status.AttachmentMetadata = map[string]string{
						volume.AttributeFirstClassDiskUUID: dummyDiskUUID2,
					}

					initObjects = append(initObjects, attachment1, attachment2)
				})

				It("returns success", func() {
					err := reconciler.ReconcileNormal(volCtx)
					Expect(err).ToNot(HaveOccurred())

					attachment1 := getCNSAttachmentForVolumeName(vm, vmVol1.Name)
					Expect(attachment1).ToNot(BeNil())
					assertAttachmentSpecFromVMVol(vm, vmVol1, attachment1)

					attachment2 := getCNSAttachmentForVolumeName(vm, vmVol2.Name)
					Expect(attachment2).ToNot(BeNil())
					assertAttachmentSpecFromVMVol(vm, vmVol2, attachment2)

					By("VM Status.Volumes are sorted by DiskUUID", func() {
						Expect(vm.Status.Volumes).To(HaveLen(2))
						assertVMVolStatusFromAttachment(vmVol2, attachment2, vm.Status.Volumes[0])
						assertVMVolStatusFromAttachment(vmVol1, attachment1, vm.Status.Volumes[1])
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

func cnsAttachmentForVMVolume(
	vm *vmopv1.VirtualMachine,
	vmVol vmopv1.VirtualMachineVolume) *cnsv1alpha1.CnsNodeVmAttachment {
	t := true
	return &cnsv1alpha1.CnsNodeVmAttachment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      volume.CNSAttachmentNameForVolume(vm.Name, vmVol.Name),
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
		Spec: cnsv1alpha1.CnsNodeVmAttachmentSpec{
			NodeUUID:   vm.Status.BiosUUID,
			VolumeName: vmVol.PersistentVolumeClaim.ClaimName,
		},
	}
}

func expectPVCsStatus(ctx *volContext.VolumeContextA2, testCtx *builder.UnitTestContextForController, selectedNodeSet, bound bool, pvcsCount int) {
	if selectedNodeSet {
		Expect(ctx.VM.Annotations).To(HaveKey(constants.InstanceStorageSelectedNodeAnnotationKey))
		Expect(ctx.VM.Annotations).To(HaveKey(constants.InstanceStorageSelectedNodeMOIDAnnotationKey))
	} else {
		Expect(ctx.VM.Annotations).ToNot(HaveKey(constants.InstanceStorageSelectedNodeAnnotationKey))
		Expect(ctx.VM.Annotations).ToNot(HaveKey(constants.InstanceStorageSelectedNodeMOIDAnnotationKey))
	}

	if bound {
		Expect(ctx.VM.Annotations).To(HaveKey(constants.InstanceStoragePVCsBoundAnnotationKey))
	} else {
		Expect(ctx.VM.Annotations).ToNot(HaveKey(constants.InstanceStoragePVCsBoundAnnotationKey))
	}

	pvcList, err := getInstanceStoragePVCs(ctx, testCtx)
	Expect(err).ToNot(HaveOccurred())
	Expect(pvcList).To(HaveLen(pvcsCount))
}

func adjustPVCCreationTimestamp(ctx *volContext.VolumeContextA2, testCtx *builder.UnitTestContextForController) {
	pvcList, err := getInstanceStoragePVCs(ctx, testCtx)
	Expect(err).ToNot(HaveOccurred())

	for _, pvc := range pvcList {
		pvc := pvc
		pvc.CreationTimestamp = metav1.NewTime(time.Now().Add(-2 * lib.GetInstanceStoragePVPlacementFailedTTL()))
		Expect(testCtx.Client.Update(ctx, &pvc)).To(Succeed())
	}
}

func getInstanceStoragePVCs(ctx *volContext.VolumeContextA2, testCtx *builder.UnitTestContextForController) ([]corev1.PersistentVolumeClaim, error) {
	var errs []error
	pvcList := make([]corev1.PersistentVolumeClaim, 0)

	volumes := volume.FilterVolumesA2(ctx.VM)
	for _, vol := range volumes {
		objKey := client.ObjectKey{
			Namespace: ctx.VM.Namespace,
			Name:      vol.PersistentVolumeClaim.ClaimName,
		}
		pvc := &corev1.PersistentVolumeClaim{}
		if err := testCtx.Client.Get(ctx, objKey, pvc); client.IgnoreNotFound(err) != nil {
			errs = append(errs, err)
			continue
		}
		// The system returns empty object when no PVC resources exists in the system.
		// This check filters out unwanted objects that causes assertions fail.
		if !metav1.IsControlledBy(pvc, ctx.VM) {
			continue
		}
		pvcList = append(pvcList, *pvc)
	}

	return pvcList, k8serrors.NewAggregate(errs)
}

func patchInstanceStoragePVCs(ctx *volContext.VolumeContextA2, testCtx *builder.UnitTestContextForController, setStatusBound, setErrorAnnotation bool) {
	pvcList, err := getInstanceStoragePVCs(ctx, testCtx)
	Expect(err).ToNot(HaveOccurred())

	for _, pvc := range pvcList {
		pvc := pvc
		if setStatusBound {
			pvc.Status.Phase = corev1.ClaimBound
		}
		if setErrorAnnotation {
			if pvc.Annotations == nil {
				pvc.Annotations = make(map[string]string)
			}
			pvc.Annotations[constants.InstanceStoragePVPlacementErrorAnnotationKey] = constants.InstanceStorageNotEnoughResErr
		} else {
			delete(pvc.Annotations, constants.InstanceStoragePVPlacementErrorAnnotationKey)
		}

		Expect(testCtx.Client.Update(ctx, &pvc)).To(Succeed())
		Expect(testCtx.Client.Status().Update(ctx, &pvc)).To(Succeed())
	}
}

func assertAttachmentSpecFromVMVol(
	vm *vmopv1.VirtualMachine,
	vmVol vmopv1.VirtualMachineVolume,
	attachment *cnsv1alpha1.CnsNodeVmAttachment) {

	ExpectWithOffset(1, attachment.Spec.NodeUUID).To(Equal(vm.Status.BiosUUID))
	ExpectWithOffset(1, attachment.Spec.VolumeName).To(Equal(vmVol.PersistentVolumeClaim.ClaimName))

	ownerRefs := attachment.GetOwnerReferences()
	ExpectWithOffset(1, ownerRefs).To(HaveLen(1))
	ownerRef := ownerRefs[0]
	ExpectWithOffset(1, ownerRef.Name).To(Equal(vm.Name))
	ExpectWithOffset(1, ownerRef.Controller).ToNot(BeNil())
	ExpectWithOffset(1, *ownerRef.Controller).To(BeTrue())
}

func assertVMVolStatusFromAttachment(
	vmVol vmopv1.VirtualMachineVolume,
	attachment *cnsv1alpha1.CnsNodeVmAttachment,
	vmVolStatus vmopv1.VirtualMachineVolumeStatus) {
	diskUUID := attachment.Status.AttachmentMetadata[volume.AttributeFirstClassDiskUUID]

	ExpectWithOffset(1, vmVolStatus.Name).To(Equal(vmVol.Name))
	ExpectWithOffset(1, vmVolStatus.Attached).To(Equal(attachment.Status.Attached))
	ExpectWithOffset(1, vmVolStatus.DiskUUID).To(Equal(diskUUID))
	ExpectWithOffset(1, vmVolStatus.Error).To(Equal(attachment.Status.Error))
}
