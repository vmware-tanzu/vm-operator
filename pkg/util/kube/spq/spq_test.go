// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package spq_test

import (
	"context"
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	spqv1 "github.com/vmware-tanzu/vm-operator/external/storage-policy-quota/api/v1alpha2"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	spqutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube/spq"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = DescribeTable("StoragePolicyUsageName",
	func(s string, expected string) {
		Ω(spqutil.StoragePolicyUsageName(s)).Should(Equal(expected))
	},
	Entry("empty input", "", "-vm-usage"),
	Entry("non-empty-input", "my-class", "my-class-vm-usage"),
)

var _ = Describe("IsStorageClassInNamespace", func() {
	var (
		ctx         context.Context
		client      ctrlclient.Client
		withObjects []ctrlclient.Object
		withFuncs   interceptor.Funcs
		sc          *storagev1.StorageClass
		namespace   string
		name        string
		ok          bool
		err         error
	)

	BeforeEach(func() {
		ctx = pkgcfg.NewContext()
		namespace = defaultNamespace
		name = myStorageClass
		withFuncs = interceptor.Funcs{}
		withObjects = []ctrlclient.Object{
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			},
		}
		sc = &storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Parameters: map[string]string{
				"storagePolicyID": sc1PolicyID,
			},
		}
	})

	JustBeforeEach(func() {
		client = builder.NewFakeClientWithInterceptors(
			withFuncs, withObjects...)
		ok, err = spqutil.IsStorageClassInNamespace(
			ctx, client, sc, namespace)
	})

	When("FSS_PODVMONSTRETCHEDSUPERVISOR is disabled", func() {
		BeforeEach(func() {
			pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
				config.Features.PodVMOnStretchedSupervisor = false
			})
		})
		When("listing resource quotas returns an error", func() {
			BeforeEach(func() {
				withFuncs.List = func(
					ctx context.Context,
					client ctrlclient.WithWatch,
					list ctrlclient.ObjectList,
					opts ...ctrlclient.ListOption) error {

					if _, ok := list.(*corev1.ResourceQuotaList); ok {
						return errors.New(fakeString)
					}

					return client.List(ctx, list, opts...)
				}
			})
			It("should return an error", func() {
				Expect(err).To(MatchError(fakeString))
				Expect(ok).To(BeFalse())
			})
		})
		When("there is a match", func() {
			BeforeEach(func() {
				withObjects = append(
					withObjects,
					&corev1.ResourceQuota{
						ObjectMeta: metav1.ObjectMeta{
							Namespace:    namespace,
							GenerateName: "fake-",
						},
						Spec: corev1.ResourceQuotaSpec{
							Hard: corev1.ResourceList{
								corev1.ResourceName(name + ".storageclass.storage.k8s.io/fake"): resource.MustParse("10Gi"),
							},
						},
					},
				)
			})
			It("should return true", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(ok).To(BeTrue())
			})
		})
		When("there is not a match", func() {
			It("should return false", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(ok).To(BeFalse())
			})
		})
	})

	When("FSS_PODVMONSTRETCHEDSUPERVISOR is enabled", func() {
		BeforeEach(func() {
			pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
				config.Features.PodVMOnStretchedSupervisor = true
			})
		})
		When("listing storage classes returns an error", func() {
			BeforeEach(func() {
				withFuncs.List = func(
					ctx context.Context,
					client ctrlclient.WithWatch,
					list ctrlclient.ObjectList,
					opts ...ctrlclient.ListOption) error {

					if _, ok := list.(*spqv1.StoragePolicyQuotaList); ok {
						return errors.New(fakeString)
					}

					return client.List(ctx, list, opts...)
				}
			})
			It("should return an error", func() {
				Expect(err).To(MatchError(fakeString))
				Expect(ok).To(BeFalse())
			})
		})
		When("there is a match", func() {
			BeforeEach(func() {
				withObjects = append(
					withObjects,
					&spqv1.StoragePolicyQuota{
						ObjectMeta: metav1.ObjectMeta{
							Namespace:    namespace,
							GenerateName: "fake-",
						},
						Spec: spqv1.StoragePolicyQuotaSpec{
							StoragePolicyId: sc1PolicyID,
						},
					},
				)
			})
			It("should return true", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(ok).To(BeTrue())
			})
		})
		When("there is not a match", func() {
			It("should return false", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(ok).To(BeFalse())
			})
		})
	})
})

