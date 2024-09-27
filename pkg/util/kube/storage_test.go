// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package kube_test

import (
	"context"
	"errors"
	"strconv"
	"sync"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

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
		`StorageClass "my-storage-class" does not have 'storagePolicyID' parameter`),
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

		ok, err = kubeutil.IsEncryptedStorageClass(
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
			ok, err := kubeutil.IsEncryptedStorageClass(ctx, client, storageClass.Name)
			Expect(err).ToNot(HaveOccurred())
			Expect(ok).To(BeTrue())
		})
		When("the storage class is marked as encrypted again", func() {
			JustBeforeEach(func() {
				Expect(kubeutil.MarkEncryptedStorageClass(ctx, client, storageClass, true)).To(Succeed())
			})
			It("should return true", func() {
				ok, err := kubeutil.IsEncryptedStorageClass(ctx, client, storageClass.Name)
				Expect(err).ToNot(HaveOccurred())
				Expect(ok).To(BeTrue())
			})
		})
		When("the storage class is marked as unencrypted", func() {
			JustBeforeEach(func() {
				Expect(kubeutil.MarkEncryptedStorageClass(ctx, client, storageClass, false)).To(Succeed())
			})
			It("should return false", func() {
				ok, err := kubeutil.IsEncryptedStorageClass(ctx, client, storageClass.Name)
				Expect(err).ToNot(HaveOccurred())
				Expect(ok).To(BeFalse())
			})

			When("the storage class is marked as encrypted again", func() {
				JustBeforeEach(func() {
					Expect(kubeutil.MarkEncryptedStorageClass(ctx, client, storageClass, true)).To(Succeed())
				})
				It("should return true", func() {
					ok, err := kubeutil.IsEncryptedStorageClass(ctx, client, storageClass.Name)
					Expect(err).ToNot(HaveOccurred())
					Expect(ok).To(BeTrue())
				})
			})
		})

		When("a second storage class is marked as not encrypted", func() {
			JustBeforeEach(func() {
				storageClass.Name += "1"
				Expect(kubeutil.MarkEncryptedStorageClass(ctx, client, storageClass, false)).To(Succeed())
			})
			It("should return false for the second storage class", func() {
				ok, err := kubeutil.IsEncryptedStorageClass(ctx, client, storageClass.Name)
				Expect(err).ToNot(HaveOccurred())
				Expect(ok).To(BeFalse())
			})
		})
	})

	When("the storage class is not marked as not encrypted", func() {
		JustBeforeEach(func() {
			Expect(kubeutil.MarkEncryptedStorageClass(ctx, client, storageClass, false)).To(Succeed())
		})
		It("should return false", func() {
			ok, err := kubeutil.IsEncryptedStorageClass(ctx, client, storageClass.Name)
			Expect(err).ToNot(HaveOccurred())
			Expect(ok).To(BeFalse())
		})
	})

	When("the storage class is not marked at all", func() {
		It("should return false", func() {
			ok, err := kubeutil.IsEncryptedStorageClass(ctx, client, storageClass.Name)
			Expect(err).ToNot(HaveOccurred())
			Expect(ok).To(BeFalse())
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
			ok, err := kubeutil.IsEncryptedStorageClass(ctx, client, storageClass.Name)
			Expect(err).To(MatchError(apierrors.NewInternalError(errors.New(fakeString)).Error()))
			Expect(ok).To(BeFalse())
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
					ok, err := kubeutil.IsEncryptedStorageClass(
						ctx, client, storageClasses[i].Name)
					Expect(err).ToNot(HaveOccurred())
					Expect(ok).To(BeTrue())
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
					ok, err := kubeutil.IsEncryptedStorageClass(
						ctx, client, storageClasses[i].Name)
					Expect(err).ToNot(HaveOccurred())
					Expect(ok).To(BeFalse())
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
					ok, err := kubeutil.IsEncryptedStorageClass(
						ctx, client, storageClasses[i].Name)
					Expect(err).ToNot(HaveOccurred())
					if i%2 == 0 {
						Expect(ok).To(BeFalse())
					} else {
						Expect(ok).To(BeTrue())
					}
				}
			})
		})
	})
})
