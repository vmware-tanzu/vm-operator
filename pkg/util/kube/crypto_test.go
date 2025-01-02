// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package kube_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	byokv1 "github.com/vmware-tanzu/vm-operator/external/byok/api/v1alpha1"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("GetDefaultEncryptionClassForNamespace", func() {
	const (
		name1      = "my-encryption-class-1"
		name2      = "my-encryption-class-2"
		namespace1 = "my-namespace-1"
		namespace2 = "my-namespace-2"
		namespace3 = "my-namespace-3"
	)
	var (
		ctx               context.Context
		k8sClient         ctrlclient.Client
		namespace         string
		withFuncs         interceptor.Funcs
		withObjs          []ctrlclient.Object
		labelsWithDefault map[string]string
	)
	BeforeEach(func() {
		ctx = context.Background()
		withObjs = nil
		withFuncs = interceptor.Funcs{}
		namespace = namespace3
		labelsWithDefault = map[string]string{
			kubeutil.DefaultEncryptionClassLabelName: kubeutil.DefaultEncryptionClassLabelValue,
		}
	})
	JustBeforeEach(func() {
		k8sClient = builder.NewFakeClientWithInterceptors(
			withFuncs, withObjs...)
	})
	Context("panic expected", func() {
		When("context is nil", func() {
			JustBeforeEach(func() {
				ctx = nil
			})
			It("should panic", func() {
				Expect(func() {
					_, _ = kubeutil.GetDefaultEncryptionClassForNamespace(
						ctx,
						k8sClient,
						namespace)
				}).To(PanicWith("context is nil"))
			})
		})
		When("k8sClient is nil", func() {
			JustBeforeEach(func() {
				k8sClient = nil
			})
			It("should panic", func() {
				Expect(func() {
					_, _ = kubeutil.GetDefaultEncryptionClassForNamespace(
						ctx,
						k8sClient,
						namespace)
				}).To(PanicWith("k8sClient is nil"))
			})
		})
		When("namespace is empty", func() {
			JustBeforeEach(func() {
				namespace = ""
			})
			It("should panic", func() {
				Expect(func() {
					_, _ = kubeutil.GetDefaultEncryptionClassForNamespace(
						ctx,
						k8sClient,
						namespace)
				}).To(PanicWith("namespace is empty"))
			})
		})
	})

	Context("panic not expected", func() {
		var (
			err      error
			obj      byokv1.EncryptionClass
			emptyObj byokv1.EncryptionClass
		)
		JustBeforeEach(func() {
			obj, err = kubeutil.GetDefaultEncryptionClassForNamespace(
				ctx, k8sClient, namespace)
		})
		When("there are no EncryptionClasses", func() {
			It("should not return an EncryptionClass", func() {
				Expect(err).To(MatchError(kubeutil.ErrNoDefaultEncryptionClass))
				Expect(obj).To(Equal(emptyObj))
			})
		})
		When("there are no EncryptionClasses in the given namespace", func() {
			BeforeEach(func() {
				withObjs = append(withObjs,
					&byokv1.EncryptionClass{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespace1,
							Name:      name1,
						},
					},
					&byokv1.EncryptionClass{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespace2,
							Name:      name1,
						},
					},
					&byokv1.EncryptionClass{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespace2,
							Name:      name2,
						},
					},
				)
			})
			It("should not return an EncryptionClass", func() {
				Expect(err).To(MatchError(kubeutil.ErrNoDefaultEncryptionClass))
				Expect(obj).To(Equal(emptyObj))
			})
		})
		When("there is a default EncryptionClass in a different namespace", func() {
			BeforeEach(func() {
				withObjs = append(withObjs,
					&byokv1.EncryptionClass{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespace1,
							Name:      name1,
							Labels:    labelsWithDefault,
						},
					},
					&byokv1.EncryptionClass{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespace2,
							Name:      name1,
						},
					},
					&byokv1.EncryptionClass{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespace2,
							Name:      name2,
							Labels:    labelsWithDefault,
						},
					},
					&byokv1.EncryptionClass{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespace,
							Name:      name1,
						},
					},
				)
			})
			It("should not return an EncryptionClass", func() {
				Expect(err).To(MatchError(kubeutil.ErrNoDefaultEncryptionClass))
				Expect(obj).To(Equal(emptyObj))
			})
		})
		When("there is a default EncryptionClass", func() {
			BeforeEach(func() {
				withObjs = append(withObjs,
					&byokv1.EncryptionClass{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespace1,
							Name:      name1,
							Labels:    labelsWithDefault,
						},
					},
					&byokv1.EncryptionClass{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespace2,
							Name:      name1,
						},
					},
					&byokv1.EncryptionClass{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespace2,
							Name:      name2,
							Labels:    labelsWithDefault,
						},
					},
					&byokv1.EncryptionClass{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespace,
							Name:      name1,
							Labels:    labelsWithDefault,
						},
					},
				)
			})
			It("should return an EncryptionClass", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(obj.Namespace).To(Equal(namespace))
				Expect(obj.Name).To(Equal(name1))
			})
		})

		When("there are multiple default EncryptionClasses in the same namespace", func() {
			BeforeEach(func() {
				withObjs = append(withObjs,
					&byokv1.EncryptionClass{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespace1,
							Name:      name1,
							Labels:    labelsWithDefault,
						},
					},
					&byokv1.EncryptionClass{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespace2,
							Name:      name1,
						},
					},
					&byokv1.EncryptionClass{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespace2,
							Name:      name2,
							Labels:    labelsWithDefault,
						},
					},
					&byokv1.EncryptionClass{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespace,
							Name:      name1,
							Labels:    labelsWithDefault,
						},
					},
					&byokv1.EncryptionClass{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespace,
							Name:      name2,
							Labels:    labelsWithDefault,
						},
					},
				)
			})
			It("should return ErrMultipleDefaultEncryptionClasses", func() {
				Expect(err).To(MatchError(kubeutil.ErrMultipleDefaultEncryptionClasses))
				Expect(obj).To(Equal(emptyObj))
			})
		})

		When("there is an error listing EncryptionClasses", func() {
			BeforeEach(func() {
				withFuncs.List = func(
					ctx context.Context,
					client ctrlclient.WithWatch,
					list ctrlclient.ObjectList,
					opts ...ctrlclient.ListOption) error {

					return errors.New(fakeString)
				}
			})
			It("should return an error", func() {
				Expect(err).To(MatchError(fakeString))
				Expect(obj).To(Equal(emptyObj))
			})
		})
	})
})