var _ = Describe("GetStorageClassInNamespace", func() {
	var (
		ctx         context.Context
		client      ctrlclient.Client
		withObjects []ctrlclient.Object
		withFuncs   interceptor.Funcs
		namespace   string
		name        string
		inName      string
		obj         storagev1.StorageClass
		err         error
	)

	BeforeEach(func() {
		ctx = pkgcfg.NewContext()
		namespace = defaultNamespace
		name = myStorageClass
		inName = name
		withFuncs = interceptor.Funcs{}
		withObjects = []ctrlclient.Object{
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			},
			&storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: name,
				},
				Parameters: map[string]string{
					"storagePolicyID": sc1PolicyID,
				},
			},
		}
	})

	JustBeforeEach(func() {
		client = builder.NewFakeClientWithInterceptors(
			withFuncs, withObjects...)

		obj, err = spqutil.GetStorageClassInNamespace(
			ctx, client, namespace, inName)
	})

	When("storage class does not exist", func() {
		BeforeEach(func() {
			inName = fakeString
		})
		It("should return NotFound", func() {
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
			Expect(obj).To(BeZero())
		})
	})

	When("storage class does exist", func() {
		When("FSS_PODVMONSTRETCHEDSUPERVISOR is enabled", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.PodVMOnStretchedSupervisor = true
				})
			})
			When("listing storage policy quotas returns an error", func() {
				BeforeEach(func() {
					withFuncs.List = func(
						ctx context.Context,
						client ctrlclient.WithWatch,
						list ctrlclient.ObjectList,
						opts ...ctrlclient.ListOption) error {

						if _, ok := list.(*spqv1.StoragePolicyQuotaList); ok {
							return errors.New(fakeString)
						}

						return client.List(ctx, list, opts...)
					}
				})
				It("should return an error", func() {
					Expect(err).To(MatchError(fakeString))
					Expect(obj).To(BeZero())
				})
			})
			When("there is a match", func() {
				BeforeEach(func() {
					withObjects = append(
						withObjects,
						&spqv1.StoragePolicyQuota{
							ObjectMeta: metav1.ObjectMeta{
								Namespace:    namespace,
								GenerateName: "fake-",
							},
							Spec: spqv1.StoragePolicyQuotaSpec{
								StoragePolicyId: sc1PolicyID,
							},
						},
					)
				})
				It("should return the storage class", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(obj.Name).To(Equal(name))
				})
			})
			When("there is not a match", func() {
				It("should return NotFoundInNamespace", func() {
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(spqutil.NotFoundInNamespace{Namespace: namespace, StorageClass: name}))
					Expect(obj).To(BeZero())
				})
			})
		})
	})
})

var _ = Describe("NotFoundInNamespace", func() {
	err := spqutil.NotFoundInNamespace{
		Namespace:    fakeString,
		StorageClass: myStorageClass,
	}
	Context("Error", func() {
		It("should return the expected string", func() {
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("Storage policy is not associated with the namespace fake"))
		})
	})
	Context("String", func() {
		It("should return the expected string", func() {
			Expect(err).To(HaveOccurred())
			Expect(err.String()).To(Equal("Storage policy is not associated with the namespace fake"))
		})
	})
})

