// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package kube_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	infrav1 "github.com/vmware-tanzu/vm-operator/external/infra/api/v1alpha1"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/internal"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = DescribeTable("GetStoragePolicyID",
	func(inPolicyID, expPolicyID, expErr string) {
		obj := storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "my-storage-class",
			},
		}
		if inPolicyID != "" {
			obj.Parameters = map[string]string{"storagePolicyID": inPolicyID}
		}
		policyID, err := kubeutil.GetStoragePolicyID(obj)
		if expErr != "" {
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(expErr))
		} else {
			Expect(policyID).To(Equal(expPolicyID))
		}
	},
	Entry(
		"has policy ID",
		fakeString,
		fakeString,
		""),

	Entry(
		"does not have policy ID",
		"",
		"",
		""),
)

var _ = Describe("SetStoragePolicyID", func() {
	var (
		obj storagev1.StorageClass
	)

	BeforeEach(func() {
		obj = storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: fakeString,
			},
			Parameters: map[string]string{
				internal.StoragePolicyIDParameter: fakeString,
			},
		}
	})

	When("storageClass is nil", func() {
		It("should panic", func() {
			Expect(func() {
				kubeutil.SetStoragePolicyID(nil, "")
			}).To(PanicWith("storageClass is nil"))
		})
	})
	When("id is empty", func() {
		It("should remove the policy ID from the StorageClass", func() {
			id, err := kubeutil.GetStoragePolicyID(obj)
			Expect(err).ToNot(HaveOccurred())
			Expect(id).To(Equal(fakeString))

			kubeutil.SetStoragePolicyID(&obj, "")

			id, err = kubeutil.GetStoragePolicyID(obj)
			Expect(err).To(MatchError(kubeutil.ErrMissingParameter{
				StorageClassName: fakeString,
				ParameterName:    internal.StoragePolicyIDParameter,
			}))
			Expect(id).To(BeEmpty())
		})
	})
	When("id is non-empty", func() {
		It("should set the policy ID on the StorageClass", func() {
			kubeutil.SetStoragePolicyID(&obj, fakeString+"1")
			id, err := kubeutil.GetStoragePolicyID(obj)
			Expect(err).ToNot(HaveOccurred())
			Expect(id).To(Equal(fakeString + "1"))
		})
	})
})

