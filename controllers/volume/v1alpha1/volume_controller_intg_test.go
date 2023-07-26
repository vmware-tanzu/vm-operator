// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1_test

import (
	"fmt"
	"os"
	"sync/atomic"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/google/uuid"

	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/pkg/syncer/cnsoperator/apis/cnsnodevmattachment/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/controllers/volume/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/patch"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/instancestorage"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {
	var (
		ctx *builder.IntegrationTestContext

		vm                    *vmopv1.VirtualMachine
		vmKey                 types.NamespacedName
		vmVolume1             vmopv1.VirtualMachineVolume
		vmVolume2             vmopv1.VirtualMachineVolume
		dummyBiosUUID         string
		dummyDiskUUID1        string
		dummyDiskUUID2        string
		dummySelectedNode     string
		dummySelectedNodeMOID string

		// represents the Instance Storage FSS. This should be manipulated atomically to avoid races
		// where the controller is trying to read this _while_ the tests are updating it.
		instanceStorageFSS uint32
	)

	// Modify the helper function to return the custom value of the FSS
	lib.IsInstanceStorageFSSEnabled = func() bool {
		return atomic.LoadUint32(&instanceStorageFSS) != 0
	}

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()

		dummyBiosUUID = uuid.New().String()
		dummyDiskUUID1 = uuid.New().String()
		dummyDiskUUID2 = uuid.New().String()

		vmVolume1 = vmopv1.VirtualMachineVolume{
			Name: "cns-volume-1",
			PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
				PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: "pvc-volume-1",
				},
			},
		}

		vmVolume2 = vmopv1.VirtualMachineVolume{
			Name: "cns-volume-2",
			PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
				PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: "pvc-volume-2",
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
				PowerState: vmopv1.VirtualMachinePoweredOff,
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
		objectKey := client.ObjectKey{Name: v1alpha1.CNSAttachmentNameForVolume(vm, vmVol.Name), Namespace: vm.Namespace}
		attachment := &cnsv1alpha1.CnsNodeVmAttachment{}
		if err := ctx.Client.Get(ctx, objectKey, attachment); err == nil {
			return attachment
		}
		return nil
	}

	waitForVirtualMachineInstanceStorage := func(objKey types.NamespacedName, selectedNodeSet, bound bool) {
		getAnnotations := func() map[string]string {
			vm := getVirtualMachine(objKey)
			if vm != nil {
				return vm.Annotations
			}
			return map[string]string{}
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

		volumes := instancestorage.FilterVolumes(vm)
		Expect(volumes).ToNot(BeEmpty())

		Eventually(func() bool {
			for _, vol := range volumes {
				pvcKey := client.ObjectKey{
					Namespace: vm.Namespace,
					Name:      vol.PersistentVolumeClaim.ClaimName,
				}

				pvc := &corev1.PersistentVolumeClaim{}
				if ctx.Client.Get(ctx, pvcKey, pvc) != nil {
					return false
				}

				isClaim := vol.PersistentVolumeClaim.InstanceVolumeClaim
				Expect(pvc.Labels).To(HaveKey(constants.InstanceStorageLabelKey))
				Expect(pvc.Annotations).To(HaveKeyWithValue(constants.KubernetesSelectedNodeAnnotationKey, dummySelectedNode))
				Expect(pvc.Spec.StorageClassName).ToNot(BeNil())
				Expect(*pvc.Spec.StorageClassName).To(Equal(isClaim.StorageClass))
				Expect(pvc.Spec.Resources.Requests).To(HaveKeyWithValue(corev1.ResourceStorage, isClaim.Size))
				Expect(pvc.Spec.AccessModes).To(ConsistOf(corev1.ReadWriteOnce))
			}
			return true
		}).Should(BeTrue(), "waiting for instance storage PVCs to be created")
	}

	waitForInstanceStoragePVCsToBeDeleted := func(objKey types.NamespacedName) {
		vm := getVirtualMachine(objKey)
		Expect(vm).ToNot(BeNil())

		volumes := instancestorage.FilterVolumes(vm)
		Expect(volumes).ToNot(BeEmpty())

		Eventually(func() bool {
			for _, vol := range volumes {
				pvcKey := client.ObjectKey{
					Namespace: vm.Namespace,
					Name:      vol.PersistentVolumeClaim.ClaimName,
				}

				// PVCs gets finalizer set on creation so expect the deletion timestamp to be set.
				pvc := &corev1.PersistentVolumeClaim{}
				if err := ctx.Client.Get(ctx, pvcKey, pvc); err != nil || pvc.DeletionTimestamp.IsZero() {
					return false
				}
			}
			return true
		}).Should(BeTrue(), "waiting for instance storage PVCs to be deleted")
	}

	patchInstanceStoragePVCs := func(objKey types.NamespacedName, setStatusBound, setErrorAnnotation bool) {
		vm := getVirtualMachine(objKey)
		Expect(vm).ToNot(BeNil())

		volumes := instancestorage.FilterVolumes(vm)
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
		var (
			origInstanceStorageFSSState  uint32
			origInstanceStorageFailedTTL string
		)

		BeforeEach(func() {
			origInstanceStorageFSSState = instanceStorageFSS
			atomic.StoreUint32(&instanceStorageFSS, 1)
			origInstanceStorageFailedTTL = os.Getenv(lib.InstanceStoragePVPlacementFailedTTLEnv)
			Expect(os.Setenv(lib.InstanceStoragePVPlacementFailedTTLEnv, "0s")).To(Succeed())

			vm.Spec.Volumes = append(vm.Spec.Volumes, builder.DummyInstanceStorageVirtualMachineVolumes()...)
			vm.Labels = map[string]string{constants.InstanceStorageLabelKey: lib.TrueString}
			Expect(ctx.Client.Create(ctx, vm)).To(Succeed())

			// This is line is due to a change in controller-runtime's change to
			// Create/Update calls that nils out empty fields. If this was set
			// prior to the above Create call, then vm.Metadata.Annotations
			// would be reset to nil.
			vm.Annotations = map[string]string{}
		})

		AfterEach(func() {
			err := ctx.Client.Delete(ctx, vm)
			Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
			atomic.StoreUint32(&instanceStorageFSS, origInstanceStorageFSSState)
			Expect(os.Setenv(lib.InstanceStoragePVPlacementFailedTTLEnv, origInstanceStorageFailedTTL)).To(Succeed())
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
	})

	Context("Reconcile", func() {
		BeforeEach(func() {
		})

		AfterEach(func() {
			err := ctx.Client.Delete(ctx, vm)
			Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
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
				vm.Spec.Volumes = append(vm.Spec.Volumes, vmVolume1)
				Expect(ctx.Client.Update(ctx, vm)).To(Succeed())
			})

			By("CnsNodeVmAttachment should be created for volume", func() {
				var attachment *cnsv1alpha1.CnsNodeVmAttachment
				Eventually(func() *cnsv1alpha1.CnsNodeVmAttachment {
					attachment = getCnsNodeVMAttachment(vm, vmVolume1)
					return attachment
				}).ShouldNot(BeNil())

				Expect(attachment.Spec.NodeUUID).To(Equal(dummyBiosUUID))
				Expect(attachment.Spec.VolumeName).To(Equal(vmVolume1.PersistentVolumeClaim.ClaimName))
			})

			By("VM Status.Volume should have entry for volume", func() {
				var vm *vmopv1.VirtualMachine
				Eventually(func() []vmopv1.VirtualMachineVolumeStatus {
					vm = getVirtualMachine(vmKey)
					if vm != nil {
						return vm.Status.Volumes
					}
					return nil
				}).Should(HaveLen(1))

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
					v1alpha1.AttributeFirstClassDiskUUID: dummyDiskUUID1,
				}
				Expect(ctx.Client.Update(ctx, attachment)).To(Succeed())
			})

			By("VM Status.Volume should reflect attached volume", func() {
				var vm *vmopv1.VirtualMachine
				Eventually(func() bool {
					vm = getVirtualMachine(vmKey)
					if vm == nil || len(vm.Status.Volumes) != 1 {
						return false
					}
					return vm.Status.Volumes[0].Attached
				}).Should(BeTrue())

				volStatus := vm.Status.Volumes[0]
				Expect(volStatus.Name).To(Equal(vmVolume1.Name))
				Expect(volStatus.DiskUuid).To(Equal(dummyDiskUUID1))
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
				vm.Spec.Volumes = append(vm.Spec.Volumes, vmVolume1, vmVolume2)
				Expect(ctx.Client.Update(ctx, vm)).To(Succeed())
			})

			By("CnsNodeVmAttachment should be created for volume1", func() {
				var attachment *cnsv1alpha1.CnsNodeVmAttachment
				Eventually(func() *cnsv1alpha1.CnsNodeVmAttachment {
					attachment = getCnsNodeVMAttachment(vm, vmVolume1)
					return attachment
				}).ShouldNot(BeNil())

				Expect(attachment.Spec.NodeUUID).To(Equal(dummyBiosUUID))
				Expect(attachment.Spec.VolumeName).To(Equal(vmVolume1.PersistentVolumeClaim.ClaimName))

				// Add our own finalizer so it hangs around after the delete below.
				attachment.Finalizers = append(attachment.Finalizers, "test-finalizer")
				Expect(ctx.Client.Update(ctx, attachment)).To(Succeed())
			})

			By("CnsNodeVmAttachment should not be created for volume2", func() {
				Consistently(func() *cnsv1alpha1.CnsNodeVmAttachment {
					return getCnsNodeVMAttachment(vm, vmVolume2)
				}, "3s").Should(BeNil())

				Eventually(func() []vmopv1.VirtualMachineVolumeStatus {
					if vm := getVirtualMachine(vmKey); vm != nil {
						return vm.Status.Volumes
					}
					return nil
				}).Should(HaveLen(1))
			})

			By("Simulate VM being powered on", func() {
				vm := getVirtualMachine(vmKey)
				Expect(vm).ToNot(BeNil())
				vm.Status.PowerState = vmopv1.VirtualMachinePoweredOn
				Expect(ctx.Client.Status().Update(ctx, vm)).To(Succeed())
			})

			By("CnsNodeVmAttachment should be created for volume2", func() {
				var attachment *cnsv1alpha1.CnsNodeVmAttachment
				Eventually(func() *cnsv1alpha1.CnsNodeVmAttachment {
					attachment = getCnsNodeVMAttachment(vm, vmVolume2)
					return attachment
				}).ShouldNot(BeNil())

				Expect(attachment.Spec.NodeUUID).To(Equal(dummyBiosUUID))
				Expect(attachment.Spec.VolumeName).To(Equal(vmVolume2.PersistentVolumeClaim.ClaimName))
			})

			By("VM Status.Volume should have entry for volume1 and volume2", func() {
				Eventually(func() []vmopv1.VirtualMachineVolumeStatus {
					vm = getVirtualMachine(vmKey)
					if vm != nil {
						return vm.Status.Volumes
					}
					return nil
				}).Should(HaveLen(2))

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
					v1alpha1.AttributeFirstClassDiskUUID: dummyDiskUUID1,
				}
				Expect(ctx.Client.Update(ctx, attachment)).To(Succeed())
			})

			By("Simulate CNS attachment for volume2", func() {
				attachment := getCnsNodeVMAttachment(vm, vmVolume2)
				Expect(attachment).ToNot(BeNil())
				attachment.Status.Attached = true
				attachment.Status.AttachmentMetadata = map[string]string{
					v1alpha1.AttributeFirstClassDiskUUID: dummyDiskUUID2,
				}
				Expect(ctx.Client.Update(ctx, attachment)).To(Succeed())
			})

			By("VM Status.Volume should reflect attached volumes", func() {
				Eventually(func() bool {
					vm = getVirtualMachine(vmKey)
					if vm == nil || len(vm.Status.Volumes) != 2 {
						return false
					}
					return vm.Status.Volumes[0].Attached && vm.Status.Volumes[1].Attached
				}).Should(BeTrue(), "Waiting VM Status.Volumes to show attached")
			})

			By("Remove CNS volume1 from VM Spec.Volumes", func() {
				vm = getVirtualMachine(vmKey)
				Expect(vm).ToNot(BeNil())
				vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{vmVolume2}
				Expect(ctx.Client.Update(ctx, vm)).To(Succeed())
			})

			By("CnsNodeVmAttachment for volume1 should marked for deletion", func() {
				Eventually(func() bool {
					attachment := getCnsNodeVMAttachment(vm, vmVolume1)
					if attachment != nil {
						return !attachment.DeletionTimestamp.IsZero()
					}
					return false
				}).Should(BeTrue())
			})

			By("VM Status.Volumes should still have entries for volume1 and volume2", func() {
				// I'm not sure if we have a better way to check for this.
				Consistently(func() []vmopv1.VirtualMachineVolumeStatus {
					vm = getVirtualMachine(vmKey)
					Expect(vm).ToNot(BeNil())
					return vm.Status.Volumes
				}, "3s").Should(HaveLen(2))
			})
		})
	})
}
