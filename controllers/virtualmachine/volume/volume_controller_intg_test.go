// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package volume_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/pkg/syncer/cnsoperator/apis/cnsnodevmattachment/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachine/volume"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/pkg/patch"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {
	Describe(
		"Reconcile",
		Label(
			testlabels.Controller,
			testlabels.EnvTest,
			testlabels.API,
		),
		intgTestsReconcile,
	)
}

func intgTestsReconcile() {
	var (
		ctx *builder.IntegrationTestContext

		vm                    *vmopv1.VirtualMachine
		vmKey                 types.NamespacedName
		vmVolume1             vmopv1.VirtualMachineVolume
		vmVolume2             vmopv1.VirtualMachineVolume
		pvc1                  *corev1.PersistentVolumeClaim
		pvc2                  *corev1.PersistentVolumeClaim
		dummyBiosUUID         string
		dummyDiskUUID1        string
		dummyDiskUUID2        string
		dummySelectedNode     string
		dummySelectedNodeMOID string
	)

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()

		dummyBiosUUID = uuid.New().String()
		dummyDiskUUID1 = uuid.New().String()
		dummyDiskUUID2 = uuid.New().String()

		vmVolume1 = vmopv1.VirtualMachineVolume{
			Name: "cns-volume-1",
			VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
				PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
					PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: "pvc-volume-1",
					},
				},
			},
		}

		pvc1 = &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      vmVolume1.VirtualMachineVolumeSource.PersistentVolumeClaim.ClaimName,
				Namespace: ctx.Namespace,
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse("1Gi"),
					},
				},
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
			},
		}

		vmVolume2 = vmopv1.VirtualMachineVolume{
			Name: "cns-volume-2",
			VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
				PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
					PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: "pvc-volume-2",
					},
				},
			},
		}

		pvc2 = &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      vmVolume2.VirtualMachineVolumeSource.PersistentVolumeClaim.ClaimName,
				Namespace: ctx.Namespace,
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse("2Gi"),
					},
				},
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
			},
		}

		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ctx.Namespace,
				Name:      "dummy-vm",
			},
			Spec: vmopv1.VirtualMachineSpec{
				ImageName:  "dummy-image",
				PowerState: vmopv1.VirtualMachinePowerStateOff,
			},
		}
		vmKey = types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace}

		dummySelectedNode = "selected-node.domain.com"
		dummySelectedNodeMOID = "host-88"
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	getVirtualMachine := func(objKey types.NamespacedName) *vmopv1.VirtualMachine {
		vm := &vmopv1.VirtualMachine{}
		if err := ctx.Client.Get(ctx, objKey, vm); err != nil {
			return nil
		}
		return vm
	}

	getCnsNodeVMAttachment := func(vm *vmopv1.VirtualMachine, vmVol vmopv1.VirtualMachineVolume) *cnsv1alpha1.CnsNodeVmAttachment {
		objectKey := client.ObjectKey{Name: util.CNSAttachmentNameForVolume(vm.Name, vmVol.Name), Namespace: vm.Namespace}
		attachment := &cnsv1alpha1.CnsNodeVmAttachment{}
		if err := ctx.Client.Get(ctx, objectKey, attachment); err == nil {
			return attachment
		}
		return nil
	}

	waitForVirtualMachineInstanceStorage := func(objKey types.NamespacedName, selectedNodeSet, bound bool) {
		getAnnotations := func(g Gomega) map[string]string {
			vm := getVirtualMachine(objKey)
			g.Expect(vm).ToNot(BeNil())
			return vm.Annotations
		}

		desc := fmt.Sprintf("waiting for selected-node annotation set %v", selectedNodeSet)
		if selectedNodeSet {
			Eventually(getAnnotations).Should(HaveKey(constants.InstanceStorageSelectedNodeAnnotationKey), desc)
			Eventually(getAnnotations).Should(HaveKey(constants.InstanceStorageSelectedNodeMOIDAnnotationKey), desc)
		} else {
			Eventually(getAnnotations).ShouldNot(HaveKey(constants.InstanceStorageSelectedNodeAnnotationKey), desc)
			Eventually(getAnnotations).ShouldNot(HaveKey(constants.InstanceStorageSelectedNodeMOIDAnnotationKey), desc)
		}

		desc = fmt.Sprintf("waiting for VirtualMachine instance storage volumes BOUND status set to %v", bound)
		if bound {
			Eventually(getAnnotations).Should(HaveKey(constants.InstanceStoragePVCsBoundAnnotationKey), desc)
		} else {
			Eventually(getAnnotations).ShouldNot(HaveKey(constants.InstanceStoragePVCsBoundAnnotationKey), desc)
		}
	}

	waitForInstanceStoragePVCs := func(objKey types.NamespacedName) {
		vm := getVirtualMachine(objKey)
		Expect(vm).ToNot(BeNil())

		volumes := vmopv1util.FilterInstanceStorageVolumes(vm)
		Expect(volumes).ToNot(BeEmpty())

		Eventually(func(g Gomega) {
			for _, vol := range volumes {
				pvcKey := client.ObjectKey{
					Namespace: vm.Namespace,
					Name:      vol.PersistentVolumeClaim.ClaimName,
				}

				pvc := &corev1.PersistentVolumeClaim{}
				g.Expect(ctx.Client.Get(ctx, pvcKey, pvc)).To(Succeed())

				isClaim := vol.PersistentVolumeClaim.InstanceVolumeClaim
				g.Expect(pvc.Labels).To(HaveKey(constants.InstanceStorageLabelKey))
				g.Expect(pvc.Annotations).To(HaveKeyWithValue(constants.KubernetesSelectedNodeAnnotationKey, dummySelectedNode))
				g.Expect(pvc.Spec.StorageClassName).To(HaveValue(Equal(isClaim.StorageClass)))
				g.Expect(pvc.Spec.Resources.Requests).To(HaveKeyWithValue(corev1.ResourceStorage, isClaim.Size))
				g.Expect(pvc.Spec.AccessModes).To(ConsistOf(corev1.ReadWriteOnce))
			}
		}).Should(Succeed(), "waiting for instance storage PVCs to be created")
	}

	waitForInstanceStoragePVCsToBeDeleted := func(objKey types.NamespacedName) {
		vm := getVirtualMachine(objKey)
		Expect(vm).ToNot(BeNil())

		volumes := vmopv1util.FilterInstanceStorageVolumes(vm)
		Expect(volumes).ToNot(BeEmpty())

		Eventually(func(g Gomega) {
			for _, vol := range volumes {
				pvcKey := client.ObjectKey{
					Namespace: vm.Namespace,
					Name:      vol.PersistentVolumeClaim.ClaimName,
				}

				// PVCs gets finalizer set on creation so expect the deletion timestamp to be set.
				pvc := &corev1.PersistentVolumeClaim{}
				err := ctx.Client.Get(ctx, pvcKey, pvc)
				if err == nil {
					g.Expect(pvc.DeletionTimestamp.IsZero()).To(BeFalse())
				} else {
					g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
				}
			}
		}).Should(Succeed(), "waiting for instance storage PVCs to be deleted")
	}

	patchInstanceStoragePVCs := func(objKey types.NamespacedName, setStatusBound, setErrorAnnotation bool) {
		vm := getVirtualMachine(objKey)
		Expect(vm).ToNot(BeNil())

		volumes := vmopv1util.FilterInstanceStorageVolumes(vm)
		Expect(volumes).ToNot(BeEmpty())

		for _, vol := range volumes {
			pvcKey := client.ObjectKey{
				Namespace: vm.Namespace,
				Name:      vol.PersistentVolumeClaim.ClaimName,
			}

			pvc := &corev1.PersistentVolumeClaim{}
			err := ctx.Client.Get(ctx, pvcKey, pvc)
			Expect(err).ToNot(HaveOccurred())

			patchHelper, err := patch.NewHelper(pvc, ctx.Client)
			Expect(err).ToNot(HaveOccurred())

			if setStatusBound {
				pvc.Status.Phase = corev1.ClaimBound
			}
			if setErrorAnnotation {
				if pvc.Annotations == nil {
					pvc.Annotations = make(map[string]string)
				}
				pvc.Annotations[constants.InstanceStoragePVPlacementErrorAnnotationKey] = constants.InstanceStorageNotEnoughResErr
			}
			Expect(patchHelper.Patch(ctx, pvc)).To(Succeed())
		}
	}

	Context("Reconcile Instance Storage", func() {
		BeforeEach(func() {
			pkgcfg.SetContext(suite, func(config *pkgcfg.Config) {
				config.Features.InstanceStorage = true
				config.InstanceStorage.PVPlacementFailedTTL = 0
			})
		})

		JustBeforeEach(func() {
			vm.Spec.Volumes = append(vm.Spec.Volumes, builder.DummyInstanceStorageVirtualMachineVolumes()...)
			vm.Labels = map[string]string{constants.InstanceStorageLabelKey: "true"}
			Expect(ctx.Client.Create(ctx, vm)).To(Succeed())

			// This is line is due to a change in controller-runtime's change to
			// Create/Update calls that nils out empty fields. If this was set
			// prior to the above Create call, then vm.Metadata.Annotations
			// would be reset to nil.
			vm.Annotations = map[string]string{}
		})

		AfterEach(func() {
			pkgcfg.SetContext(suite, func(config *pkgcfg.Config) {
				config.Features.InstanceStorage = false
			})

			err := ctx.Client.Delete(ctx, vm)
			Expect(err == nil || apierrors.IsNotFound(err)).To(BeTrue())
		})

		It("Reconcile instance storage PVCs - selected-node annotation not set", func() {
			waitForVirtualMachineInstanceStorage(vmKey, false, false)
		})

		It("Reconcile instance storage PVCs - PVCs should be created and realized after setting selected-node annotation", func() {
			By("set selected-node annotation", func() {
				vm.Annotations[constants.InstanceStorageSelectedNodeAnnotationKey] = dummySelectedNode
				vm.Annotations[constants.InstanceStorageSelectedNodeMOIDAnnotationKey] = dummySelectedNodeMOID
				Expect(ctx.Client.Update(ctx, vm)).To(Succeed())
			})
			By("PVCs should be created", func() {
				waitForInstanceStoragePVCs(vmKey)
			})
			By("realize PVCs by setting PVC status to BOUND", func() {
				patchInstanceStoragePVCs(vmKey, true, false)
			})
			By("PVCs should be bound", func() {
				waitForVirtualMachineInstanceStorage(vmKey, true, true)
			})
		})

		It("Reconcile instance storage PVCs - PVCs should be deleted after PVCs turned into error", func() {
			By("set selected-node annotation", func() {
				vm.Annotations[constants.InstanceStorageSelectedNodeAnnotationKey] = dummySelectedNode
				vm.Annotations[constants.InstanceStorageSelectedNodeMOIDAnnotationKey] = dummySelectedNodeMOID
				Expect(ctx.Client.Update(ctx, vm)).To(Succeed())
			})
			By("PVCs should be created", func() {
				waitForInstanceStoragePVCs(vmKey)
			})
			By("Set PVCs with errors", func() {
				patchInstanceStoragePVCs(vmKey, false, true)
			})
			By("PVCs should be deleted", func() {
				waitForInstanceStoragePVCsToBeDeleted(vmKey)
			})
			By("selected-node annotations should be removed", func() {
				waitForVirtualMachineInstanceStorage(vmKey, false, false)
			})
		})

		When("VM has non-empty spec.crypto.encryptionClassName", func() {
			BeforeEach(func() {
				if vm.Annotations == nil {
					vm.Annotations = map[string]string{}
				}
				vm.Annotations[constants.InstanceStorageSelectedNodeAnnotationKey] = dummySelectedNode
				vm.Annotations[constants.InstanceStorageSelectedNodeMOIDAnnotationKey] = dummySelectedNodeMOID

				vm.Spec.Crypto = &vmopv1.VirtualMachineCryptoSpec{
					EncryptionClassName: "my-encryption-class",
				}
			})
			Specify("PVCs are created with the expected annotation", func() {
				Expect(ctx.Client.Get(ctx, vmKey, vm)).To(Succeed())

				volumes := vmopv1util.FilterInstanceStorageVolumes(vm)
				Expect(volumes).ToNot(BeEmpty())

				Eventually(func(g Gomega) {
					for i := range volumes {
						var obj corev1.PersistentVolumeClaim
						g.Expect(ctx.Client.Get(
							ctx,
							client.ObjectKey{
								Namespace: vm.Namespace,
								Name:      volumes[i].PersistentVolumeClaim.ClaimName,
							},
							&obj)).To(Succeed())
						g.Expect(obj.Annotations).To(HaveKeyWithValue(
							constants.EncryptionClassNameAnnotation,
							vm.Spec.Crypto.EncryptionClassName))
					}
				}).Should(Succeed())
			})
		})
	})

	Context("Reconcile", func() {
		BeforeEach(func() {
		})

		AfterEach(func() {
			err := ctx.Client.Delete(ctx, vm)
			Expect(err == nil || apierrors.IsNotFound(err)).To(BeTrue())
		})

		When("VM has a pause annotation", func() {
			BeforeEach(func() {
				vm.Annotations = make(map[string]string)
				vm.Annotations[vmopv1.PauseAnnotation] = "true"
			})

			It("Reconciles VM", func() {
				Expect(ctx.Client.Create(ctx, vm)).To(Succeed())

				vm = getVirtualMachine(vmKey)
				Expect(vm).ToNot(BeNil())

				By("VM has no volumes", func() {
					Expect(vm.Spec.Volumes).To(BeEmpty())
					Expect(vm.Status.Volumes).To(BeEmpty())
				})

				By("Assign VM BiosUUID", func() {
					vm.Status.BiosUUID = dummyBiosUUID
					Expect(ctx.Client.Status().Update(ctx, vm)).To(Succeed())
				})

				By("Add CNS volume to Spec.Volumes", func() {
					vm.Spec.Volumes = append(vm.Spec.Volumes, vmVolume1)
					Expect(ctx.Client.Update(ctx, vm)).To(Succeed())
				})

				By("CnsNodeVmAttachment should not be created for volume", func() {
					Consistently(func(g Gomega) {
						g.Expect(getCnsNodeVMAttachment(vm, vmVolume1)).To(BeNil())
					}, "3s").Should(Succeed())
				})
			})
		})

		It("Reconciles VirtualMachine Spec.Volumes", func() {
			Expect(ctx.Client.Create(ctx, vm)).To(Succeed())

			vm = getVirtualMachine(vmKey)
			Expect(vm).ToNot(BeNil())

			By("VM has no volumes", func() {
				Expect(vm.Spec.Volumes).To(BeEmpty())
				Expect(vm.Status.Volumes).To(BeEmpty())
			})

			By("Assign VM BiosUUID", func() {
				vm.Status.BiosUUID = dummyBiosUUID
				Expect(ctx.Client.Status().Update(ctx, vm)).To(Succeed())
			})

			By("Add CNS volume to Spec.Volumes", func() {
				By("Create PVC and Bind", func() {
					Expect(ctx.Client.Create(ctx, pvc1)).To(Succeed())
					pvc1.Status.Phase = corev1.ClaimBound
					Expect(ctx.Client.Status().Update(ctx, pvc1)).To(Succeed())
				})
				vm.Spec.Volumes = append(vm.Spec.Volumes, vmVolume1)
				Expect(ctx.Client.Update(ctx, vm)).To(Succeed())
			})

			By("CnsNodeVmAttachment should be created for volume", func() {
				var attachment *cnsv1alpha1.CnsNodeVmAttachment
				Eventually(func(g Gomega) {
					attachment = getCnsNodeVMAttachment(vm, vmVolume1)
					g.Expect(attachment).ToNot(BeNil())
				}).Should(Succeed())

				Expect(attachment.Spec.NodeUUID).To(Equal(dummyBiosUUID))
				Expect(attachment.Spec.VolumeName).To(Equal(vmVolume1.PersistentVolumeClaim.ClaimName))
			})

			By("VM Status.Volume should have entry for volume", func() {
				var vm *vmopv1.VirtualMachine
				Eventually(func(g Gomega) {
					vm = getVirtualMachine(vmKey)
					g.Expect(vm).ToNot(BeNil())
					g.Expect(vm.Status.Volumes).To(HaveLen(1))
				}).Should(Succeed())

				volStatus := vm.Status.Volumes[0]
				Expect(volStatus.Name).To(Equal(vmVolume1.Name))
				Expect(volStatus.Attached).To(BeFalse())
			})

			errMsg := "dummy error message"

			By("Simulate CNS attachment", func() {
				attachment := getCnsNodeVMAttachment(vm, vmVolume1)
				Expect(attachment).ToNot(BeNil())
				attachment.Status.Attached = true
				attachment.Status.Error = errMsg
				attachment.Status.AttachmentMetadata = map[string]string{
					volume.AttributeFirstClassDiskUUID: dummyDiskUUID1,
				}
				Expect(ctx.Client.Update(ctx, attachment)).To(Succeed())
			})

			By("VM Status.Volume should reflect attached volume", func() {
				var vm *vmopv1.VirtualMachine
				Eventually(func(g Gomega) {
					vm = getVirtualMachine(vmKey)
					g.Expect(vm).ToNot(BeNil())
					g.Expect(vm.Status.Volumes).To(HaveLen(1))
					g.Expect(vm.Status.Volumes[0].Attached).To(BeTrue())
				}).Should(Succeed())

				volStatus := vm.Status.Volumes[0]
				Expect(volStatus.Name).To(Equal(vmVolume1.Name))
				Expect(volStatus.DiskUUID).To(Equal(dummyDiskUUID1))
				Expect(volStatus.Error).To(Equal(errMsg))
			})
		})

		It("Reconciles VirtualMachine with multiple Spec.Volumes", func() {
			Expect(ctx.Client.Create(ctx, vm)).To(Succeed())

			By("Assign VM BiosUUID", func() {
				vm.Status.BiosUUID = dummyBiosUUID
				Expect(ctx.Client.Status().Update(ctx, vm)).To(Succeed())
			})

			By("Add CNS volume to Spec.Volumes", func() {
				By("Create PVCs and Bind", func() {
					Expect(ctx.Client.Create(ctx, pvc1)).To(Succeed())
					pvc1.Status.Phase = corev1.ClaimBound
					Expect(ctx.Client.Status().Update(ctx, pvc1)).To(Succeed())

					Expect(ctx.Client.Create(ctx, pvc2)).To(Succeed())
					pvc2.Status.Phase = corev1.ClaimBound
					Expect(ctx.Client.Status().Update(ctx, pvc2)).To(Succeed())
				})
				vm.Spec.Volumes = append(vm.Spec.Volumes, vmVolume1, vmVolume2)
				Expect(ctx.Client.Update(ctx, vm)).To(Succeed())
			})

			By("CnsNodeVmAttachment should be created for volume1", func() {
				var attachment *cnsv1alpha1.CnsNodeVmAttachment
				Eventually(func(g Gomega) {
					attachment = getCnsNodeVMAttachment(vm, vmVolume1)
					g.Expect(attachment).ToNot(BeNil())
				}).Should(Succeed())

				Expect(attachment.Spec.NodeUUID).To(Equal(dummyBiosUUID))
				Expect(attachment.Spec.VolumeName).To(Equal(vmVolume1.PersistentVolumeClaim.ClaimName))

				// Add our own finalizer so it hangs around after the delete below.
				attachment.Finalizers = append(attachment.Finalizers, "test-finalizer")
				Expect(ctx.Client.Update(ctx, attachment)).To(Succeed())
			})

			By("CnsNodeVmAttachment should not be created for volume2", func() {
				Consistently(func(g Gomega) {
					g.Expect(getCnsNodeVMAttachment(vm, vmVolume2)).To(BeNil())
				}, "3s").Should(Succeed())

				Eventually(func(g Gomega) {
					vm := getVirtualMachine(vmKey)
					g.Expect(vm).ToNot(BeNil())
					g.Expect(vm.Status.Volumes).To(HaveLen(1))
				}).Should(Succeed())
			})

			By("Simulate VM being powered on", func() {
				vm := getVirtualMachine(vmKey)
				Expect(vm).ToNot(BeNil())
				vm.Status.PowerState = vmopv1.VirtualMachinePowerStateOn
				Expect(ctx.Client.Status().Update(ctx, vm)).To(Succeed())
			})

			By("CnsNodeVmAttachment should be created for volume2", func() {
				var attachment *cnsv1alpha1.CnsNodeVmAttachment
				Eventually(func(g Gomega) {
					attachment = getCnsNodeVMAttachment(vm, vmVolume2)
					g.Expect(attachment).ToNot(BeNil())
				}).Should(Succeed())

				Expect(attachment.Spec.NodeUUID).To(Equal(dummyBiosUUID))
				Expect(attachment.Spec.VolumeName).To(Equal(vmVolume2.PersistentVolumeClaim.ClaimName))
			})

			By("VM Status.Volume should have entry for volume1 and volume2", func() {
				Eventually(func(g Gomega) {
					vm = getVirtualMachine(vmKey)
					g.Expect(vm).ToNot(BeNil())
					g.Expect(vm.Status.Volumes).To(HaveLen(2))
				}).Should(Succeed())

				volStatus := vm.Status.Volumes[0]
				Expect(volStatus.Name).To(Equal(vmVolume1.Name))
				Expect(volStatus.Attached).To(BeFalse())

				volStatus = vm.Status.Volumes[1]
				Expect(volStatus.Name).To(Equal(vmVolume2.Name))
				Expect(volStatus.Attached).To(BeFalse())
			})

			By("Simulate CNS attachment for volume1", func() {
				attachment := getCnsNodeVMAttachment(vm, vmVolume1)
				Expect(attachment).ToNot(BeNil())
				attachment.Status.Attached = true
				attachment.Status.AttachmentMetadata = map[string]string{
					volume.AttributeFirstClassDiskUUID: dummyDiskUUID1,
				}
				Expect(ctx.Client.Update(ctx, attachment)).To(Succeed())
			})

			By("Simulate CNS attachment for volume2", func() {
				attachment := getCnsNodeVMAttachment(vm, vmVolume2)
				Expect(attachment).ToNot(BeNil())
				attachment.Status.Attached = true
				attachment.Status.AttachmentMetadata = map[string]string{
					volume.AttributeFirstClassDiskUUID: dummyDiskUUID2,
				}
				Expect(ctx.Client.Update(ctx, attachment)).To(Succeed())
			})

			By("VM Status.Volume should reflect attached volumes", func() {
				Eventually(func(g Gomega) {
					vm = getVirtualMachine(vmKey)
					g.Expect(vm).ToNot(BeNil())
					g.Expect(vm.Status.Volumes).To(HaveLen(2))
					g.Expect(vm.Status.Volumes[0].Attached).To(BeTrue())
					g.Expect(vm.Status.Volumes[1].Attached).To(BeTrue())
				}).Should(Succeed(), "Waiting VM Status.Volumes to show attached")
			})

			By("Remove CNS volume1 from VM Spec.Volumes", func() {
				vm = getVirtualMachine(vmKey)
				Expect(vm).ToNot(BeNil())
				vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{vmVolume2}
				Expect(ctx.Client.Update(ctx, vm)).To(Succeed())
			})

			By("CnsNodeVmAttachment for volume1 should marked for deletion", func() {
				Eventually(func(g Gomega) {
					attachment := getCnsNodeVMAttachment(vm, vmVolume1)
					g.Expect(attachment).ToNot(BeNil())
					g.Expect(attachment.DeletionTimestamp.IsZero()).To(BeFalse())
				}).Should(Succeed())
			})

			By("VM Status.Volumes should still have entries for volume1 and volume2", func() {
				// I'm not sure if we have a better way to check for this.
				Consistently(func(g Gomega) {
					vm = getVirtualMachine(vmKey)
					g.Expect(vm).ToNot(BeNil())
					g.Expect(vm.Status.Volumes).To(HaveLen(2))
				}, "3s").Should(Succeed())
			})
		})
	})
}