var _ = Describe("GetPVCZoneConstraints", func() {

	It("Unmarshal JSON", func() {
		pvcs := make([]corev1.PersistentVolumeClaim, 1)
		pvcs[0] = corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: "bound-pvc",
				Annotations: map[string]string{
					"csi.vsphere.volume-accessible-topology": `[{"topology.kubernetes.io/zone":"zone1"},{"topology.kubernetes.io/zone":"zone2"},{"topology.kubernetes.io/zone":"zone3"}]`,
				},
			},
			Status: corev1.PersistentVolumeClaimStatus{
				Phase: corev1.ClaimBound,
			},
		}
		zones, err := kubeutil.GetPVCZoneConstraints(nil, pvcs)
		Expect(err).ToNot(HaveOccurred())
		Expect(zones.UnsortedList()).To(ConsistOf("zone1", "zone2", "zone3"))
	})

	It("Unmarshal JSON", func() {
		pvcs := make([]corev1.PersistentVolumeClaim, 1)
		pvcs[0] = corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: "pending-pvc",
				Annotations: map[string]string{
					"csi.vsphere.volume-requested-topology": `[{"topology.kubernetes.io/zone":"zone1"},{"topology.kubernetes.io/zone":"zone2"},{"topology.kubernetes.io/zone":"zone3"}]`,
				},
			},
			Status: corev1.PersistentVolumeClaimStatus{
				Phase: corev1.ClaimPending,
			},
		}
		zones, err := kubeutil.GetPVCZoneConstraints(nil, pvcs)
		Expect(err).ToNot(HaveOccurred())
		Expect(zones.UnsortedList()).To(ConsistOf("zone1", "zone2", "zone3"))
	})

	Context("Pending PVC with Immediate StorageClass", func() {

		It("Returns error", func() {
			sc := *builder.DummyStorageClass()
			sc.VolumeBindingMode = ptr.To(storagev1.VolumeBindingImmediate)
			storageClasses := map[string]storagev1.StorageClass{
				sc.Name: sc,
			}

			pvcs := make([]corev1.PersistentVolumeClaim, 1)
			pvcs[0] = corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pending-pvc",
					Annotations: map[string]string{
						"csi.vsphere.volume-requested-topology": `[{"topology.kubernetes.io/zone":"zone1"}]`,
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: &sc.Name,
				},
				Status: corev1.PersistentVolumeClaimStatus{
					Phase: corev1.ClaimPending,
				},
			}

			_, err := kubeutil.GetPVCZoneConstraints(storageClasses, pvcs)
			Expect(err).To(MatchError("PVC pending-pvc is not bound"))
		})
	})

	Context("Bound PVC with Immediate StorageClass and Unbound PVC with WFFC StorageClass", func() {

		It("Returns success with common zones", func() {
			immdSC := *builder.DummyStorageClass()
			immdSC.VolumeBindingMode = ptr.To(storagev1.VolumeBindingImmediate)
			wffcSC := *builder.DummyStorageClass()
			wffcSC.Name += "-wffc"
			wffcSC.VolumeBindingMode = ptr.To(storagev1.VolumeBindingWaitForFirstConsumer)

			storageClasses := map[string]storagev1.StorageClass{
				immdSC.Name: immdSC,
				wffcSC.Name: wffcSC,
			}

			pvcs := make([]corev1.PersistentVolumeClaim, 2)
			pvcs[0] = corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name: "bound-pvc",
					Annotations: map[string]string{
						"csi.vsphere.volume-accessible-topology": `[{"topology.kubernetes.io/zone":"zone1"},{"topology.kubernetes.io/zone":"zoneX"}]`,
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: &immdSC.Name,
				},
				Status: corev1.PersistentVolumeClaimStatus{
					Phase: corev1.ClaimBound,
				},
			}
			pvcs[1] = corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pending-pvc",
					Annotations: map[string]string{
						"csi.vsphere.volume-requested-topology": `[{"topology.kubernetes.io/zone":"zone1"},{"topology.kubernetes.io/zone":"zoneY"}]`,
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: &wffcSC.Name,
				},
				Status: corev1.PersistentVolumeClaimStatus{
					Phase: corev1.ClaimPending,
				},
			}

			zones, err := kubeutil.GetPVCZoneConstraints(storageClasses, pvcs)
			Expect(err).ToNot(HaveOccurred())
			Expect(zones.UnsortedList()).To(ConsistOf("zone1"))
		})
	})

	Context("Bound PVC with accessible zones that include zones not requested", func() {

		It("Returns success with common zones", func() {
			immdSC := *builder.DummyStorageClass()
			immdSC.VolumeBindingMode = ptr.To(storagev1.VolumeBindingImmediate)

			storageClasses := map[string]storagev1.StorageClass{
				immdSC.Name: immdSC,
			}

			pvcs := make([]corev1.PersistentVolumeClaim, 1)
			pvcs[0] = corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name: "bound-pvc",
					Annotations: map[string]string{
						"csi.vsphere.volume-requested-topology":  `[{"topology.kubernetes.io/zone":"zone1"},{"topology.kubernetes.io/zone":"zone2"},{"topology.kubernetes.io/zone":"zone3"}]`,
						"csi.vsphere.volume-accessible-topology": `[{"topology.kubernetes.io/zone":"zone1"},{"topology.kubernetes.io/zone":"zoneX"}]`,
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: &immdSC.Name,
				},
				Status: corev1.PersistentVolumeClaimStatus{
					Phase: corev1.ClaimBound,
				},
			}

			zones, err := kubeutil.GetPVCZoneConstraints(storageClasses, pvcs)
			Expect(err).ToNot(HaveOccurred())
			Expect(zones.UnsortedList()).To(ConsistOf("zone1"))
		})
	})
})

