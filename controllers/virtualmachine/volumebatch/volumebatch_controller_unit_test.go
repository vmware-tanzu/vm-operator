// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package volumebatch_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/log"

	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/test/builder"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachine/volumebatch"
	volumebatchutils "github.com/vmware-tanzu/vm-operator/controllers/virtualmachine/volumebatch/utils"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
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
	const ns = "dummy-ns"
	var (
		reconciler  *volumebatch.Reconciler
		initObjects []client.Object
		withFuncs   interceptor.Funcs
		ctx         *builder.UnitTestContextForController
	)

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
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		initObjects = nil
		withFuncs = interceptor.Funcs{}
		reconciler = nil
	})

	Context("BuildVolumeSpecs", func() {
		var (
			vm     *vmopv1.VirtualMachine
			volCtx *pkgctx.VolumeContext
		)

		BeforeEach(func() {
			vm = &vmopv1.VirtualMachine{}
			volCtx = &pkgctx.VolumeContext{
				Context: ctx,
				Logger:  log.Log,
				VM:      vm,
			}
		})

		AfterEach(func() {
			vm = nil
			volCtx = nil
		})

		It("should handle basic PVC volume", func() {
			volumes := []vmopv1.VirtualMachineVolume{
				{
					Name: "test-volume",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
							PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "test-pvc",
							},
						},
					},
				},
			}

			specs, err := reconciler.BuildVolumeSpecs(volCtx, volumes)
			Expect(err).ToNot(HaveOccurred())
			Expect(specs).To(HaveLen(1))
			Expect(specs[0].Name).To(Equal("test-volume"))
			Expect(specs[0].PersistentVolumeClaim.ClaimName).To(Equal("test-pvc"))
		})

		It("should handle OracleRAC application type", func() {
			volumes := []vmopv1.VirtualMachineVolume{
				{
					Name: "oracle-volume",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
							PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "oracle-pvc",
							},
							ApplicationType: vmopv1.VolumeApplicationTypeOracleRAC,
						},
					},
				},
			}

			specs, err := reconciler.BuildVolumeSpecs(volCtx, volumes)
			Expect(err).ToNot(HaveOccurred())
			Expect(specs).To(HaveLen(1))
			Expect(specs[0].PersistentVolumeClaim.DiskMode).To(Equal(cnsv1alpha1.IndependentPersistent))
			Expect(specs[0].PersistentVolumeClaim.SharingMode).To(Equal(cnsv1alpha1.SharingMultiWriter))
		})

		It("should handle MicrosoftWSFC application type", func() {
			volumes := []vmopv1.VirtualMachineVolume{
				{
					Name: "wsfc-volume",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
							PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "wsfc-pvc",
							},
							ApplicationType: vmopv1.VolumeApplicationTypeMicrosoftWSFC,
						},
					},
				},
			}

			specs, err := reconciler.BuildVolumeSpecs(volCtx, volumes)
			Expect(err).ToNot(HaveOccurred())
			Expect(specs).To(HaveLen(1))
			Expect(specs[0].PersistentVolumeClaim.DiskMode).To(Equal(cnsv1alpha1.IndependentPersistent))
		})

		It("should handle controller type and bus number", func() {
			volumes := []vmopv1.VirtualMachineVolume{
				{
					Name: "scsi-volume",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
							PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "scsi-pvc",
							},
							ControllerType:      vmopv1.VirtualControllerTypeSCSI,
							ControllerBusNumber: ptr.To[int32](1),
							UnitNumber:          ptr.To[int32](5),
						},
					},
				},
			}

			specs, err := reconciler.BuildVolumeSpecs(volCtx, volumes)
			Expect(err).ToNot(HaveOccurred())
			Expect(specs).To(HaveLen(1))
			Expect(specs[0].PersistentVolumeClaim.ControllerKey).To(Equal("SCSI:1"))
			Expect(specs[0].PersistentVolumeClaim.UnitNumber).To(Equal("5"))
		})

		It("should handle disk mode and sharing mode overrides", func() {
			volumes := []vmopv1.VirtualMachineVolume{
				{
					Name: "override-volume",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
							PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "override-pvc",
							},
							ApplicationType: vmopv1.VolumeApplicationTypeOracleRAC, // This sets IndependentPersistent + MultiWriter
							DiskMode:        vmopv1.VolumeDiskModePersistent,       // Override to Persistent
							SharingMode:     vmopv1.VolumeSharingModeNone,          // Override to None
						},
					},
				},
			}

			specs, err := reconciler.BuildVolumeSpecs(volCtx, volumes)
			Expect(err).ToNot(HaveOccurred())
			Expect(specs).To(HaveLen(1))
			// Explicit settings should override application type presets
			Expect(specs[0].PersistentVolumeClaim.DiskMode).To(Equal(cnsv1alpha1.Persistent))
			Expect(specs[0].PersistentVolumeClaim.SharingMode).To(Equal(cnsv1alpha1.SharingNone))
		})
	})

	Context("BuildControllerKey", func() {
		It("should build controller key with type and bus number", func() {
			key := reconciler.BuildControllerKey(vmopv1.VirtualControllerTypeSCSI, ptr.To[int32](2))
			Expect(key).To(Equal("SCSI:2"))
		})

		It("should build controller key with type only", func() {
			key := reconciler.BuildControllerKey(vmopv1.VirtualControllerTypeNVME, nil)
			Expect(key).To(Equal("NVME"))
		})

		It("should build controller key with bus number only", func() {
			key := reconciler.BuildControllerKey("", ptr.To[int32](3))
			Expect(key).To(Equal(":3"))
		})

		It("should return empty string when neither specified", func() {
			key := reconciler.BuildControllerKey("", nil)
			Expect(key).To(Equal(""))
		})
	})

	Context("GetBatchAttachmentForVM", func() {
		var (
			vm     *vmopv1.VirtualMachine
			volCtx *pkgctx.VolumeContext
		)

		BeforeEach(func() {

			vm = &vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm",
					Namespace: ns,
					UID:       types.UID("test-vm-uid"),
				},
			}

			volCtx = &pkgctx.VolumeContext{
				Context: ctx,
				Logger:  log.Log,
				VM:      vm,
			}
		})

		AfterEach(func() {
			vm = nil
			volCtx = nil
		})

		It("should return nil when attachment does not exist", func() {
			attachment, err := reconciler.GetBatchAttachmentForVM(volCtx)
			Expect(err).ToNot(HaveOccurred())
			Expect(attachment).To(BeNil())
		})

		When("attachment exists and is owned by VM", func() {
			BeforeEach(func() {
				attachment := &cnsv1alpha1.CnsNodeVmBatchAttachment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-vm",
						Namespace: ns,
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         vmopv1.GroupVersion.String(),
								Kind:               "VirtualMachine",
								Name:               vm.Name,
								UID:                vm.UID,
								Controller:         ptr.To(true),
								BlockOwnerDeletion: ptr.To(true),
							},
						},
					},
				}
				initObjects = append(initObjects, attachment)
			})

			It("should return attachment when it exists and is owned by VM", func() {
				result, err := reconciler.GetBatchAttachmentForVM(volCtx)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
				Expect(result.Name).To(Equal("test-vm"))
			})
		})

		When("attachment exists but is not owned by VM", func() {
			BeforeEach(func() {
				otherVM := &vmopv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "other-vm",
						Namespace: ns,
						UID:       types.UID("other-vm-uid"),
					},
				}
				attachment := &cnsv1alpha1.CnsNodeVmBatchAttachment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-vm",
						Namespace: ns,
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         vmopv1.GroupVersion.String(),
								Kind:               "VirtualMachine",
								Name:               otherVM.Name,
								UID:                otherVM.UID,
								Controller:         ptr.To(true),
								BlockOwnerDeletion: ptr.To(true),
							},
						},
					},
				}
				initObjects = append(initObjects, attachment)
			})

			It("should return error", func() {
				result, err := reconciler.GetBatchAttachmentForVM(volCtx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("has a different controlling owner"))
				Expect(result).To(BeNil())
			})
		})

		When("attachment exists but has no owner references", func() {
			BeforeEach(func() {
				attachment := &cnsv1alpha1.CnsNodeVmBatchAttachment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-vm",
						Namespace: ns,
						// No owner references
					},
				}
				initObjects = append(initObjects, attachment)
			})

			It("should return error", func() {
				result, err := reconciler.GetBatchAttachmentForVM(volCtx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("has a different controlling owner"))
				Expect(result).To(BeNil())
			})
		})
	})

	Context("HandlePVCWithWFFC", func() {
		var (
			vm               *vmopv1.VirtualMachine
			volCtx           *pkgctx.VolumeContext
			boundPVC1        *corev1.PersistentVolumeClaim
			vmVolumeWithPVC1 *vmopv1.VirtualMachineVolume
			vmVol            *vmopv1.VirtualMachineVolume
		)

		BeforeEach(func() {
			vm = &vmopv1.VirtualMachine{}
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
			volCtx = &pkgctx.VolumeContext{
				Context: ctx,
				Logger:  log.Log,
				VM:      vm,
			}
		})

		AfterEach(func() {
			vm = nil
			volCtx = nil
			vmVol = nil
			vmVolumeWithPVC1 = nil
			boundPVC1 = nil
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
					err := reconciler.HandlePVCWithWFFC(volCtx, *vmVol)
					Expect(err).To(MatchError("PVC with WFFC storage class support is not enabled"))
				})
			})

			When("PVC does not exist", func() {
				BeforeEach(func() {
					vm.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = "bogus"
				})

				It("returns error", func() {
					err := reconciler.HandlePVCWithWFFC(volCtx, *vmVol)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("cannot get PVC"))
				})
			})

			When("PVC StorageClassName is unset", func() {
				BeforeEach(func() {
					wffcPVC.Spec.StorageClassName = nil
				})

				It("returns error", func() {
					err := reconciler.HandlePVCWithWFFC(volCtx, *vmVol)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("does not have StorageClassName set"))
				})
			})

			When("PVC has KubernetesSelectedNodeAnnotationKey annotation", func() {
				var node *corev1.Node
				BeforeEach(func() {
					wffcPVC.Annotations = map[string]string{
						constants.KubernetesSelectedNodeAnnotationKey: "node1",
					}
				})

				When("Node does not exist", func() {
					It("returns error", func() {
						err := reconciler.HandlePVCWithWFFC(volCtx, *vmVol)
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(
							ContainSubstring("cannot get Node \"node1\": nodes \"node1\" not found"))
					})
				})

				When("Node's zone doesn't match the VM's zone", func() {
					BeforeEach(func() {
						node = &corev1.Node{
							ObjectMeta: metav1.ObjectMeta{
								Name: "node1",
								Labels: map[string]string{
									corev1.LabelTopologyZone: "node2",
								},
							},
						}
						initObjects = append(initObjects, node)
					})

					It("returns error", func() {
						err := reconciler.HandlePVCWithWFFC(volCtx, *vmVol)
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(
							ContainSubstring("node \"node1\" is not in the VM's zone \"my-zone\", but in zone \"node2\""))
					})
				})

				When("Node exists and is in the VM's zone", func() {
					BeforeEach(func() {
						node = &corev1.Node{
							ObjectMeta: metav1.ObjectMeta{
								Name: "node1",
								Labels: map[string]string{
									corev1.LabelTopologyZone: zoneName,
								},
							},
						}
						initObjects = append(initObjects, node)
					})

					It("returns success", func() {
						err := reconciler.HandlePVCWithWFFC(volCtx, *vmVol)
						Expect(err).ToNot(HaveOccurred())
					})
				})
			})

			When("StorageClass does not exist", func() {
				BeforeEach(func() {
					wffcPVC.Spec.StorageClassName = ptr.To("bogus")
				})

				It("returns error", func() {
					err := reconciler.HandlePVCWithWFFC(volCtx, *vmVol)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("cannot get StorageClass for PVC"))
				})
			})

			When("VM does not have Zone assigned", func() {
				BeforeEach(func() {
					vm.Status.Zone = ""
				})

				It("returns error", func() {
					err := reconciler.HandlePVCWithWFFC(volCtx, *vmVol)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("VM does not have Zone set"))
				})
			})

			It("returns success", func() {
				err := reconciler.HandlePVCWithWFFC(volCtx, *vmVol)
				Expect(err).ToNot(HaveOccurred())

				By("Adds node-is-zone and selected-node annotation to PVC", func() {
					pvc := &corev1.PersistentVolumeClaim{}
					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(wffcPVC), pvc)).To(Succeed())
					Expect(pvc.Annotations).To(HaveKeyWithValue(volumebatchutils.CNSSelectedNodeIsZoneAnnotationKey, "true"))
					Expect(pvc.Annotations).To(HaveKeyWithValue(constants.KubernetesSelectedNodeAnnotationKey, zoneName))
				})
			})
		})
	})
}
