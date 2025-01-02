// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package kube_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

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
		fakeString,
		kubeutil.ErrMissingParameter{
			StorageClassName: "my-storage-class",
			ParameterName:    internal.StoragePolicyIDParameter,
		}.Error()),
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
		ok           bool
		ctx          context.Context
		err          error
		client       ctrlclient.Client
		funcs        interceptor.Funcs
		withObjs     []ctrlclient.Object
		storageClass storagev1.StorageClass
		profile      string
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
			Parameters: map[string]string{
				internal.StoragePolicyIDParameter: fakeString,
			},
		}
		withObjs = []ctrlclient.Object{&storageClass}
		profile = fakeString
	})

	JustBeforeEach(func() {
		client = fake.NewClientBuilder().
			WithObjects(withObjs...).
			WithInterceptorFuncs(funcs).
			Build()

		Expect(kubeutil.MarkEncryptedStorageClass(
			ctx,
			client,
			storageClass,
			true)).To(Succeed())

		ok, err = kubeutil.IsEncryptedStorageProfile(
			ctx, client, profile)
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
			profile = fakeString + "1"
		})
		It("should return false", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(ok).To(BeFalse())
		})
	})

	When("there is a StorageClass that matches the profile ID", func() {
		It("should return true", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(ok).To(BeTrue())
		})
	})
})