var _ = DescribeTableSubtree("GetPVCZoneConstraints Table",

	func(phase corev1.PersistentVolumeClaimPhase) {

		DescribeTable("PVC Phase",
			func(pvcsZones [][]string, expZones []string, expErr string) {
				var pvcs []corev1.PersistentVolumeClaim
				for i, zones := range pvcsZones {
					var annotations map[string]string

					if len(zones) > 0 {
						var topology []map[string]string
						for _, z := range zones {
							topology = append(topology, map[string]string{"topology.kubernetes.io/zone": z})
						}

						volTopology, err := json.Marshal(topology)
						Expect(err).NotTo(HaveOccurred())
						if phase == corev1.ClaimBound {
							annotations = map[string]string{
								"csi.vsphere.volume-accessible-topology": string(volTopology),
							}
						} else {
							annotations = map[string]string{
								"csi.vsphere.volume-requested-topology": string(volTopology),
							}
						}
					}

					pvc := corev1.PersistentVolumeClaim{
						ObjectMeta: metav1.ObjectMeta{
							Name:        fmt.Sprintf("pvc-%d", i),
							Annotations: annotations,
						},
						Status: corev1.PersistentVolumeClaimStatus{
							Phase: phase,
						},
					}
					pvcs = append(pvcs, pvc)
				}

				zones, err := kubeutil.GetPVCZoneConstraints(nil, pvcs)
				if expErr != "" {
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring(expErr))
					Expect(zones).To(BeEmpty())
				} else {
					Expect(zones.UnsortedList()).To(ConsistOf(expZones))
				}
			},
			Entry(
				"One PVC with no zones",
				[][]string{
					{},
				},
				[]string{},
				""),
			Entry(
				"One PVC with one zone",
				[][]string{
					{"zone1"},
				},
				[]string{"zone1"},
				""),
			Entry(
				"One PVC with one zone and PVC without zone",
				[][]string{
					{},
					{"zone1"},
				},
				[]string{"zone1"},
				""),
			Entry(
				"Multiple PVCs with one common zone",
				[][]string{
					{"zone1", "zoneX"},
					{"zone1", "zoneY"},
					{"zone1", "zoneZ"},
				},
				[]string{"zone1"},
				""),
			Entry(
				"Multiple PVCs with multiple common zones",
				[][]string{
					{"zone1", "zoneX", "zone2"},
					{"zone1", "zoneY", "zone2"},
					{"zone1", "zoneZ", "zone2"},
				},
				[]string{"zone1", "zone2"},
				""),
			Entry(
				"Multiple PVCs with no common zones",
				[][]string{
					{"zoneX"},
					{"zoneY", "zone1"},
					{"zoneZ", "zone1"},
				},
				nil,
				"no allowed zones remaining after applying PVC zone constraints"),
		)
	},
	Entry("Bound", corev1.ClaimBound),
	Entry("Pending", corev1.ClaimPending),
)