var _ = Describe("GetStorageClassesForPolicyQuota", func() {
	var (
		ctx         context.Context
		client      ctrlclient.Client
		withObjects []ctrlclient.Object
		withFuncs   interceptor.Funcs
		namespace   string
		name        string
		ok          bool
		err         error
		sqp         *spqv1.StoragePolicyQuota
		sc          []storagev1.StorageClass
	)

	BeforeEach(func() {
		ctx = pkgcfg.NewContext()
		namespace = defaultNamespace
		name = myStorageClass
		withFuncs = interceptor.Funcs{}
		withObjects = []ctrlclient.Object{
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			},
			&storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: name,
				},
				Parameters: map[string]string{
					"storagePolicyID": sc1PolicyID,
				},
			},
			&storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: name + "-other",
				},
				Parameters: map[string]string{
					"storagePolicyID": sc1PolicyID + "-other",
				},
			},
		}

		sqp = &spqv1.StoragePolicyQuota{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      name + "-storagepolicyquota",
			},
			Spec: spqv1.StoragePolicyQuotaSpec{
				StoragePolicyId: sc1PolicyID,
			},
		}
	})

	JustBeforeEach(func() {
		client = builder.NewFakeClientWithInterceptors(withFuncs, withObjects...)
		sc, err = spqutil.GetStorageClassesForPolicyQuota(ctx, client, sqp)
	})

	When("listing storage classes returns an error", func() {
		BeforeEach(func() {
			withFuncs.List = func(
				ctx context.Context,
				client ctrlclient.WithWatch,
				list ctrlclient.ObjectList,
				opts ...ctrlclient.ListOption) error {

				if _, ok := list.(*storagev1.StorageClassList); ok {
					return errors.New(fakeString)
				}

				return client.List(ctx, list, opts...)
			}
		})

		It("should return an error", func() {
			Expect(err).To(MatchError(fakeString))
			Expect(ok).To(BeFalse())
			Expect(sc).To(BeEmpty())
		})
	})

	When("storage class is not part of policy quota", func() {
		BeforeEach(func() {
			sqp.Spec.StoragePolicyId = "some-other-policy-id"
		})

		It("should not return storage class", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(sc).To(BeEmpty())
		})
	})

	When("storage class is a part of policy quota", func() {
		It("should return storage class", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(sc).To(HaveLen(1))
			Expect(sc[0].Name).To(Equal(name))
		})
	})
})

var _ = Describe("GetStoragePolicyIDFromClass", func() {
	var (
		ctx         context.Context
		client      ctrlclient.Client
		withObjects []ctrlclient.Object
		namespace   string
		name        string
		inName      string
		id          string
		err         error
	)

	BeforeEach(func() {
		ctx = pkgcfg.NewContext()
		namespace = defaultNamespace
		name = myStorageClass
		inName = name
		withObjects = []ctrlclient.Object{
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			},
			&storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: name,
				},
				Parameters: map[string]string{
					"storagePolicyID": sc1PolicyID,
				},
			},
		}
	})

	JustBeforeEach(func() {
		client = builder.NewFakeClient(withObjects...)

		id, err = spqutil.GetStoragePolicyIDFromClass(
			ctx, client, inName)
	})

	When("storage class does not exist", func() {
		BeforeEach(func() {
			inName = fakeString
		})
		It("should return NotFound", func() {
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
			Expect(id).To(BeEmpty())
		})
	})

	When("storage class does exist", func() {
		It("should return the expected id", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(id).To(Equal(sc1PolicyID))
		})
	})
})