var _ = Describe("EncryptedStorageClass", func() {
	var (
		ctx          context.Context
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
			Parameters: map[string]string{
				internal.StoragePolicyIDParameter: fakeString,
			},
		}
	})

	JustBeforeEach(func() {
		client = fake.NewClientBuilder().
			WithObjects(&storageClass).
			WithInterceptorFuncs(funcs).
			Build()
	})

	When("the storage class is marked as encrypted", func() {
		JustBeforeEach(func() {
			Expect(kubeutil.MarkEncryptedStorageClass(ctx, client, storageClass, true)).To(Succeed())
		})
		It("should return true", func() {
			ok, pid, err := kubeutil.IsEncryptedStorageClass(ctx, client, storageClass.Name)
			Expect(err).ToNot(HaveOccurred())
			Expect(ok).To(BeTrue())
			Expect(pid).To(Equal(fakeString))
		})
		When("the storage class is marked as encrypted again", func() {
			JustBeforeEach(func() {
				Expect(kubeutil.MarkEncryptedStorageClass(ctx, client, storageClass, true)).To(Succeed())
			})
			It("should return true", func() {
				ok, pid, err := kubeutil.IsEncryptedStorageClass(ctx, client, storageClass.Name)
				Expect(err).ToNot(HaveOccurred())
				Expect(ok).To(BeTrue())
				Expect(pid).To(Equal(fakeString))
			})
		})
		When("the storage class is marked as unencrypted", func() {
			JustBeforeEach(func() {
				Expect(kubeutil.MarkEncryptedStorageClass(ctx, client, storageClass, false)).To(Succeed())
			})
			It("should return false", func() {
				ok, pid, err := kubeutil.IsEncryptedStorageClass(ctx, client, storageClass.Name)
				Expect(err).ToNot(HaveOccurred())
				Expect(ok).To(BeFalse())
				Expect(pid).To(BeEmpty())

			})

			When("the storage class is marked as encrypted again", func() {
				JustBeforeEach(func() {
					Expect(kubeutil.MarkEncryptedStorageClass(ctx, client, storageClass, true)).To(Succeed())
				})
				It("should return true", func() {
					ok, pid, err := kubeutil.IsEncryptedStorageClass(ctx, client, storageClass.Name)
					Expect(err).ToNot(HaveOccurred())
					Expect(ok).To(BeTrue())
					Expect(pid).To(Equal(fakeString))
				})
			})
		})

		When("a second storage class is marked as not encrypted", func() {
			JustBeforeEach(func() {
				storageClass.Name += "1"
				Expect(kubeutil.MarkEncryptedStorageClass(ctx, client, storageClass, false)).To(Succeed())
			})
			It("should return false for the second storage class", func() {
				ok, pid, err := kubeutil.IsEncryptedStorageClass(ctx, client, storageClass.Name)
				Expect(err).ToNot(HaveOccurred())
				Expect(ok).To(BeFalse())
				Expect(pid).To(BeEmpty())
			})
		})
	})

	When("the storage class is not marked as not encrypted", func() {
		JustBeforeEach(func() {
			Expect(kubeutil.MarkEncryptedStorageClass(ctx, client, storageClass, false)).To(Succeed())
		})
		It("should return false", func() {
			ok, pid, err := kubeutil.IsEncryptedStorageClass(ctx, client, storageClass.Name)
			Expect(err).ToNot(HaveOccurred())
			Expect(ok).To(BeFalse())
			Expect(pid).To(BeEmpty())
		})
	})

	When("the storage class is not marked at all", func() {
		It("should return false", func() {
			ok, pid, err := kubeutil.IsEncryptedStorageClass(ctx, client, storageClass.Name)
			Expect(err).ToNot(HaveOccurred())
			Expect(ok).To(BeFalse())
			Expect(pid).To(BeEmpty())
		})
	})

	When("getting the underlying ConfigMap produces a non-404 error", func() {
		BeforeEach(func() {
			funcs.Get = func(
				ctx context.Context,
				client ctrlclient.WithWatch,
				key ctrlclient.ObjectKey,
				obj ctrlclient.Object,
				opts ...ctrlclient.GetOption) error {

				if _, ok := obj.(*corev1.ConfigMap); ok {
					return apierrors.NewInternalError(errors.New(fakeString))
				}

				return client.Get(ctx, key, obj, opts...)
			}
		})
		It("should return an error for IsEncryptedStorageClass", func() {
			ok, pid, err := kubeutil.IsEncryptedStorageClass(ctx, client, storageClass.Name)
			Expect(err).To(MatchError(apierrors.NewInternalError(errors.New(fakeString)).Error()))
			Expect(ok).To(BeFalse())
			Expect(pid).To(BeEmpty())
		})
		It("should return an error for MarkEncryptedStorageClass", func() {
			err := kubeutil.MarkEncryptedStorageClass(ctx, client, storageClass, false)
			Expect(err).To(MatchError(apierrors.NewInternalError(errors.New(fakeString)).Error()))
		})
	})

	// Please note, this test requires the use of envtest due to a bug in the
	// fake client that does not support concurrent patch attempts against the
	// same ConfigMap resource.
	When("there are concurrent attempts to update the ConfigMap", func() {

		var (
			env            envtest.Environment
			numAttempts    int
			storageClasses []storagev1.StorageClass
		)

		BeforeEach(func() {
			numAttempts = 10

			ctx = pkgcfg.WithConfig(pkgcfg.Config{
				PodNamespace: "default",
			})
		})

		JustBeforeEach(func() {
			env = envtest.Environment{}
			restConfig, err := env.Start()
			Expect(err).ToNot(HaveOccurred())
			Expect(restConfig).ToNot(BeNil())

			c, err := ctrlclient.New(restConfig, ctrlclient.Options{})
			Expect(err).ToNot(HaveOccurred())
			Expect(c).ToNot(BeNil())
			client = c

			storageClasses = make([]storagev1.StorageClass, numAttempts)
			for i := range storageClasses {
				storageClasses[i].Name = strconv.Itoa(i)
				storageClasses[i].Provisioner = fakeString
				storageClasses[i].Parameters = map[string]string{
					internal.StoragePolicyIDParameter: fakeString,
				}
				Expect(client.Create(ctx, &storageClasses[i])).To(Succeed())
			}
		})

		AfterEach(func() {
			Expect(env.Stop()).To(Succeed())
		})

		When("there are concurrent additions", func() {
			JustBeforeEach(func() {
				// Ensure the ConfigMap exists so all of the additions use the
				// Patch operation.
				Expect(client.Create(ctx, &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      internal.EncryptedStorageClassNamesConfigMapName,
						Namespace: pkgcfg.FromContext(ctx).PodNamespace,
					},
				})).To(Succeed())

				var (
					ready  sync.WaitGroup
					marked sync.WaitGroup
					start  = make(chan struct{})
				)

				marked.Add(len(storageClasses))
				ready.Add(len(storageClasses))

				for i := range storageClasses {
					go func(obj storagev1.StorageClass) {
						defer GinkgoRecover()
						defer func() {
							marked.Done()
						}()
						ready.Done()
						<-start
						Expect(kubeutil.MarkEncryptedStorageClass(
							ctx, client, obj, true)).To(Succeed(),
							"StorageClass.name="+obj.Name)
					}(storageClasses[i])
				}

				ready.Wait()
				close(start)
				marked.Wait()
			})

			Specify("all concurrent attempts should succeed", func() {
				refs, err := internal.GetEncryptedStorageClassRefs(ctx, client)
				Expect(err).ToNot(HaveOccurred())

				expRefs := make([]any, len(storageClasses))
				for i := range storageClasses {
					expRefs[i] = internal.GetOwnerRefForStorageClass(storageClasses[i])
				}
				Expect(refs).To(HaveLen(len(expRefs)))
				Expect(refs).To(ContainElements(expRefs...))

				for i := range storageClasses {
					ok, pid, err := kubeutil.IsEncryptedStorageClass(
						ctx, client, storageClasses[i].Name)
					Expect(err).ToNot(HaveOccurred())
					Expect(ok).To(BeTrue())
					Expect(pid).To(Equal(fakeString))
				}
			})
		})

		When("there are concurrent removals", func() {
			JustBeforeEach(func() {
				// Mark all the StorageClasses as encrypted.
				for i := range storageClasses {
					Expect(kubeutil.MarkEncryptedStorageClass(
						ctx, client, storageClasses[i], true)).To(Succeed())
				}

				var (
					ready  sync.WaitGroup
					marked sync.WaitGroup
					start  = make(chan struct{})
				)

				marked.Add(len(storageClasses))
				ready.Add(len(storageClasses))

				for i := range storageClasses {
					go func(obj storagev1.StorageClass) {
						defer GinkgoRecover()
						defer func() {
							marked.Done()
						}()
						ready.Done()
						<-start
						Expect(kubeutil.MarkEncryptedStorageClass(
							ctx, client, obj, false)).To(Succeed(),
							"StorageClass.name="+obj.Name)
					}(storageClasses[i])
				}

				ready.Wait()
				close(start)
				marked.Wait()
			})

			Specify("all concurrent attempts should succeed", func() {
				refs, err := internal.GetEncryptedStorageClassRefs(ctx, client)
				Expect(err).ToNot(HaveOccurred())
				Expect(refs).To(BeEmpty())

				for i := range storageClasses {
					ok, pid, err := kubeutil.IsEncryptedStorageClass(
						ctx, client, storageClasses[i].Name)
					Expect(err).ToNot(HaveOccurred())
					Expect(ok).To(BeFalse())
					Expect(pid).To(BeEmpty())
				}
			})
		})

		When("there are concurrent additions and removals", func() {
			BeforeEach(func() {
				// Set the number of attempts to odd to ensure the count at the
				// end is not a false-positive due to equal halves.
				numAttempts = 13
			})

			JustBeforeEach(func() {
				// Mark half of the StorageClasses as encrypted.
				for i := range storageClasses {
					if i%2 == 0 {
						Expect(kubeutil.MarkEncryptedStorageClass(
							ctx, client, storageClasses[i], true)).To(Succeed())
					}
				}

				var (
					ready  sync.WaitGroup
					marked sync.WaitGroup
					start  = make(chan struct{})
				)

				marked.Add(len(storageClasses))
				ready.Add(len(storageClasses))

				for i := range storageClasses {
					go func(i int, obj storagev1.StorageClass) {
						defer GinkgoRecover()
						defer func() {
							marked.Done()
						}()
						ready.Done()
						<-start
						Expect(kubeutil.MarkEncryptedStorageClass(
							ctx, client, obj, i%2 != 0)).To(Succeed(),
							"StorageClass.name="+obj.Name)
					}(i, storageClasses[i])
				}

				ready.Wait()
				close(start)
				marked.Wait()
			})

			Specify("all concurrent attempts should succeed", func() {
				refs, err := internal.GetEncryptedStorageClassRefs(ctx, client)
				Expect(err).ToNot(HaveOccurred())

				var expRefs []any
				for i := range storageClasses {
					if i%2 != 0 {
						expRefs = append(
							expRefs,
							internal.GetOwnerRefForStorageClass(storageClasses[i]))
					}
				}
				Expect(refs).To(HaveLen(len(expRefs)))
				Expect(refs).To(ContainElements(expRefs...))

				for i := range storageClasses {
					ok, pid, err := kubeutil.IsEncryptedStorageClass(
						ctx, client, storageClasses[i].Name)
					Expect(err).ToNot(HaveOccurred())
					if i%2 == 0 {
						Expect(ok).To(BeFalse())
						Expect(pid).To(BeEmpty())
					} else {
						Expect(ok).To(BeTrue())
						Expect(pid).To(Equal(fakeString))
					}
				}
			})
		})
	})
})
