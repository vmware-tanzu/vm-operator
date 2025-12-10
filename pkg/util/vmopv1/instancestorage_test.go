// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1_test

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	apirecord "k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("IsInstanceStoragePresent", func() {
	var vm *vmopv1.VirtualMachine

	BeforeEach(func() {
		vm = &vmopv1.VirtualMachine{}
	})

	When("VM does not have instance storage", func() {
		BeforeEach(func() {
			vm.Spec.Volumes = append(vm.Spec.Volumes, vmopv1.VirtualMachineVolume{
				Name: "my-vol",
				VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
					PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
						PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: "my-pvc",
						},
						InstanceVolumeClaim: nil,
					},
				},
			})
		})

		It("returns false", func() {
			Expect(vmopv1util.IsInstanceStoragePresent(vm)).To(BeFalse())
		})
	})

	When("VM has instance storage", func() {
		BeforeEach(func() {
			vm.Spec.Volumes = append(vm.Spec.Volumes,
				vmopv1.VirtualMachineVolume{
					Name: "my-vol",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
							PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "my-pvc",
							},
						},
					},
				},
				vmopv1.VirtualMachineVolume{
					Name: "my-is-vol",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
							PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "my-is-pvc",
							},
							InstanceVolumeClaim: &vmopv1.InstanceVolumeClaimVolumeSource{
								StorageClass: "foo",
							},
						},
					},
				})
		})

		It("returns true", func() {
			Expect(vmopv1util.IsInstanceStoragePresent(vm)).To(BeTrue())
		})
	})
})

var _ = Describe("FilterInstanceStorageVolumes", func() {
	var vm *vmopv1.VirtualMachine

	BeforeEach(func() {
		vm = &vmopv1.VirtualMachine{}
	})

	When("VM has no volumes", func() {
		It("returns empty list", func() {
			Expect(vmopv1util.FilterInstanceStorageVolumes(vm)).To(BeEmpty())
		})
	})

	When("VM has volumes", func() {
		BeforeEach(func() {
			vm.Spec.Volumes = append(vm.Spec.Volumes,
				vmopv1.VirtualMachineVolume{
					Name: "my-vol",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
							PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "my-pvc",
							},
						},
					},
				},
				vmopv1.VirtualMachineVolume{
					Name: "my-is-vol",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
							PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "my-is-pvc",
							},
							InstanceVolumeClaim: &vmopv1.InstanceVolumeClaimVolumeSource{
								StorageClass: "foo",
							},
						},
					},
				})
		})

		It("returns instance storage volumes", func() {
			volumes := vmopv1util.FilterInstanceStorageVolumes(vm)
			Expect(volumes).To(HaveLen(1))
			Expect(volumes[0]).To(Equal(vm.Spec.Volumes[1]))
		})
	})
})

var _ = Describe("IsInsufficientQuota", func() {

	It("returns true for insufficient quota error", func() {
		err := apierrors.NewForbidden(schema.GroupResource{}, "", errors.New("insufficient quota for creating PVC"))
		Expect(vmopv1util.IsInsufficientQuota(err)).To(BeTrue())
	})

	It("returns true for exceeded quota error", func() {
		err := apierrors.NewForbidden(schema.GroupResource{}, "", errors.New("exceeded quota for creating PVC"))
		Expect(vmopv1util.IsInsufficientQuota(err)).To(BeTrue())
	})

	It("returns false for non-forbidden error", func() {
		err := apierrors.NewBadRequest("bad request")
		Expect(vmopv1util.IsInsufficientQuota(err)).To(BeFalse())
	})

	It("returns false for forbidden error without quota message", func() {
		err := apierrors.NewForbidden(schema.GroupResource{}, "", errors.New("other forbidden error"))
		Expect(vmopv1util.IsInsufficientQuota(err)).To(BeFalse())
	})
})