var _ = Describe("GetStorageClassesForPolicy", func() {
	var (
		ctx         context.Context
		client      ctrlclient.Client
		withObjects []ctrlclient.Object
		withFuncs   interceptor.Funcs
		namespace   string
		inPolicyID  string
		sc1Name     string
		sc2Name     string
		obj         []storagev1.StorageClass
		err         error
	)

	BeforeEach(func() {
		ctx = pkgcfg.NewContext()
		namespace = defaultNamespace
		inPolicyID = sc1PolicyID
		sc1Name = "my-storage-class-1"
		sc2Name = "my-storage-class-2"
		withFuncs = interceptor.Funcs{}
		withObjects = []ctrlclient.Object{
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			},
		}
	})

	JustBeforeEach(func() {
		withObjects = append(
			withObjects,
			&storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: sc1Name,
				},
				Parameters: map[string]string{
					"storagePolicyID": sc1PolicyID,
				},
			},
			&storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: sc2Name,
				},
				Parameters: map[string]string{
					"storagePolicyID": sc2PolicyID,
				},
			},
		)

		client = builder.NewFakeClientWithInterceptors(
			withFuncs, withObjects...)

		obj, err = spqutil.GetStorageClassesForPolicy(
			ctx, client, namespace, inPolicyID)
	})

	When("policy id does not exist", func() {
		BeforeEach(func() {
			inPolicyID = fakeString
		})
		It("should return an empty list", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(obj).To(BeEmpty())
		})
	})

	When("storage classes are available", func() {
		When("FSS_PODVMONSTRETCHEDSUPERVISOR is enabled", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.PodVMOnStretchedSupervisor = true
				})
			})
			When("listing storage classes returns an error", func() {
				BeforeEach(func() {
					withFuncs.List = func(
						ctx context.Context,
						client ctrlclient.WithWatch,
						list ctrlclient.ObjectList,
						opts ...ctrlclient.ListOption) error {

						if _, ok := list.(*storagev1.StorageClassList); ok {
							return errors.New(fakeString)
						}

						return client.List(ctx, list, opts...)
					}
				})
				It("should return an error", func() {
					Expect(err).To(MatchError(fakeString))
					Expect(obj).To(BeZero())
				})
			})
			When("listing storage policy quotas returns an error", func() {
				BeforeEach(func() {
					withFuncs.List = func(
						ctx context.Context,
						client ctrlclient.WithWatch,
						list ctrlclient.ObjectList,
						opts ...ctrlclient.ListOption) error {

						if _, ok := list.(*spqv1.StoragePolicyQuotaList); ok {
							return errors.New(fakeString)
						}

						return client.List(ctx, list, opts...)
					}
				})
				It("should return an error", func() {
					Expect(err).To(MatchError(fakeString))
					Expect(obj).To(BeZero())
				})
			})
			When("there is a match", func() {
				BeforeEach(func() {
					withObjects = append(
						withObjects,
						&spqv1.StoragePolicyQuota{
							ObjectMeta: metav1.ObjectMeta{
								Namespace:    namespace,
								GenerateName: "fake-",
							},
							Spec: spqv1.StoragePolicyQuotaSpec{
								StoragePolicyId: sc1PolicyID,
							},
						},
					)
				})
				It("should return the storage class", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(obj).To(HaveLen(1))
					Expect(obj[0].Name).To(Equal(sc1Name))
				})
			})
			When("there are multiple matches", func() {
				const newPolicyID = "my-new-policy-id"

				BeforeEach(func() {
					inPolicyID = newPolicyID

					withObjects = append(
						withObjects,
						&storagev1.StorageClass{
							ObjectMeta: metav1.ObjectMeta{
								Name: sc1Name + "-new",
							},
							Parameters: map[string]string{
								"storagePolicyID": newPolicyID,
							},
						},
						&storagev1.StorageClass{
							ObjectMeta: metav1.ObjectMeta{
								Name: sc2Name + "-new",
							},
							Parameters: map[string]string{
								"storagePolicyID": newPolicyID,
							},
						},
						&spqv1.StoragePolicyQuota{
							ObjectMeta: metav1.ObjectMeta{
								Namespace:    namespace,
								GenerateName: "fake-",
							},
							Spec: spqv1.StoragePolicyQuotaSpec{
								StoragePolicyId: newPolicyID,
							},
						},
					)
				})
				It("should return the storage classes", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(obj).To(HaveLen(2))
					Expect(obj[0].Name).To(Equal(sc1Name + "-new"))
					Expect(obj[1].Name).To(Equal(sc2Name + "-new"))
				})
			})
			When("there is not a match", func() {
				It("should return an empty list", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(obj).To(BeEmpty())
				})
			})
		})
	})
})