var _ = Describe("IsEncryptedStorageClass", func() {
	var (
		ok           bool
		ctx          context.Context
		err          error
		client       ctrlclient.Client
		funcs        interceptor.Funcs
		storageClass storagev1.StorageClass
	)

	BeforeEach(func() {
		ctx = pkgcfg.WithConfig(pkgcfg.Config{
			PodNamespace: fakeString,
		})
		funcs = interceptor.Funcs{}
		storageClass = storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: fakeString,
				UID:  types.UID(uuid.NewString()),
			},
		}
	})

	JustBeforeEach(func() {
		client = fake.NewClientBuilder().
			WithObjects(&storageClass).
			WithInterceptorFuncs(funcs).
			Build()

		ok, _, err = kubeutil.IsEncryptedStorageClass(
			ctx, client, storageClass.Name)
	})

	When("getting the StorageClass returns a 404 error", func() {
		BeforeEach(func() {
			funcs.Get = func(
				ctx context.Context,
				client ctrlclient.WithWatch,
				key ctrlclient.ObjectKey,
				obj ctrlclient.Object,
				opts ...ctrlclient.GetOption) error {

				if _, ok := obj.(*storagev1.StorageClass); ok {
					return apierrors.NewNotFound(
						storagev1.Resource(internal.StorageClassResource),
						obj.GetName())
				}

				return client.Get(ctx, key, obj, opts...)
			}
		})
		It("should not return an error", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(ok).To(BeFalse())
		})
	})

	When("getting the StorageClass returns a non-404 error", func() {
		BeforeEach(func() {
			funcs.Get = func(
				ctx context.Context,
				client ctrlclient.WithWatch,
				key ctrlclient.ObjectKey,
				obj ctrlclient.Object,
				opts ...ctrlclient.GetOption) error {

				if _, ok := obj.(*storagev1.StorageClass); ok {
					return apierrors.NewInternalError(errors.New(fakeString))
				}

				return client.Get(ctx, key, obj, opts...)
			}
		})
		It("should not return the error", func() {
			Expect(err).To(MatchError(apierrors.NewInternalError(errors.New(fakeString)).Error()))
			Expect(ok).To(BeFalse())
		})
	})
})

var _ = Describe("IsEncryptedStorageProfile", func() {
	var (
		ok            bool
		ctx           context.Context
		err           error
		client        ctrlclient.Client
		funcs         interceptor.Funcs
		withObjs      []ctrlclient.Object
		storageClass  storagev1.StorageClass
		storagePolicy infrav1.StoragePolicy
		profileID     string
	)

	BeforeEach(func() {
		ctx = pkgcfg.WithConfig(pkgcfg.Config{
			PodNamespace: fakeString,
		})
		funcs = interceptor.Funcs{}
		profileID = uuid.NewString()
		storageClass = storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: fakeString,
				UID:  types.UID(uuid.NewString()),
			},
			Parameters: map[string]string{
				internal.StoragePolicyIDParameter: profileID,
			},
		}
		storagePolicy = infrav1.StoragePolicy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: fakeString,
				Name:      kubeutil.GetStoragePolicyObjectName(profileID),
				UID:       types.UID(uuid.NewString()),
			},
			Spec: infrav1.StoragePolicySpec{
				ID: profileID,
			},
			Status: infrav1.StoragePolicyStatus{
				Encrypted: true,
			},
		}
		withObjs = []ctrlclient.Object{&storageClass}
	})

	JustBeforeEach(func() {
		scheme := runtime.NewScheme()
		Expect(infrav1.AddToScheme(scheme)).To(Succeed())
		Expect(storagev1.AddToScheme(scheme)).To(Succeed())

		client = fake.NewClientBuilder().
			WithObjects(withObjs...).
			WithScheme(scheme).
			WithInterceptorFuncs(funcs).
			Build()

		ok, err = kubeutil.IsEncryptedStorageProfile(
			ctx, client, profileID)
	})

	When("getting the StorageClass returns an error", func() {
		BeforeEach(func() {
			funcs.List = func(
				ctx context.Context,
				client ctrlclient.WithWatch,
				list ctrlclient.ObjectList,
				opts ...ctrlclient.ListOption) error {

				if _, ok := list.(*storagev1.StorageClassList); ok {
					return apierrors.NewInternalError(errors.New(fakeString))
				}

				return client.List(ctx, list, opts...)
			}
		})
		It("should return the error", func() {
			Expect(err).To(MatchError(apierrors.NewInternalError(errors.New(fakeString)).Error()))
			Expect(ok).To(BeFalse())
		})
	})

	When("there are no StorageClasses", func() {
		BeforeEach(func() {
			withObjs = nil
		})
		It("should return false", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(ok).To(BeFalse())
		})
	})

	When("there is a StorageClass but does not match the profile ID", func() {
		BeforeEach(func() {
			profileID += "1"
		})
		It("should return false", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(ok).To(BeFalse())
		})
	})

	When("there is not a StoragePolicy", func() {
		It("should return false", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(ok).To(BeFalse())
		})
	})

	When("there is a StoragePolicy that matches the profile ID", func() {
		BeforeEach(func() {
			withObjs = append(withObjs, &storagePolicy)
		})
		It("should return true", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(ok).To(BeTrue())
		})
	})
})

