// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package volumebatch_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
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

		vm                *vmopv1.VirtualMachine
		vmKey             types.NamespacedName
		vmVolume1         vmopv1.VirtualMachineVolume
		vmVolume2         vmopv1.VirtualMachineVolume
		pvc1              *corev1.PersistentVolumeClaim
		pvc2              *corev1.PersistentVolumeClaim
		dummyBiosUUID     string
		dummyInstanceUUID string
		dummyDiskUUID1    string
		dummyDiskUUID2    string
	)

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()

		dummyBiosUUID = uuid.New().String()
		dummyInstanceUUID = uuid.New().String()
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
			ControllerType:      vmopv1.VirtualControllerTypeSCSI,
			ControllerBusNumber: ptr.To(int32(0)),
			UnitNumber:          ptr.To(int32(0)),
			DiskMode:            vmopv1.VolumeDiskModePersistent,
			SharingMode:         vmopv1.VolumeSharingModeNone,
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
			ControllerType:      vmopv1.VirtualControllerTypeSCSI,
			ControllerBusNumber: ptr.To(int32(0)),
			UnitNumber:          ptr.To(int32(1)),
			DiskMode:            vmopv1.VolumeDiskModePersistent,
			SharingMode:         vmopv1.VolumeSharingModeNone,
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
			Status: vmopv1.VirtualMachineStatus{
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
		vmKey = types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace}

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

	getCnsNodeVMBatchAttachment := func(vm *vmopv1.VirtualMachine) *cnsv1alpha1.CnsNodeVMBatchAttachment {
		objectKey := client.ObjectKey{Name: util.CNSBatchAttachmentNameForVM(vm.Name), Namespace: vm.Namespace}
		attachment := &cnsv1alpha1.CnsNodeVMBatchAttachment{}
		if err := ctx.Client.Get(ctx, objectKey, attachment); err == nil {
			return attachment
		}
		return nil
	}

	Context("Reconcile", func() {

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

				By("Assign VM BiosUUID and InstanceUUID", func() {
					vm.Status.BiosUUID = dummyBiosUUID
					vm.Status.InstanceUUID = dummyInstanceUUID
					Expect(ctx.Client.Status().Update(ctx, vm)).To(Succeed())
				})

				By("Add CNS volume to Spec.Volumes", func() {
					vm.Spec.Volumes = append(vm.Spec.Volumes, vmVolume1)
					Expect(ctx.Client.Update(ctx, vm)).To(Succeed())
				})

				By("CnsNodeVMBatchAttachment should not be created for volume", func() {
					Consistently(func(g Gomega) {
						g.Expect(getCnsNodeVMBatchAttachment(vm)).To(BeNil())
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

			By("Assign VM BiosUUID and InstanceUUID", func() {
				vm.Status.BiosUUID = dummyBiosUUID
				vm.Status.InstanceUUID = dummyInstanceUUID
				vm.Status.Hardware = &vmopv1.VirtualMachineHardwareStatus{
					Controllers: []vmopv1.VirtualControllerStatus{
						{
							Type:      "SCSI",
							BusNumber: 0,
							DeviceKey: 1000,
						},
					},
				}
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

			By("CnsNodeVMBatchAttachment should be created for volume", func() {
				var attachment *cnsv1alpha1.CnsNodeVMBatchAttachment
				Eventually(func(g Gomega) {
					attachment = getCnsNodeVMBatchAttachment(vm)
					g.Expect(attachment).ToNot(BeNil())
					g.Expect(attachment.Spec.InstanceUUID).To(Equal(dummyInstanceUUID))
					g.Expect(attachment.Spec.Volumes).To(HaveLen(1))
				}).Should(Succeed())
				Expect(attachment.Spec.Volumes[0].Name).To(Equal(vmVolume1.Name))
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

			By("Simulate CNS update batchAttachment with volume status", func() {
				attachment := getCnsNodeVMBatchAttachment(vm)
				Expect(attachment).ToNot(BeNil())
				attachment.Status.VolumeStatus = append(
					attachment.Status.VolumeStatus,
					cnsv1alpha1.VolumeStatus{
						Name: vmVolume1.Name,
						PersistentVolumeClaim: cnsv1alpha1.PersistentVolumeClaimStatus{
							Attached:  true,
							DiskUUID:  dummyDiskUUID1,
							ClaimName: vmVolume1.PersistentVolumeClaim.ClaimName,
							Error:     errMsg,
						},
					},
				)

				Expect(ctx.Client.Status().Update(ctx, attachment)).To(Succeed())
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

			By("Assign VM BiosUUID and InstanceUUID", func() {
				vm.Status.BiosUUID = dummyBiosUUID
				vm.Status.InstanceUUID = dummyInstanceUUID
				vm.Status.Hardware = &vmopv1.VirtualMachineHardwareStatus{
					Controllers: []vmopv1.VirtualControllerStatus{
						{
							Type:      "SCSI",
							BusNumber: 0,
							DeviceKey: 1000,
						},
					},
				}
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

			By("CnsNodeVMBatchAttachment should be created", func() {
				var attachment *cnsv1alpha1.CnsNodeVMBatchAttachment
				Eventually(func(g Gomega) {
					attachment = getCnsNodeVMBatchAttachment(vm)
					g.Expect(attachment).ToNot(BeNil())
					g.Expect(attachment.Spec.InstanceUUID).To(Equal(dummyInstanceUUID))
					g.Expect(attachment.Spec.Volumes).To(HaveLen(2))
				}).Should(Succeed())
				Expect(attachment.Spec.Volumes[0].Name).To(Equal(vmVolume1.Name))
				Expect(attachment.Spec.Volumes[1].Name).To(Equal(vmVolume2.Name))
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
				attachment := getCnsNodeVMBatchAttachment(vm)
				Expect(attachment).ToNot(BeNil())
				attachment.Status.VolumeStatus = append(
					attachment.Status.VolumeStatus,
					cnsv1alpha1.VolumeStatus{
						Name: vmVolume1.Name,
						PersistentVolumeClaim: cnsv1alpha1.PersistentVolumeClaimStatus{
							Attached:  true,
							DiskUUID:  dummyDiskUUID1,
							ClaimName: vmVolume1.PersistentVolumeClaim.ClaimName,
						},
					},
				)

				Expect(ctx.Client.Status().Update(ctx, attachment)).To(Succeed())
			})

			By("Simulate CNS attachment for volume2", func() {
				attachment := getCnsNodeVMBatchAttachment(vm)
				Expect(attachment).ToNot(BeNil())
				attachment.Status.VolumeStatus = append(
					attachment.Status.VolumeStatus,
					cnsv1alpha1.VolumeStatus{
						Name: vmVolume2.Name,
						PersistentVolumeClaim: cnsv1alpha1.PersistentVolumeClaimStatus{
							Attached:  true,
							DiskUUID:  dummyDiskUUID2,
							ClaimName: vmVolume2.PersistentVolumeClaim.ClaimName,
						},
					},
				)

				Expect(ctx.Client.Status().Update(ctx, attachment)).To(Succeed())
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

			By("VM Status.Volumes should still have entries for volume1 and volume2", func() {
				// I'm not sure if we have a better way to check for this.
				Eventually(func(g Gomega) {
					vm = getVirtualMachine(vmKey)
					g.Expect(vm).ToNot(BeNil())
					g.Expect(vm.Status.Volumes).To(HaveLen(2))
				}, "3s").Should(Succeed())
			})
		})
	})
}