var _ = Describe("GetWebhookCABundle", func() {
	var (
		ctx         context.Context
		client      ctrlclient.Client
		withObjects []ctrlclient.Object
		withFuncs   interceptor.Funcs
		namespace   string
		config      *admissionv1.ValidatingWebhookConfiguration
		secret      *corev1.Secret
		caBundle    []byte
		err         error
	)

	BeforeEach(func() {
		ctx = pkgcfg.NewContext()
		namespace = defaultNamespace
		withFuncs = interceptor.Funcs{}
		withObjects = []ctrlclient.Object{
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			},
		}

		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vmware-system-vmop-serving-cert",
				Namespace: namespace,
			},
			Data: map[string][]byte{
				"ca.crt": []byte("fake-ca-bundle"),
			},
		}

		config = &admissionv1.ValidatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name: spqutil.ValidatingWebhookConfigName,
				Annotations: map[string]string{
					"cert-manager.io/inject-ca-from": fmt.Sprintf("%s/vmware-system-vmop-serving-cert", namespace),
				},
			},
			Webhooks: []admissionv1.ValidatingWebhook{},
		}
	})

	JustBeforeEach(func() {
		withObjects = append(withObjects, config, secret)

		client = builder.NewFakeClientWithInterceptors(withFuncs, withObjects...)

		caBundle, err = spqutil.GetWebhookCABundle(ctx, client)
	})

	When("webhooks exists and contains CA Bundle", func() {
		BeforeEach(func() {
			config.Webhooks = []admissionv1.ValidatingWebhook{
				{
					ClientConfig: admissionv1.WebhookClientConfig{
						CABundle: []byte("fake-ca-bundle"),
					},
				},
			}
		})

		It("should return the CA bundle", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(caBundle).To(Equal(config.Webhooks[0].ClientConfig.CABundle))
		})
	})

	When("webhooks does not exist", func() {
		It("should return the CA bundle from the secret", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(caBundle).To(Equal(secret.Data["ca.crt"]))
		})
	})

	When("ValidatingWebhookConfiguration is not found", func() {
		errMsg := `validatingwebhookconfigurations.admissionregistration.k8s.io "vmware-system-vmop-validating-webhook-configuration" not found`

		BeforeEach(func() {
			config = &admissionv1.ValidatingWebhookConfiguration{}
		})

		It("should return an error", func() {
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(errMsg))
		})
	})

	When("certificate annotation is not present", func() {
		BeforeEach(func() {
			delete(config.Annotations, "cert-manager.io/inject-ca-from")
		})

		It("should return an error", func() {
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unable to get annotation for key"))
		})
	})

	When("certificate annotation is present with empty value", func() {
		BeforeEach(func() {
			config.Annotations["cert-manager.io/inject-ca-from"] = ""
		})

		It("should return an error", func() {
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unable to get annotation for key"))
		})
	})

	When("certificate annotation is present and missing namespace", func() {
		BeforeEach(func() {
			config.Annotations["cert-manager.io/inject-ca-from"] = "vmware-system-vmop-serving-cert"
		})

		It("should not return an error", func() {
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unable to get namespace and name for key"))
		})
	})

	When("certificate Secret is not found", func() {
		BeforeEach(func() {
			secret.Name = "fake"
		})

		It("should return an error", func() {
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(`secrets "vmware-system-vmop-serving-cert" not found`))
		})
	})

	When("secret is missing ca.crt", func() {
		BeforeEach(func() {
			delete(secret.Data, "ca.crt")
		})

		It("should return an error", func() {
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unable to get CA bundle from secret"))
		})
	})

	When("ca.crt is empty", func() {
		BeforeEach(func() {
			secret.Data["ca.crt"] = []byte("")
		})

		It("should return an error", func() {
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unable to get CA bundle from secret"))
		})
	})
})