var _ = DescribeTable("GetStoragePolicyObjectName",
	func(in, expOut string) {
		Expect(kubeutil.GetStoragePolicyObjectName(in)).To(Equal(expOut))
	},
	Entry(
		"empty policy ID",
		"",
		""),

	Entry(
		"invalid policy ID",
		fakeString,
		"pol-51d9d4388a3dccf4"),

	Entry(
		"valid policy ID",
		"b0b48647-dd96-4e24-8ef0-f5ba458d4d2d",
		"pol-b0b48647-8ef0f5ba458d4d2d"),
)

var _ = Describe("MarkEncryptedStorageClass", func() {
	var (
		ctx           context.Context
		err           error
		client        ctrlclient.Client
		funcs         interceptor.Funcs
		withObjs      []ctrlclient.Object
		storageClass  storagev1.StorageClass
		storagePolicy infrav1.StoragePolicy
		profileID     string
		encrypted     bool
	)

	BeforeEach(func() {
		ctx = pkgcfg.WithConfig(pkgcfg.Config{
			PodNamespace: fakeString,
		})
		funcs = interceptor.Funcs{
			// The real code does a Patch that includes status changes.
			// With WithStatusSubresource, we need to intercept and handle this specially.
			Patch: func(
				ctx context.Context,
				client ctrlclient.WithWatch,
				obj ctrlclient.Object,
				patch ctrlclient.Patch,
				opts ...ctrlclient.PatchOption) error {

				// Save the status before patching (for StoragePolicy)
				var savedStatus *infrav1.StoragePolicyStatus
				if pol, ok := obj.(*infrav1.StoragePolicy); ok {
					savedStatus = pol.Status.DeepCopy()
				}

				// First, apply the spec/metadata patch
				if err := client.Patch(ctx, obj, patch, opts...); err != nil {
					return err
				}

				// Then, apply the status separately (if it's a StoragePolicy and status was set)
				if savedStatus != nil {
					if pol, ok := obj.(*infrav1.StoragePolicy); ok {
						pol.Status = *savedStatus
						return client.Status().Update(ctx, pol)
					}
				}
				return nil
			},
		}
		profileID = uuid.NewString()
		encrypted = true
		storageClass = storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: fakeString,
				UID:  types.UID(uuid.NewString()),
			},
			Parameters: map[string]string{
				internal.StoragePolicyIDParameter: profileID,
			},
		}
		storagePolicy = infrav1.StoragePolicy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: fakeString,
				Name:      kubeutil.GetStoragePolicyObjectName(profileID),
				UID:       types.UID(uuid.NewString()),
			},
			Spec: infrav1.StoragePolicySpec{
				ID: profileID,
			},
		}
		withObjs = []ctrlclient.Object{&storageClass}
	})

	JustBeforeEach(func() {
		scheme := runtime.NewScheme()
		Expect(infrav1.AddToScheme(scheme)).To(Succeed())
		Expect(storagev1.AddToScheme(scheme)).To(Succeed())

		client = fake.NewClientBuilder().
			WithObjects(withObjs...).
			WithScheme(scheme).
			WithStatusSubresource(&infrav1.StoragePolicy{}).
			WithInterceptorFuncs(funcs).
			Build()

		err = kubeutil.MarkEncryptedStorageClass(
			ctx, client, storageClass, encrypted)
	})

	When("StorageClass does not have a policy ID", func() {
		BeforeEach(func() {
			storageClass.Parameters = nil
		})
		It("should return an error", func() {
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(kubeutil.ErrMissingParameter{
				StorageClassName: fakeString,
				ParameterName:    internal.StoragePolicyIDParameter,
			}))
		})
	})

	When("StoragePolicy does not exist", func() {
		It("should create a new StoragePolicy with encryption status", func() {
			Expect(err).ToNot(HaveOccurred())

			var obj infrav1.StoragePolicy
			Expect(client.Get(ctx, ctrlclient.ObjectKey{
				Namespace: fakeString,
				Name:      kubeutil.GetStoragePolicyObjectName(profileID),
			}, &obj)).To(Succeed())

			Expect(obj.Spec.ID).To(Equal(profileID))
			Expect(obj.Status.Encrypted).To(Equal(encrypted))
			Expect(obj.OwnerReferences).To(HaveLen(1))
			Expect(obj.OwnerReferences[0].UID).To(Equal(storageClass.UID))
			Expect(obj.OwnerReferences[0].Name).To(Equal(storageClass.Name))
		})
	})

	When("creating the StoragePolicy returns an error", func() {
		BeforeEach(func() {
			funcs.Create = func(
				ctx context.Context,
				client ctrlclient.WithWatch,
				obj ctrlclient.Object,
				opts ...ctrlclient.CreateOption) error {

				if _, ok := obj.(*infrav1.StoragePolicy); ok {
					return apierrors.NewInternalError(errors.New(fakeString))
				}

				return client.Create(ctx, obj, opts...)
			}
		})
		It("should return the error", func() {
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to create StoragePolicy"))
			Expect(err.Error()).To(ContainSubstring(fakeString))
		})
	})

	When("updating the StoragePolicy status returns an error", func() {
		BeforeEach(func() {
			funcs.SubResourceUpdate = func(
				ctx context.Context,
				client ctrlclient.Client,
				subResourceName string,
				obj ctrlclient.Object,
				opts ...ctrlclient.SubResourceUpdateOption) error {

				if _, ok := obj.(*infrav1.StoragePolicy); ok {
					return apierrors.NewInternalError(errors.New(fakeString))
				}

				return client.SubResource(subResourceName).Update(ctx, obj, opts...)
			}
		})
		It("should return the error", func() {
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to update StoragePolicy"))
			Expect(err.Error()).To(ContainSubstring(fakeString))
		})
	})

	When("StoragePolicy exists and is already marked encrypted with StorageClass as owner", func() {
		BeforeEach(func() {
			storagePolicy.Status.Encrypted = true
			storagePolicy.OwnerReferences = []metav1.OwnerReference{
				{
					APIVersion: storagev1.SchemeGroupVersion.String(),
					Kind:       internal.StorageClassKind,
					Name:       storageClass.Name,
					UID:        storageClass.UID,
				},
			}
			withObjs = append(withObjs, &storagePolicy)
			encrypted = true
		})
		It("should not update the StoragePolicy", func() {
			Expect(err).ToNot(HaveOccurred())

			var obj infrav1.StoragePolicy
			Expect(client.Get(ctx, ctrlclient.ObjectKey{
				Namespace: fakeString,
				Name:      kubeutil.GetStoragePolicyObjectName(profileID),
			}, &obj)).To(Succeed())

			Expect(obj.Status.Encrypted).To(BeTrue())
		})
	})

	When("StoragePolicy exists and is already marked not encrypted with StorageClass as owner", func() {
		BeforeEach(func() {
			storagePolicy.Status.Encrypted = false
			storagePolicy.OwnerReferences = []metav1.OwnerReference{
				{
					APIVersion: storagev1.SchemeGroupVersion.String(),
					Kind:       internal.StorageClassKind,
					Name:       storageClass.Name,
					UID:        storageClass.UID,
				},
			}
			withObjs = append(withObjs, &storagePolicy)
			encrypted = false
		})
		It("should not update the StoragePolicy", func() {
			Expect(err).ToNot(HaveOccurred())

			var obj infrav1.StoragePolicy
			Expect(client.Get(ctx, ctrlclient.ObjectKey{
				Namespace: fakeString,
				Name:      kubeutil.GetStoragePolicyObjectName(profileID),
			}, &obj)).To(Succeed())

			Expect(obj.Status.Encrypted).To(BeFalse())
		})
	})

	When("StoragePolicy exists but encryption status needs to be updated", func() {
		BeforeEach(func() {
			storagePolicy.Status.Encrypted = false
			storagePolicy.OwnerReferences = []metav1.OwnerReference{
				{
					APIVersion: storagev1.SchemeGroupVersion.String(),
					Kind:       internal.StorageClassKind,
					Name:       storageClass.Name,
					UID:        storageClass.UID,
				},
			}
			withObjs = append(withObjs, &storagePolicy)
			encrypted = true
		})
		It("should update the encryption status", func() {
			Expect(err).ToNot(HaveOccurred())

			var obj infrav1.StoragePolicy
			Expect(client.Get(ctx, ctrlclient.ObjectKey{
				Namespace: fakeString,
				Name:      kubeutil.GetStoragePolicyObjectName(profileID),
			}, &obj)).To(Succeed())

			Expect(obj.Status.Encrypted).To(BeTrue())
		})
	})

	When("StoragePolicy exists but StorageClass is not an owner", func() {
		BeforeEach(func() {
			storagePolicy.Status.Encrypted = true
			storagePolicy.OwnerReferences = []metav1.OwnerReference{
				{
					APIVersion: storagev1.SchemeGroupVersion.String(),
					Kind:       internal.StorageClassKind,
					Name:       "other-storage-class",
					UID:        types.UID(uuid.NewString()),
				},
			}
			withObjs = append(withObjs, &storagePolicy)
			encrypted = true
		})
		It("should add StorageClass as an owner", func() {
			Expect(err).ToNot(HaveOccurred())

			var obj infrav1.StoragePolicy
			Expect(client.Get(ctx, ctrlclient.ObjectKey{
				Namespace: fakeString,
				Name:      kubeutil.GetStoragePolicyObjectName(profileID),
			}, &obj)).To(Succeed())

			Expect(obj.OwnerReferences).To(HaveLen(2))
			Expect(obj.Status.Encrypted).To(BeTrue())

			// Check that both owners are present
			var foundOriginal, foundNew bool
			for _, ref := range obj.OwnerReferences {
				if ref.Name == "other-storage-class" {
					foundOriginal = true
				}
				if ref.Name == storageClass.Name && ref.UID == storageClass.UID {
					foundNew = true
				}
			}
			Expect(foundOriginal).To(BeTrue())
			Expect(foundNew).To(BeTrue())
		})
	})

	When("patching the StoragePolicy returns an error", func() {
		BeforeEach(func() {
			storagePolicy.Status.Encrypted = false
			storagePolicy.OwnerReferences = []metav1.OwnerReference{
				{
					APIVersion: storagev1.SchemeGroupVersion.String(),
					Kind:       internal.StorageClassKind,
					Name:       storageClass.Name,
					UID:        storageClass.UID,
				},
			}
			withObjs = append(withObjs, &storagePolicy)
			encrypted = true

			funcs.Patch = func(
				ctx context.Context,
				client ctrlclient.WithWatch,
				obj ctrlclient.Object,
				patch ctrlclient.Patch,
				opts ...ctrlclient.PatchOption) error {

				if _, ok := obj.(*infrav1.StoragePolicy); ok {
					return apierrors.NewInternalError(errors.New(fakeString))
				}

				return client.Patch(ctx, obj, patch, opts...)
			}
		})
		It("should return the error", func() {
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to patch StoragePolicy"))
			Expect(err.Error()).To(ContainSubstring(fakeString))
		})
	})

	When("getting the StoragePolicy returns a non-404 error", func() {
		BeforeEach(func() {
			funcs.Get = func(
				ctx context.Context,
				client ctrlclient.WithWatch,
				key ctrlclient.ObjectKey,
				obj ctrlclient.Object,
				opts ...ctrlclient.GetOption) error {

				if _, ok := obj.(*infrav1.StoragePolicy); ok {
					return apierrors.NewInternalError(errors.New(fakeString))
				}

				return client.Get(ctx, key, obj, opts...)
			}
		})
		It("should return the error", func() {
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(fakeString))
		})
	})
})