var _ = Describe("ShouldRequeueForInstanceStoragePVCs", func() {
	var (
		ctx context.Context
		vm  *vmopv1.VirtualMachine
	)

	BeforeEach(func() {
		ctx = context.Background()
		vm = &vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{},
		}
	})

	When("instance storage feature is disabled", func() {
		BeforeEach(func() {
			ctx = pkgcfg.WithContext(ctx, pkgcfg.Config{
				Features: pkgcfg.FeatureStates{
					InstanceStorage: false,
				},
			})
		})

		It("returns empty result", func() {
			result := vmopv1util.ShouldRequeueForInstanceStoragePVCs(ctx, vm)
			Expect(result.RequeueAfter).To(BeZero())
		})
	})

	When("instance storage feature is enabled", func() {
		BeforeEach(func() {
			ctx = pkgcfg.WithContext(ctx, pkgcfg.Config{
				Features: pkgcfg.FeatureStates{
					InstanceStorage: true,
				},
				InstanceStorage: pkgcfg.InstanceStorage{
					SeedRequeueDuration: time.Second * 5,
					JitterMaxFactor:     1.0,
				},
			})
		})

		When("VM has no instance storage volumes", func() {
			It("returns empty result", func() {
				result := vmopv1util.ShouldRequeueForInstanceStoragePVCs(ctx, vm)
				Expect(result.RequeueAfter).To(BeZero())
			})
		})

		When("VM has instance storage volumes", func() {
			BeforeEach(func() {
				vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
					{
						Name: "my-is-vol",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "my-is-pvc",
								},
								InstanceVolumeClaim: &vmopv1.InstanceVolumeClaimVolumeSource{
									StorageClass: "foo",
								},
							},
						},
					},
				}
			})

			When("PVCs are already bound", func() {
				BeforeEach(func() {
					vm.Annotations = map[string]string{
						"vmoperator.vmware.com/instance-storage-pvcs-bound": "true",
					}
				})

				It("returns empty result", func() {
					result := vmopv1util.ShouldRequeueForInstanceStoragePVCs(ctx, vm)
					Expect(result.RequeueAfter).To(BeZero())
				})
			})

			When("PVCs are not yet bound", func() {
				It("returns requeue result", func() {
					result := vmopv1util.ShouldRequeueForInstanceStoragePVCs(ctx, vm)
					Expect(result.RequeueAfter).To(BeNumerically(">", 0))
					// Should be around 5 seconds with jitter
					Expect(result.RequeueAfter).To(BeNumerically("<=", time.Second*10))
				})
			})
		})
	})
})

