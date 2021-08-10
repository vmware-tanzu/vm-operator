// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package volume_test

import (
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/controllers/volume"
	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/pkg/syncer/cnsoperator/apis/cnsnodevmattachment/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {
	var (
		ctx *builder.IntegrationTestContext

		vm             *vmopv1alpha1.VirtualMachine
		vmKey          types.NamespacedName
		vmVolume1      vmopv1alpha1.VirtualMachineVolume
		vmVolume2      vmopv1alpha1.VirtualMachineVolume
		dummyBiosUUID  string
		dummyDiskUUID1 string
		dummyDiskUUID2 string
	)

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()

		dummyBiosUUID = uuid.New().String()
		dummyDiskUUID1 = uuid.New().String()
		dummyDiskUUID2 = uuid.New().String()

		vmVolume1 = vmopv1alpha1.VirtualMachineVolume{
			Name: "cns-volume-1",
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: "pvc-volume-1",
			},
		}

		vmVolume2 = vmopv1alpha1.VirtualMachineVolume{
			Name: "cns-volume-2",
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: "pvc-volume-2",
			},
		}

		vm = &vmopv1alpha1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ctx.Namespace,
				Name:      "dummy-vm",
			},
			Spec: vmopv1alpha1.VirtualMachineSpec{
				ImageName:  "dummy-image",
				PowerState: vmopv1alpha1.VirtualMachinePoweredOn,
			},
		}
		vmKey = types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	getVirtualMachine := func(objKey types.NamespacedName) *vmopv1alpha1.VirtualMachine {
		vm := &vmopv1alpha1.VirtualMachine{}
		if err := ctx.Client.Get(ctx, objKey, vm); err != nil {
			return nil
		}
		return vm
	}

	getCnsNodeVMAttachment := func(vm *vmopv1alpha1.VirtualMachine, vmVol vmopv1alpha1.VirtualMachineVolume) *cnsv1alpha1.CnsNodeVmAttachment {
		objectKey := client.ObjectKey{Name: volume.CNSAttachmentNameForVolume(vm, vmVol.Name), Namespace: vm.Namespace}
		attachment := &cnsv1alpha1.CnsNodeVmAttachment{}
		if err := ctx.Client.Get(ctx, objectKey, attachment); err == nil {
			return attachment
		}
		return nil
	}

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
				var vm *vmopv1alpha1.VirtualMachine
				Eventually(func() []vmopv1alpha1.VirtualMachineVolumeStatus {
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
					volume.AttributeFirstClassDiskUUID: dummyDiskUUID1,
				}
				Expect(ctx.Client.Status().Update(ctx, attachment)).To(Succeed())
			})

			By("VM Status.Volume should reflect attached volume", func() {
				var vm *vmopv1alpha1.VirtualMachine
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

				// Add our own finalizer on so it hangs around after the delete below.
				attachment.Finalizers = append(attachment.Finalizers, "test-finalizer")
				Expect(ctx.Client.Update(ctx, attachment)).To(Succeed())
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
				Eventually(func() []vmopv1alpha1.VirtualMachineVolumeStatus {
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
					volume.AttributeFirstClassDiskUUID: dummyDiskUUID1,
				}
				Expect(ctx.Client.Status().Update(ctx, attachment)).To(Succeed())
			})

			By("Simulate CNS attachment for volume2", func() {
				attachment := getCnsNodeVMAttachment(vm, vmVolume2)
				Expect(attachment).ToNot(BeNil())
				attachment.Status.Attached = true
				attachment.Status.AttachmentMetadata = map[string]string{
					volume.AttributeFirstClassDiskUUID: dummyDiskUUID2,
				}
				Expect(ctx.Client.Status().Update(ctx, attachment)).To(Succeed())
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
				vm.Spec.Volumes = []vmopv1alpha1.VirtualMachineVolume{vmVolume2}
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
				Consistently(func() int {
					vm = getVirtualMachine(vmKey)
					Expect(vm).ToNot(BeNil())
					return len(vm.Status.Volumes)
				}).Should(Equal(2))
			})
		})
	})
}
