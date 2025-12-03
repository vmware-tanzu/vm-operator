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
	storagehelpers "k8s.io/component-helpers/storage/volume"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/pkg/patch"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
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
	Describe(
		"Instance Storage",
		Label(
			testlabels.Controller,
			testlabels.EnvTest,
		),
		intgTestsInstanceStorage,
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

func intgTestsInstanceStorage() {
	var (
		ctx *builder.IntegrationTestContext
	)

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()
		// Enable instance storage feature
		pkgcfg.SetContext(suite, func(config *pkgcfg.Config) {
			config.Features.InstanceStorage = true
		})
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	Context("Instance Storage PVC Creation", func() {
		It("should successfully create instance storage PVCs with correct labels", func() {
			// The volumebatch controller watches all PVCs

			// Create a VM with instance storage
			vm := &vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: ctx.Namespace,
					Name:      "test-vm-with-instance-storage",
					// Set the selected node annotation to trigger PVC creation
					Annotations: map[string]string{
						constants.InstanceStorageSelectedNodeAnnotationKey:     "test-node",
						constants.InstanceStorageSelectedNodeMOIDAnnotationKey: "node-123",
					},
				},
				Spec: vmopv1.VirtualMachineSpec{
					ClassName: "dummy-class",
					Volumes: []vmopv1.VirtualMachineVolume{
						{
							Name: "instance-storage-pvc",
							VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
								PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
									PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "instance-storage-pvc",
									},
									InstanceVolumeClaim: &vmopv1.InstanceVolumeClaimVolumeSource{
										StorageClass: "instance-storage-sc",
										Size:         resource.MustParse("10Gi"),
									},
								},
							},
						},
					},
				},
			}

			// Create the VM - the controller should create the PVC
			Expect(ctx.Client.Create(ctx, vm)).To(Succeed())

			// Verify the controller creates the PVC with the instance storage label
			Eventually(func(g Gomega) {
				pvc := &corev1.PersistentVolumeClaim{}
				err := ctx.Client.Get(ctx, client.ObjectKey{
					Name:      "instance-storage-pvc",
					Namespace: ctx.Namespace,
				}, pvc)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(pvc.Labels[constants.InstanceStorageLabelKey]).To(Equal("true"))
				g.Expect(pvc.OwnerReferences).ToNot(BeEmpty())
				g.Expect(pvc.OwnerReferences[0].Name).To(Equal(vm.Name))
			}).Should(Succeed())
		})
	})

	Context("Reconcile Instance Storage", func() {
		var (
			vm                    *vmopv1.VirtualMachine
			vmKey                 types.NamespacedName
			dummySelectedNode     string
			dummySelectedNodeMOID string
		)

		BeforeEach(func() {
			pkgcfg.SetContext(suite, func(config *pkgcfg.Config) {
				config.Features.InstanceStorage = true
				config.InstanceStorage.PVPlacementFailedTTL = 0
			})

			dummySelectedNode = "test-node-1"
			dummySelectedNodeMOID = "node-moid-123"
		})

		JustBeforeEach(func() {
			vm = &vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: ctx.Namespace,
					Name:      "test-vm-instance-storage",
					Labels:    map[string]string{constants.InstanceStorageLabelKey: "true"},
				},
				Spec: vmopv1.VirtualMachineSpec{
					ClassName: "dummy-class",
					Volumes: []vmopv1.VirtualMachineVolume{
						{
							Name: "is-pvc-1",
							VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
								PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
									PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "is-pvc-1",
									},
									InstanceVolumeClaim: &vmopv1.InstanceVolumeClaimVolumeSource{
										StorageClass: "instance-storage-sc",
										Size:         resource.MustParse("10Gi"),
									},
								},
							},
						},
						{
							Name: "is-pvc-2",
							VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
								PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
									PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "is-pvc-2",
									},
									InstanceVolumeClaim: &vmopv1.InstanceVolumeClaimVolumeSource{
										StorageClass: "instance-storage-sc",
										Size:         resource.MustParse("20Gi"),
									},
								},
							},
						},
					},
				},
			}
			vmKey = client.ObjectKeyFromObject(vm)
			Expect(ctx.Client.Create(ctx, vm)).To(Succeed())

			// Need to set annotations after create due to controller-runtime behavior
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
			// Without selected-node annotation, PVCs should not be created
			Eventually(func(g Gomega) {
				g.Expect(ctx.Client.Get(ctx, vmKey, vm)).To(Succeed())
				g.Expect(vm.Annotations).ToNot(HaveKey(constants.InstanceStoragePVCsBoundAnnotationKey))
			}).Should(Succeed())

			// PVCs should not exist
			Consistently(func(g Gomega) {
				pvc1 := &corev1.PersistentVolumeClaim{}
				err := ctx.Client.Get(ctx, client.ObjectKey{
					Name:      "is-pvc-1",
					Namespace: ctx.Namespace,
				}, pvc1)
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}).Should(Succeed())
		})

		It("Reconcile instance storage PVCs - PVCs should be created and realized after setting selected-node annotation", func() {
			By("set selected-node annotation", func() {
				Expect(ctx.Client.Get(ctx, vmKey, vm)).To(Succeed())
				if vm.Annotations == nil {
					vm.Annotations = make(map[string]string)
				}
				vm.Annotations[constants.InstanceStorageSelectedNodeAnnotationKey] = dummySelectedNode
				vm.Annotations[constants.InstanceStorageSelectedNodeMOIDAnnotationKey] = dummySelectedNodeMOID
				Expect(ctx.Client.Update(ctx, vm)).To(Succeed())
			})

			By("PVCs should be created", func() {
				Eventually(func(g Gomega) {
					for _, volName := range []string{"is-pvc-1", "is-pvc-2"} {
						pvc := &corev1.PersistentVolumeClaim{}
						g.Expect(ctx.Client.Get(ctx, client.ObjectKey{
							Name:      volName,
							Namespace: ctx.Namespace,
						}, pvc)).To(Succeed())
						g.Expect(pvc.Labels).To(HaveKeyWithValue(constants.InstanceStorageLabelKey, "true"))
						g.Expect(pvc.Annotations).To(HaveKeyWithValue(storagehelpers.AnnSelectedNode, dummySelectedNode))
					}
				}).Should(Succeed(), "waiting for instance storage PVCs to be created")
			})

			By("realize PVCs by setting PVC status to BOUND", func() {
				Eventually(func(g Gomega) {
					for _, volName := range []string{"is-pvc-1", "is-pvc-2"} {
						pvc := &corev1.PersistentVolumeClaim{}
						g.Expect(ctx.Client.Get(ctx, client.ObjectKey{
							Name:      volName,
							Namespace: ctx.Namespace,
						}, pvc)).To(Succeed())
						pvc.Status.Phase = corev1.ClaimBound
						g.Expect(ctx.Client.Status().Update(ctx, pvc)).To(Succeed())
					}
				}).Should(Succeed())
			})

			By("PVCs should be bound", func() {
				Eventually(func(g Gomega) {
					g.Expect(ctx.Client.Get(ctx, vmKey, vm)).To(Succeed())
					g.Expect(vm.Annotations).To(HaveKey(constants.InstanceStoragePVCsBoundAnnotationKey))
				}).Should(Succeed())
			})
		})

		It("Reconcile instance storage PVCs - PVCs should be deleted after PVCs turned into error", func() {
			By("set selected-node annotation", func() {
				Expect(ctx.Client.Get(ctx, vmKey, vm)).To(Succeed())
				if vm.Annotations == nil {
					vm.Annotations = make(map[string]string)
				}
				vm.Annotations[constants.InstanceStorageSelectedNodeAnnotationKey] = dummySelectedNode
				vm.Annotations[constants.InstanceStorageSelectedNodeMOIDAnnotationKey] = dummySelectedNodeMOID
				Expect(ctx.Client.Update(ctx, vm)).To(Succeed())
			})

			By("PVCs should be created", func() {
				Eventually(func(g Gomega) {
					for _, volName := range []string{"is-pvc-1", "is-pvc-2"} {
						pvc := &corev1.PersistentVolumeClaim{}
						g.Expect(ctx.Client.Get(ctx, client.ObjectKey{
							Name:      volName,
							Namespace: ctx.Namespace,
						}, pvc)).To(Succeed())
						g.Expect(pvc.Labels).To(HaveKeyWithValue(constants.InstanceStorageLabelKey, "true"))
						g.Expect(pvc.Annotations).To(HaveKeyWithValue(storagehelpers.AnnSelectedNode, dummySelectedNode))
					}
				}).Should(Succeed(), "waiting for instance storage PVCs to be created")
			})

			By("Set PVCs with errors", func() {
				for _, volName := range []string{"is-pvc-1", "is-pvc-2"} {
					pvc := &corev1.PersistentVolumeClaim{}
					Expect(ctx.Client.Get(ctx, client.ObjectKey{
						Name:      volName,
						Namespace: ctx.Namespace,
					}, pvc)).To(Succeed())

					patchHelper, err := patch.NewHelper(pvc, ctx.Client)
					Expect(err).ToNot(HaveOccurred())

					if pvc.Annotations == nil {
						pvc.Annotations = make(map[string]string)
					}
					pvc.Annotations[constants.InstanceStoragePVPlacementErrorAnnotationKey] = constants.InstanceStorageNotEnoughResErr
					Expect(patchHelper.Patch(ctx, pvc)).To(Succeed())
				}

				// Wait for cache to sync by verifying we can read the updated PVCs with error annotations.
				// This ensures the controller's cache has been updated before we expect it to act on the changes.
				Eventually(func(g Gomega) {
					for _, volName := range []string{"is-pvc-1", "is-pvc-2"} {
						pvc := &corev1.PersistentVolumeClaim{}
						g.Expect(ctx.Client.Get(ctx, client.ObjectKey{
							Name:      volName,
							Namespace: ctx.Namespace,
						}, pvc)).To(Succeed())
						g.Expect(pvc.Annotations).To(HaveKeyWithValue(
							constants.InstanceStoragePVPlacementErrorAnnotationKey,
							constants.InstanceStorageNotEnoughResErr,
						))
					}
				}).Should(Succeed(), "waiting for PVC error annotations to be visible in cache")
			})

			By("PVCs should be deleted", func() {
				Eventually(func(g Gomega) {
					for _, volName := range []string{"is-pvc-1", "is-pvc-2"} {
						pvc := &corev1.PersistentVolumeClaim{}
						err := ctx.Client.Get(ctx, client.ObjectKey{
							Name:      volName,
							Namespace: ctx.Namespace,
						}, pvc)
						if err == nil {
							g.Expect(pvc.DeletionTimestamp.IsZero()).To(BeFalse())
						} else {
							g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
						}
					}
				}, "30s").Should(Succeed(), "waiting for instance storage PVCs to be deleted")
			})

			By("selected-node annotations should be removed", func() {
				Eventually(func(g Gomega) {
					g.Expect(ctx.Client.Get(ctx, vmKey, vm)).To(Succeed())
					g.Expect(vm.Annotations).ToNot(HaveKey(constants.InstanceStorageSelectedNodeAnnotationKey))
					g.Expect(vm.Annotations).ToNot(HaveKey(constants.InstanceStorageSelectedNodeMOIDAnnotationKey))
				}, "30s").Should(Succeed())
			})
		})

		When("VM has non-empty spec.crypto.encryptionClassName", func() {
			var encryptedVM *vmopv1.VirtualMachine

			BeforeEach(func() {
				encryptedVM = &vmopv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ctx.Namespace,
						Name:      "test-vm-encrypted-instance-storage",
						Labels:    map[string]string{constants.InstanceStorageLabelKey: "true"},
						Annotations: map[string]string{
							constants.InstanceStorageSelectedNodeAnnotationKey:     dummySelectedNode,
							constants.InstanceStorageSelectedNodeMOIDAnnotationKey: dummySelectedNodeMOID,
						},
					},
					Spec: vmopv1.VirtualMachineSpec{
						ClassName: "dummy-class",
						Crypto: &vmopv1.VirtualMachineCryptoSpec{
							EncryptionClassName: "my-encryption-class",
						},
						Volumes: []vmopv1.VirtualMachineVolume{
							{
								Name: "encrypted-is-pvc",
								VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
									PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
										PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
											ClaimName: "encrypted-is-pvc",
										},
										InstanceVolumeClaim: &vmopv1.InstanceVolumeClaimVolumeSource{
											StorageClass: "instance-storage-sc",
											Size:         resource.MustParse("15Gi"),
										},
									},
								},
							},
						},
					},
				}
				Expect(ctx.Client.Create(ctx, encryptedVM)).To(Succeed())
			})

			AfterEach(func() {
				err := ctx.Client.Delete(ctx, encryptedVM)
				Expect(err == nil || apierrors.IsNotFound(err)).To(BeTrue())
			})

			It("PVCs are created with the expected encryption annotation", func() {
				Eventually(func(g Gomega) {
					pvc := &corev1.PersistentVolumeClaim{}
					g.Expect(ctx.Client.Get(ctx, client.ObjectKey{
						Name:      "encrypted-is-pvc",
						Namespace: ctx.Namespace,
					}, pvc)).To(Succeed())
					g.Expect(pvc.Annotations).To(HaveKeyWithValue(
						constants.EncryptionClassNameAnnotation,
						encryptedVM.Spec.Crypto.EncryptionClassName))
				}).Should(Succeed())
			})
		})
	})
}
