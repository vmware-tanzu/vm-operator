// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package volumebatch_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"

	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachine/volumebatch"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

func TestVolumeBatch(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Volume Batch Controller")
}

var _ = Describe("Volume Batch Controller", func() {

	var (
		reconciler *volumebatch.Reconciler
	)

	BeforeEach(func() {
		reconciler = volumebatch.NewReconciler(
			context.Background(),
			nil, // client not needed for unit tests
			log.Log,
			record.New(nil),
		)
	})

	Context("BuildVolumeSpecs", func() {

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

			vm := &vmopv1.VirtualMachine{}
			volCtx := &pkgctx.VolumeContext{
				Context: context.Background(),
				Logger:  log.Log,
				VM:      vm,
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

			vm := &vmopv1.VirtualMachine{}
			volCtx := &pkgctx.VolumeContext{
				Context: context.Background(),
				Logger:  log.Log,
				VM:      vm,
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

			vm := &vmopv1.VirtualMachine{}
			volCtx := &pkgctx.VolumeContext{
				Context: context.Background(),
				Logger:  log.Log,
				VM:      vm,
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

			vm := &vmopv1.VirtualMachine{}
			volCtx := &pkgctx.VolumeContext{
				Context: context.Background(),
				Logger:  log.Log,
				VM:      vm,
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

			vm := &vmopv1.VirtualMachine{}
			volCtx := &pkgctx.VolumeContext{
				Context: context.Background(),
				Logger:  log.Log,
				VM:      vm,
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

	Context("getBatchAttachmentForVM", func() {
		var (
			scheme     *runtime.Scheme
			fakeClient client.Client
			vm         *vmopv1.VirtualMachine
			volCtx     *pkgctx.VolumeContext
		)

		BeforeEach(func() {
			scheme = runtime.NewScheme()
			Expect(vmopv1.AddToScheme(scheme)).To(Succeed())
			Expect(cnsv1alpha1.AddToScheme(scheme)).To(Succeed())

			vm = &vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm",
					Namespace: "test-namespace",
					UID:       types.UID("test-vm-uid"),
				},
			}

			volCtx = &pkgctx.VolumeContext{
				Context: context.Background(),
				Logger:  log.Log,
				VM:      vm,
			}
		})

		It("should return nil when attachment does not exist", func() {
			fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			reconciler := volumebatch.NewReconciler(
				context.Background(),
				fakeClient,
				log.Log,
				record.New(nil),
			)

			attachment, err := reconciler.GetBatchAttachmentForVM(volCtx)
			Expect(err).ToNot(HaveOccurred())
			Expect(attachment).To(BeNil())
		})

		It("should return attachment when it exists and is owned by VM", func() {
			attachment := &cnsv1alpha1.CnsNodeVmBatchAttachment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm",
					Namespace: "test-namespace",
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

			fakeClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(attachment).Build()
			reconciler := volumebatch.NewReconciler(
				context.Background(),
				fakeClient,
				log.Log,
				record.New(nil),
			)

			result, err := reconciler.GetBatchAttachmentForVM(volCtx)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())
			Expect(result.Name).To(Equal("test-vm"))
		})

		It("should return error when attachment exists but is not owned by VM", func() {
			otherVM := &vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "other-vm",
					Namespace: "test-namespace",
					UID:       types.UID("other-vm-uid"),
				},
			}

			attachment := &cnsv1alpha1.CnsNodeVmBatchAttachment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm",
					Namespace: "test-namespace",
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

			fakeClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(attachment).Build()
			reconciler := volumebatch.NewReconciler(
				context.Background(),
				fakeClient,
				log.Log,
				record.New(nil),
			)

			result, err := reconciler.GetBatchAttachmentForVM(volCtx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("has a different controlling owner"))
			Expect(result).To(BeNil())
		})

		It("should return error when attachment exists but has no owner references", func() {
			attachment := &cnsv1alpha1.CnsNodeVmBatchAttachment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm",
					Namespace: "test-namespace",
					// No owner references
				},
			}

			fakeClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(attachment).Build()
			reconciler := volumebatch.NewReconciler(
				context.Background(),
				fakeClient,
				log.Log,
				record.New(nil),
			)

			result, err := reconciler.GetBatchAttachmentForVM(volCtx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("has a different controlling owner"))
			Expect(result).To(BeNil())
		})
	})
})