var _ = Describe("ReconcileInstanceStoragePVCs", func() {
	const (
		storageClass = "test-storage-class"
	)

	var (
		ctx          *pkgctx.VolumeContext
		k8sClient    client.Client
		reader       client.Reader
		recorder     record.Recorder
		vm           *vmopv1.VirtualMachine
		ready        bool
		reconcileErr error

		// Helper function to create a PVC with common fields and optional customizations
		newInstanceStoragePVC func(opts ...func(*corev1.PersistentVolumeClaim)) *corev1.PersistentVolumeClaim
	)

	BeforeEach(func() {
		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-vm",
				Namespace: "test-namespace",
				UID:       "test-uid",
			},
			Spec: vmopv1.VirtualMachineSpec{},
		}
		baseCtx := pkgcfg.WithContext(context.Background(), pkgcfg.Config{
			Features: pkgcfg.FeatureStates{
				InstanceStorage: true,
			},
			InstanceStorage: pkgcfg.InstanceStorage{
				PVPlacementFailedTTL: 5 * time.Minute,
			},
		})
		ctx = &pkgctx.VolumeContext{
			Context: baseCtx,
			VM:      vm,
		}
		recorder = record.New(apirecord.NewFakeRecorder(100))

		// Initialize helper function to create instance storage PVCs with common defaults
		newInstanceStoragePVC = func(opts ...func(*corev1.PersistentVolumeClaim)) *corev1.PersistentVolumeClaim {
			sc := storageClass
			pvc := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "is-pvc-1",
					Namespace: vm.Namespace,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         "vmoperator.vmware.com/v1alpha5",
							Kind:               "VirtualMachine",
							Name:               vm.Name,
							UID:                vm.UID,
							Controller:         ptr.To(true),
							BlockOwnerDeletion: ptr.To(true),
						},
					},
					Annotations: map[string]string{
						"volume.kubernetes.io/selected-node": "test-node-1",
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: &sc,
					AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				},
				Status: corev1.PersistentVolumeClaimStatus{
					Phase: corev1.ClaimBound,
				},
			}
			// Apply optional customizations
			for _, opt := range opts {
				opt(pvc)
			}
			return pvc
		}
	})

	JustBeforeEach(func() {
		ready, reconcileErr = vmopv1util.ReconcileInstanceStoragePVCs(ctx, k8sClient, reader, recorder)
	})

	When("VM has no instance storage volumes", func() {
		BeforeEach(func() {
			// Use separate clients to match real-world usage pattern
			k8sClient = builder.NewFakeClient()
			reader = builder.NewFakeClient()
		})

		It("returns ready without error", func() {
			Expect(ready).To(BeTrue())
			Expect(reconcileErr).ToNot(HaveOccurred())
		})
	})

	When("VM has instance storage volumes", func() {
		var storageClass string

		BeforeEach(func() {
			storageClass = "test-storage-class"
			vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
				{
					Name: "is-pvc-1",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
							PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "is-pvc-1",
							},
							InstanceVolumeClaim: &vmopv1.InstanceVolumeClaimVolumeSource{
								StorageClass: storageClass,
							},
						},
					},
				},
			}
		})

		When("no PVCs exist and no node is selected", func() {
			BeforeEach(func() {
				// Use separate clients to match real-world usage pattern
				k8sClient = builder.NewFakeClient()
				reader = builder.NewFakeClient()
			})

			It("returns not ready without creating PVCs", func() {
				Expect(ready).To(BeFalse())
				Expect(reconcileErr).ToNot(HaveOccurred())

				pvcList := &corev1.PersistentVolumeClaimList{}
				Expect(k8sClient.List(ctx, pvcList, client.InNamespace(vm.Namespace))).To(Succeed())
				Expect(pvcList.Items).To(BeEmpty())
			})
		})

		When("node is selected but PVCs don't exist", func() {
			BeforeEach(func() {
				vm.Annotations = map[string]string{
					"vmoperator.vmware.com/instance-storage-selected-node": "test-node-1",
				}
				// Use separate clients to match real-world usage pattern
				k8sClient = builder.NewFakeClient(vm)
				reader = builder.NewFakeClient(vm)
			})

			It("creates PVCs and returns not ready", func() {
				Expect(ready).To(BeFalse())
				Expect(reconcileErr).ToNot(HaveOccurred())

				pvcList := &corev1.PersistentVolumeClaimList{}
				Expect(k8sClient.List(ctx, pvcList, client.InNamespace(vm.Namespace))).To(Succeed())
				Expect(pvcList.Items).To(HaveLen(1))
				Expect(pvcList.Items[0].Name).To(Equal("is-pvc-1"))
				Expect(pvcList.Items[0].Annotations).To(HaveKeyWithValue("volume.kubernetes.io/selected-node", "test-node-1"))
			})
		})

		When("PVCs exist and are bound", func() {
			BeforeEach(func() {
				vm.Annotations = map[string]string{
					"vmoperator.vmware.com/instance-storage-selected-node": "test-node-1",
				}
				pvc := newInstanceStoragePVC() // Already bound by default
				// Use separate clients with same data to match real-world usage pattern
				k8sClient = builder.NewFakeClient(vm, pvc)
				reader = builder.NewFakeClient(vm, pvc)
			})

			It("returns ready and sets bound annotation", func() {
				Expect(ready).To(BeTrue())
				Expect(reconcileErr).ToNot(HaveOccurred())
				Expect(vm.Annotations).To(HaveKeyWithValue("vmoperator.vmware.com/instance-storage-pvcs-bound", "true"))
			})
		})

		When("PVCs exist but are not yet bound", func() {
			BeforeEach(func() {
				vm.Annotations = map[string]string{
					"vmoperator.vmware.com/instance-storage-selected-node": "test-node-1",
				}
				pvc := newInstanceStoragePVC(func(p *corev1.PersistentVolumeClaim) {
					p.Status.Phase = corev1.ClaimPending
				})
				// Use separate clients with same data to match real-world usage pattern
				k8sClient = builder.NewFakeClient(vm, pvc)
				reader = builder.NewFakeClient(vm, pvc)
			})

			It("returns not ready without error", func() {
				Expect(ready).To(BeFalse())
				Expect(reconcileErr).ToNot(HaveOccurred())
			})
		})

		When("PVC has failed placement", func() {
			BeforeEach(func() {
				vm.Annotations = map[string]string{
					"vmoperator.vmware.com/instance-storage-selected-node":      "test-node-1",
					"vmoperator.vmware.com/instance-storage-selected-node-moid": "host-123",
				}
				pvc := newInstanceStoragePVC(func(p *corev1.PersistentVolumeClaim) {
					p.CreationTimestamp = metav1.NewTime(time.Now().Add(-10 * time.Minute))
					p.Annotations["failure-domain.beta.vmware.com/storagepool"] = "FAILED_PLACEMENT-NotEnoughResources"
					p.Status.Phase = corev1.ClaimPending
				})
				// Use separate clients with same data to match real-world usage pattern
				k8sClient = builder.NewFakeClient(vm, pvc)
				reader = builder.NewFakeClient(vm, pvc)
			})

			It("clears node annotations and deletes failed PVC", func() {
				Expect(ready).To(BeFalse())
				Expect(reconcileErr).ToNot(HaveOccurred())
				Expect(vm.Annotations).ToNot(HaveKey("vmoperator.vmware.com/instance-storage-selected-node"))
				Expect(vm.Annotations).ToNot(HaveKey("vmoperator.vmware.com/instance-storage-selected-node-moid"))

				pvc := &corev1.PersistentVolumeClaim{}
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "is-pvc-1", Namespace: vm.Namespace}, pvc)
				Expect(apierrors.IsNotFound(err)).To(BeTrue())
			})
		})

		When("PVC exists on different node", func() {
			BeforeEach(func() {
				vm.Annotations = map[string]string{
					"vmoperator.vmware.com/instance-storage-selected-node": "test-node-2",
				}
				pvc := newInstanceStoragePVC(func(p *corev1.PersistentVolumeClaim) {
					// PVC is on test-node-1 but VM wants test-node-2
					p.Annotations["volume.kubernetes.io/selected-node"] = "test-node-1"
					p.Status.Phase = corev1.ClaimPending
				})
				// Use separate clients with same data to match real-world usage pattern
				k8sClient = builder.NewFakeClient(vm, pvc)
				reader = builder.NewFakeClient(vm, pvc)
			})

			It("deletes stale PVC and creates new one", func() {
				Expect(ready).To(BeFalse())
				Expect(reconcileErr).ToNot(HaveOccurred())

				// Old PVC should be deleted
				pvc := &corev1.PersistentVolumeClaim{}
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "is-pvc-1", Namespace: vm.Namespace}, pvc)
				if err == nil {
					// If it still exists (race condition), check it was marked for deletion or recreated
					Expect(pvc.Annotations["volume.kubernetes.io/selected-node"]).To(Equal("test-node-2"))
				}
			})
		})

		When("PVC is being deleted", func() {
			When("cache and API server both show PVC being deleted (pessimistic case)", func() {
				BeforeEach(func() {
					vm.Annotations = map[string]string{
						"vmoperator.vmware.com/instance-storage-selected-node": "test-node-1",
					}
					now := metav1.Now()
					pvc := newInstanceStoragePVC(func(p *corev1.PersistentVolumeClaim) {
						p.DeletionTimestamp = &now
						p.Finalizers = []string{"kubernetes.io/pvc-protection"}
						p.Status.Phase = corev1.ClaimPending
					})
					// Use separate clients with same data to match real-world usage pattern
					k8sClient = builder.NewFakeClient(vm, pvc)
					reader = builder.NewFakeClient(vm, pvc)
				})

				It("tries to create new PVC but gets AlreadyExists error", func() {
					// Pessimistic case: PVC still exists with finalizer blocking deletion
					// Create will fail with AlreadyExists, caller should requeue
					Expect(ready).To(BeFalse())
					Expect(reconcileErr).To(HaveOccurred())
					Expect(reconcileErr.Error()).To(ContainSubstring("already exists"))
				})
			})

			When("cache shows PVC being deleted but API server shows it's gone (optimistic case - cache staleness)", func() {
				BeforeEach(func() {
					vm.Annotations = map[string]string{
						"vmoperator.vmware.com/instance-storage-selected-node": "test-node-1",
					}
					now := metav1.Now()
					stalePVC := newInstanceStoragePVC(func(p *corev1.PersistentVolumeClaim) {
						p.DeletionTimestamp = &now
						p.Finalizers = []string{"kubernetes.io/pvc-protection"}
						p.Status.Phase = corev1.ClaimPending
					})
					// Reader (cache) has stale view - PVC still being deleted
					reader = builder.NewFakeClient(vm, stalePVC)
					// Writer (API server) has current view - PVC is gone, CSI finished cleanup
					k8sClient = builder.NewFakeClient(vm)
				})

				It("successfully creates new PVC because old one is actually gone", func() {
					// Optimistic case: Cache is stale, PVC is actually deleted
					// Create succeeds, demonstrating cache staleness tolerance
					Expect(ready).To(BeFalse())
					Expect(reconcileErr).ToNot(HaveOccurred())

					// Verify new PVC was created on the API server (writer)
					pvc := &corev1.PersistentVolumeClaim{}
					err := k8sClient.Get(ctx, client.ObjectKey{Name: "is-pvc-1", Namespace: vm.Namespace}, pvc)
					Expect(err).ToNot(HaveOccurred())
					Expect(pvc.DeletionTimestamp).To(BeNil()) // New PVC, not being deleted
					Expect(metav1.IsControlledBy(pvc, vm)).To(BeTrue())
				})
			})
		})

		When("PVC is not owned by VM", func() {
			When("cache and API server both show unowned PVC (pessimistic case)", func() {
				BeforeEach(func() {
					vm.Annotations = map[string]string{
						"vmoperator.vmware.com/instance-storage-selected-node": "test-node-1",
					}
					pvc := newInstanceStoragePVC(func(p *corev1.PersistentVolumeClaim) {
						// Remove owner references to simulate orphaned PVC
						p.OwnerReferences = nil
					})
					// Use separate clients with same data to match real-world usage pattern
					k8sClient = builder.NewFakeClient(vm, pvc)
					reader = builder.NewFakeClient(vm, pvc)
				})

				It("tries to create owned PVC but gets AlreadyExists error", func() {
					// Pessimistic case: Unowned PVC still exists (orphaned from previous VM)
					// Create will fail with AlreadyExists, needs manual cleanup or requeue
					Expect(ready).To(BeFalse())
					Expect(reconcileErr).To(HaveOccurred())
					Expect(reconcileErr.Error()).To(ContainSubstring("already exists"))
				})
			})

			When("cache shows unowned PVC but API server shows it's gone (optimistic case - cache staleness)", func() {
				BeforeEach(func() {
					vm.Annotations = map[string]string{
						"vmoperator.vmware.com/instance-storage-selected-node": "test-node-1",
					}
					unownedPVC := newInstanceStoragePVC(func(p *corev1.PersistentVolumeClaim) {
						// Remove owner references - orphaned from previous VM
						p.OwnerReferences = nil
					})
					// Reader (cache) has stale view - unowned PVC exists
					reader = builder.NewFakeClient(vm, unownedPVC)
					// Writer (API server) has current view - unowned PVC was cleaned up
					k8sClient = builder.NewFakeClient(vm)
				})

				It("successfully creates properly owned PVC because unowned one is actually gone", func() {
					// Optimistic case: Cache is stale, unowned PVC was cleaned up
					// Create succeeds with proper ownership
					Expect(ready).To(BeFalse())
					Expect(reconcileErr).ToNot(HaveOccurred())

					// Verify properly owned PVC was created on the API server (writer)
					pvc := &corev1.PersistentVolumeClaim{}
					err := k8sClient.Get(ctx, client.ObjectKey{Name: "is-pvc-1", Namespace: vm.Namespace}, pvc)
					Expect(err).ToNot(HaveOccurred())
					Expect(metav1.IsControlledBy(pvc, vm)).To(BeTrue()) // Properly owned
					Expect(pvc.Labels).To(HaveKeyWithValue("vmoperator.vmware.com/instance-storage-resource", "true"))
				})
			})
		})
	})
})
