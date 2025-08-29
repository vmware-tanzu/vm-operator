// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package storagepolicyusage_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha5/common"
	spqv1 "github.com/vmware-tanzu/vm-operator/external/storage-policy-quota/api/v1alpha2"
	"github.com/vmware-tanzu/vm-operator/pkg/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	spqutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube/spq"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {
	Describe(
		"Reconcile",
		Label(
			testlabels.Controller,
			testlabels.EnvTest,
		),
		intgTestsReconcile,
	)
}

func intgTestsReconcile() {

	const (
		storageQuotaName = "my-storage-quota"
		storageClassName = "my-storage-class"
		storagePolicyID  = "my-storage-policy"
	)

	var (
		ctx *builder.IntegrationTestContext
	)

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()
		// Please note this is necessary to ensure the test context has the
		// channel the controller uses to receive events.
		ctx.Context = cource.JoinContext(ctx.Context, suite.Context)
	})

	AfterEach(func() {
		ctx.AfterEach()
	})

	BeforeEach(func() {
		By("create StorageClass", func() {
			Expect(ctx.Client.Create(
				ctx,
				&storagev1.StorageClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: storageClassName,
					},
					Provisioner: "fake",
					Parameters: map[string]string{
						"storagePolicyID": storagePolicyID,
					},
				})).To(Succeed())
		})

		By("create StoragePolicyQuota", func() {
			obj := spqv1.StoragePolicyQuota{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: ctx.Namespace,
					Name:      storageQuotaName,
				},
				Spec: spqv1.StoragePolicyQuotaSpec{
					StoragePolicyId: storagePolicyID,
				},
			}
			Expect(ctx.Client.Create(ctx, &obj)).To(Succeed())
			Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(&obj), &obj)).To(Succeed())
			obj.Status = spqv1.StoragePolicyQuotaStatus{
				SCLevelQuotaStatuses: spqv1.SCLevelQuotaStatusList{
					{
						StorageClassName: storageClassName,
					},
				},
			}
			Expect(ctx.Client.Status().Update(ctx, &obj)).To(Succeed())
		})

		By("create StoragePolicyUsage for VM", func() {
			Expect(ctx.Client.Create(ctx, &spqv1.StoragePolicyUsage{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: ctx.Namespace,
					Name:      spqutil.StoragePolicyUsageNameForVM(storageClassName),
				},
				Spec: spqv1.StoragePolicyUsageSpec{
					StoragePolicyId:       storagePolicyID,
					StorageClassName:      storageClassName,
					ResourceAPIgroup:      ptr.To(vmopv1.GroupVersion.Group),
					ResourceKind:          spqutil.VirtualMachineKind,
					ResourceExtensionName: spqutil.StoragePolicyQuotaVMExtensionName,
				},
			})).To(Succeed())
		})

		By("create StoragePolicyUsage for VMSnapshot", func() {
			Expect(ctx.Client.Create(ctx, &spqv1.StoragePolicyUsage{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: ctx.Namespace,
					Name:      spqutil.StoragePolicyUsageNameForVMSnapshot(storageClassName),
				},
				Spec: spqv1.StoragePolicyUsageSpec{
					StoragePolicyId:       storagePolicyID,
					StorageClassName:      storageClassName,
					ResourceAPIgroup:      ptr.To(vmopv1.GroupVersion.Group),
					ResourceKind:          spqutil.VirtualMachineSnapshotKind,
					ResourceExtensionName: spqutil.StoragePolicyQuotaVMSnapshotExtensionName,
				},
			})).To(Succeed())
		})

		By("create VirtualMachines", func() {
			list := []vmopv1.VirtualMachine{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ctx.Namespace,
						Name:      "vm-1",
					},
					Spec: vmopv1.VirtualMachineSpec{
						ClassName:    "my-vm-class",
						ImageName:    "my-vm-image",
						StorageClass: storageClassName,
						Volumes: []vmopv1.VirtualMachineVolume{
							{
								Name: "vm-1-volume-1",
								VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
									PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
										PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
											ClaimName: "vm-1-pvc-1",
										},
									},
								},
							},
						},
					},
					Status: vmopv1.VirtualMachineStatus{
						Conditions: []metav1.Condition{
							{
								Type:               vmopv1.VirtualMachineConditionCreated,
								Status:             metav1.ConditionTrue,
								Reason:             "created",
								LastTransitionTime: metav1.Now(),
							},
						},
						Storage: &vmopv1.VirtualMachineStorageStatus{
							Total: ptr.To(resource.MustParse("20Gi")),
						},
						Volumes: []vmopv1.VirtualMachineVolumeStatus{
							{
								Name:  "vm-1-volume-1",
								Limit: ptr.To(resource.MustParse("10Gi")),
								Type:  vmopv1.VolumeTypeManaged,
							},
							{
								Name:  "vm-1-volume-2",
								Limit: ptr.To(resource.MustParse("20Gi")),
								Type:  vmopv1.VolumeTypeClassic,
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ctx.Namespace,
						Name:      "vm-2",
					},
					Spec: vmopv1.VirtualMachineSpec{
						ClassName:    "my-vm-class",
						ImageName:    "my-vm-image",
						StorageClass: storageClassName,
						Volumes: []vmopv1.VirtualMachineVolume{
							{
								Name: "vm-2-volume-1",
								VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
									PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
										PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
											ClaimName: "vm-2-pvc-1",
										},
									},
								},
							},
						},
					},
					Status: vmopv1.VirtualMachineStatus{
						Conditions: []metav1.Condition{
							{
								Type:               vmopv1.VirtualMachineConditionCreated,
								Status:             metav1.ConditionTrue,
								Reason:             "created",
								LastTransitionTime: metav1.Now(),
							},
						},
						Storage: &vmopv1.VirtualMachineStorageStatus{
							Total: ptr.To(resource.MustParse("50Gi")),
						},
						Volumes: []vmopv1.VirtualMachineVolumeStatus{
							{
								Name:  "vm-2-volume-1",
								Limit: ptr.To(resource.MustParse("10Gi")),
								Type:  vmopv1.VolumeTypeManaged,
							},
							{
								Name:  "vm-2-volume-2",
								Limit: ptr.To(resource.MustParse("20Gi")),
								Type:  vmopv1.VolumeTypeClassic,
							},
						},
					},
				},
			}
			for i := range list {
				obj := &list[i]
				originalStatus := obj.DeepCopy().Status

				Expect(ctx.Client.Create(ctx, obj)).To(Succeed())
				Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(obj), obj)).To(Succeed())
				obj.Status = originalStatus
				Expect(ctx.Client.Status().Update(ctx, obj)).To(Succeed())
			}
		})

		By("create VMSnapshots", func() {
			list := []vmopv1.VirtualMachineSnapshot{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ctx.Namespace,
						Name:      "vm-snapshot-1",
						Annotations: map[string]string{
							constants.CSIVSphereVolumeSyncAnnotationKey: constants.CSIVSphereVolumeSyncAnnotationValueRequested,
						},
					},
					Spec: vmopv1.VirtualMachineSnapshotSpec{
						VMRef: &vmopv1common.LocalObjectRef{
							APIVersion: vmopv1.GroupVersion.String(),
							Kind:       spqutil.VirtualMachineKind,
							Name:       "vm-1",
						},
					},
					Status: vmopv1.VirtualMachineSnapshotStatus{
						Storage: &vmopv1.VirtualMachineSnapshotStorageStatus{
							Used: ptr.To(resource.MustParse("10Gi")),
							Requested: []vmopv1.VirtualMachineSnapshotStorageStatusRequested{
								{
									StorageClass: storageClassName,
									Total:        ptr.To(resource.MustParse("15Gi")),
								},
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ctx.Namespace,
						Name:      "vm-snapshot-2",
						Annotations: map[string]string{
							constants.CSIVSphereVolumeSyncAnnotationKey: constants.CSIVSphereVolumeSyncAnnotationValueCompleted,
						},
					},
					Spec: vmopv1.VirtualMachineSnapshotSpec{
						VMRef: &vmopv1common.LocalObjectRef{
							APIVersion: vmopv1.GroupVersion.String(),
							Kind:       spqutil.VirtualMachineKind,
							Name:       "vm-2",
						},
					},
					Status: vmopv1.VirtualMachineSnapshotStatus{
						Conditions: []metav1.Condition{
							{
								Type:               vmopv1.VirtualMachineSnapshotReadyCondition,
								Status:             metav1.ConditionFalse,
								Reason:             "notReady",
								LastTransitionTime: metav1.Now(),
							},
						},
						Storage: &vmopv1.VirtualMachineSnapshotStorageStatus{
							Used: ptr.To(resource.MustParse("20Gi")),
							Requested: []vmopv1.VirtualMachineSnapshotStorageStatusRequested{
								{
									StorageClass: storageClassName,
									Total:        ptr.To(resource.MustParse("25Gi")),
								},
							},
						},
					},
				},
			}

			for i := range list {
				obj := &list[i]
				originalStatus := obj.DeepCopy().Status

				Expect(ctx.Client.Create(ctx, obj)).To(Succeed())
				Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(obj), obj)).To(Succeed())
				obj.Status = originalStatus
				Expect(ctx.Client.Status().Update(ctx, obj)).To(Succeed())
			}
		})
	})

	When("there is a single reconcile request", func() {
		BeforeEach(func() {
			vmopv1util.SyncStorageUsageForNamespace(
				ctx,
				ctx.Namespace,
				storageClassName)
		})

		It("should sync the storage usage for the namespace", func() {
			Eventually(func(g Gomega) {
				var spuForVM spqv1.StoragePolicyUsage
				g.Expect(ctx.Client.Get(
					ctx,
					client.ObjectKey{
						Namespace: ctx.Namespace,
						Name:      spqutil.StoragePolicyUsageNameForVM(storageClassName),
					},
					&spuForVM),
				).To(Succeed())
				g.Expect(spuForVM.Status.ResourceTypeLevelQuotaUsage).ToNot(BeNil())
				g.Expect(spuForVM.Status.ResourceTypeLevelQuotaUsage.Reserved).ToNot(BeNil())
				g.Expect(spuForVM.Status.ResourceTypeLevelQuotaUsage.Reserved.Value()).To(Equal(int64(0)))
				g.Expect(spuForVM.Status.ResourceTypeLevelQuotaUsage.Used).ToNot(BeNil())
				g.Expect(spuForVM.Status.ResourceTypeLevelQuotaUsage.Used).To(Equal(ptr.To(resource.MustParse("70Gi"))))
			}, time.Second*5).Should(Succeed())

			// The SPU for VMSnapshot should not be updated since the feature is disabled.
			Consistently(func(g Gomega) {
				var spuForVMSnapshot spqv1.StoragePolicyUsage
				g.Expect(ctx.Client.Get(
					ctx,
					client.ObjectKey{
						Namespace: ctx.Namespace,
						Name:      spqutil.StoragePolicyUsageNameForVMSnapshot(storageClassName),
					},
					&spuForVMSnapshot),
				).To(Succeed())
				g.Expect(spuForVMSnapshot.Status.ResourceTypeLevelQuotaUsage).To(BeNil())
			}, time.Second*5).Should(Succeed())
		})
	})
}
