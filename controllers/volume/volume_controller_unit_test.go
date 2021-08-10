// Copyright (c) 2020-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package volume_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/controllers/volume"
	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/pkg/syncer/cnsoperator/apis/cnsnodevmattachment/v1alpha1"
	volContext "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func unitTests() {
	Describe("Invoking Reconcile", unitTestsReconcile)
}

func unitTestsReconcile() {
	const (
		dummyBiosUUID = "dummy-bios-uuid"
		dummyDiskUUID = "111-222-333-disk-uuid"
	)

	var (
		initObjects []client.Object
		ctx         *builder.UnitTestContextForController

		reconciler *volume.Reconciler
		volCtx     *volContext.VolumeContext
		vm         *vmopv1alpha1.VirtualMachine

		vmVol               vmopv1alpha1.VirtualMachineVolume
		vmVolumeWithVsphere *vmopv1alpha1.VirtualMachineVolume
		vmVolumeWithPVC1    *vmopv1alpha1.VirtualMachineVolume
		vmVolumeWithPVC2    *vmopv1alpha1.VirtualMachineVolume
		attachment          *cnsv1alpha1.CnsNodeVmAttachment
	)

	BeforeEach(func() {
		vmVolumeWithVsphere = &vmopv1alpha1.VirtualMachineVolume{
			Name:          "vsphere-volume",
			VsphereVolume: &vmopv1alpha1.VsphereVolumeSource{},
		}

		vmVolumeWithPVC1 = &vmopv1alpha1.VirtualMachineVolume{
			Name: "cns-volume-1",
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: "pvc-volume-1",
			},
		}

		vmVolumeWithPVC2 = &vmopv1alpha1.VirtualMachineVolume{
			Name: "cns-volume-2",
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: "pvc-volume-2",
			},
		}

		vm = &vmopv1alpha1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-vm",
				Namespace: "dummy-ns",
			},
			Status: vmopv1alpha1.VirtualMachineStatus{
				BiosUUID: dummyBiosUUID,
			},
		}
	})

	JustBeforeEach(func() {
		ctx = suite.NewUnitTestContextForController(initObjects...)
		reconciler = volume.NewReconciler(
			ctx.Client,
			ctx.Logger,
			ctx.Recorder,
			ctx.Scheme,
		)

		volCtx = &volContext.VolumeContext{
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
		attachment = nil
		reconciler = nil
	})

	getCNSAttachmentForVolumeName := func(vm *vmopv1alpha1.VirtualMachine, volumeName string) *cnsv1alpha1.CnsNodeVmAttachment {
		objectKey := client.ObjectKey{Name: volume.CNSAttachmentNameForVolume(vm, volumeName), Namespace: vm.Namespace}
		attachment := &cnsv1alpha1.CnsNodeVmAttachment{}

		err := ctx.Client.Get(ctx, objectKey, attachment)
		if err == nil {
			return attachment
		}

		if errors.IsNotFound(err) {
			return nil
		}

		ExpectWithOffset(1, err).ToNot(HaveOccurred())
		return nil
	}

	Context("ReconcileNormal", func() {
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
					attachment := getCNSAttachmentForVolumeName(vm, vmVol.Name)
					Expect(attachment).To(BeNil())
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
				attachment = cnsAttachmentForVMVolume(vm, vmVol)

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
					assertVMVolStatusFromAttachment(vmVol, attachment, vm.Status.Volumes[0])
				})
			})
		})

		When("VM Spec.Volumes contains Vsphere volume", func() {
			BeforeEach(func() {
				vm.Spec.Volumes = append(vm.Spec.Volumes, *vmVolumeWithVsphere)
			})

			It("returns success", func() {
				err := reconciler.ReconcileNormal(volCtx)
				Expect(err).ToNot(HaveOccurred())

				Expect(vm.Spec.Volumes).To(HaveLen(1))
				Expect(vm.Status.Volumes).To(BeEmpty())
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

				By("Created expected CnsNodeVmAttachment", func() {
					attachment = getCNSAttachmentForVolumeName(vm, vmVol.Name)
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

				attachment = cnsAttachmentForVMVolume(vm, vmVol)
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
					assertVMVolStatusFromAttachment(vmVol, attachment, vm.Status.Volumes[0])
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

				attachment = cnsAttachmentForVMVolume(vm, vmVol)
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
					attachment.Status.Error = "failed to attach cns volume"
					assertVMVolStatusFromAttachment(vmVol, attachment, vm.Status.Volumes[0])
				})
			})
		})

		When("VM has orphaned CNS volume in Status.Volumes", func() {
			BeforeEach(func() {
				vmVol = *vmVolumeWithPVC1
				vm.Status.Volumes = append(vm.Status.Volumes, vmopv1alpha1.VirtualMachineVolumeStatus{
					Name:     vmVol.Name,
					DiskUuid: dummyDiskUUID,
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
				})
			})
		})

		When("VM Status.Volumes is sorted as expected", func() {
			var vmVol1 vmopv1alpha1.VirtualMachineVolume
			var vmVol2 vmopv1alpha1.VirtualMachineVolume

			BeforeEach(func() {
				vmVol1 = *vmVolumeWithPVC1
				vmVol2 = *vmVolumeWithPVC2
				vm.Spec.Volumes = append(vm.Spec.Volumes, vmVol1, vmVol2)
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
	vm *vmopv1alpha1.VirtualMachine,
	vmVol vmopv1alpha1.VirtualMachineVolume) *cnsv1alpha1.CnsNodeVmAttachment {
	t := true
	return &cnsv1alpha1.CnsNodeVmAttachment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      volume.CNSAttachmentNameForVolume(vm, vmVol.Name),
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

func assertAttachmentSpecFromVMVol(
	vm *vmopv1alpha1.VirtualMachine,
	vmVol vmopv1alpha1.VirtualMachineVolume,
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
	vmVol vmopv1alpha1.VirtualMachineVolume,
	attachment *cnsv1alpha1.CnsNodeVmAttachment,
	vmVolStatus vmopv1alpha1.VirtualMachineVolumeStatus) {
	diskUUID := attachment.Status.AttachmentMetadata[volume.AttributeFirstClassDiskUUID]

	ExpectWithOffset(1, vmVolStatus.Name).To(Equal(vmVol.Name))
	ExpectWithOffset(1, vmVolStatus.Attached).To(Equal(attachment.Status.Attached))
	ExpectWithOffset(1, vmVolStatus.DiskUuid).To(Equal(diskUUID))
	ExpectWithOffset(1, vmVolStatus.Error).To(Equal(attachment.Status.Error))
}
