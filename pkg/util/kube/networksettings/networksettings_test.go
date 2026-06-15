// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package networksettings_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	netopv1alpha1 "github.com/vmware-tanzu/net-operator-api/api/v1alpha1"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	netsetutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube/networksettings"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("GetProviderType", func() {
	const namespace = "test-ns"

	var (
		ctx         context.Context
		reader      ctrlclient.Reader
		withObjects []ctrlclient.Object
		withFuncs   interceptor.Funcs
		result      pkgcfg.NetworkProviderType
		err         error
	)

	BeforeEach(func() {
		ctx = pkgcfg.NewContext()
		withFuncs = interceptor.Funcs{}
		withObjects = nil
	})

	JustBeforeEach(func() {
		reader = builder.NewFakeClientWithInterceptors(withFuncs, withObjects...)
		result, err = netsetutil.GetProviderType(ctx, reader, namespace)
	})

	When("PerNamespaceNetworkProvider capability is disabled", func() {
		BeforeEach(func() {
			pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
				config.Features.PerNamespaceNetworkProvider = false
				config.NetworkProviderType = pkgcfg.NetworkProviderTypeVDS
			})
		})

		It("returns the global network provider config value", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(pkgcfg.NetworkProviderTypeVDS))
		})
	})

	When("PerNamespaceNetworkProvider capability is enabled", func() {
		BeforeEach(func() {
			pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
				config.Features.PerNamespaceNetworkProvider = true
			})
		})

		When("NetworkSettings/default does not exist", func() {
			It("returns a not-found error", func() {
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(netsetutil.ErrNetworkSettingsNotFound))
				Expect(result).To(BeEmpty())
			})
		})

		When("the client returns an unexpected error", func() {
			BeforeEach(func() {
				withFuncs.Get = func(
					ctx context.Context,
					client ctrlclient.WithWatch,
					key ctrlclient.ObjectKey,
					obj ctrlclient.Object,
					opts ...ctrlclient.GetOption) error {

					if _, ok := obj.(*netopv1alpha1.NetworkSettings); ok {
						return apierrors.NewServiceUnavailable("fake error")
					}
					return client.Get(ctx, key, obj, opts...)
				}
			})

			It("returns error", func() {
				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsServiceUnavailable(err)).To(BeTrue())
				Expect(result).To(BeEmpty())
			})
		})

		When("NetworkSettings/default has provider vsphere-distributed", func() {
			BeforeEach(func() {
				withObjects = append(withObjects, &netopv1alpha1.NetworkSettings{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "default",
						Namespace: namespace,
					},
					Provider: netopv1alpha1.NetworkProviderVSphereDistributed,
				})
			})

			It("returns NetworkProviderTypeVDS", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(pkgcfg.NetworkProviderTypeVDS))
			})
		})

		When("NetworkSettings/default has provider nsx-tier1", func() {
			BeforeEach(func() {
				withObjects = append(withObjects, &netopv1alpha1.NetworkSettings{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "default",
						Namespace: namespace,
					},
					Provider: netopv1alpha1.NetworkProviderNSXTier1,
				})
			})

			It("returns NetworkProviderTypeNSXT", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(pkgcfg.NetworkProviderTypeNSXT))
			})
		})

		When("NetworkSettings/default has provider vpc", func() {
			BeforeEach(func() {
				withObjects = append(withObjects, &netopv1alpha1.NetworkSettings{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "default",
						Namespace: namespace,
					},
					Provider: netopv1alpha1.NetworkProviderVPC,
				})
			})

			It("returns NetworkProviderTypeVPC", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(pkgcfg.NetworkProviderTypeVPC))
			})
		})

		When("NetworkSettings/default has an unknown provider value", func() {
			BeforeEach(func() {
				withObjects = append(withObjects, &netopv1alpha1.NetworkSettings{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "default",
						Namespace: namespace,
					},
					Provider: "unknown-provider",
				})
			})

			It("returns an error", func() {
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(ContainSubstring("unknown network provider")))
			})
		})
	})
})

var _ = Describe("GetSupportedProviderTypes", func() {
	const namespace = "test-ns"

	var (
		ctx         context.Context
		reader      ctrlclient.Reader
		withObjects []ctrlclient.Object
		withFuncs   interceptor.Funcs
		result      []pkgcfg.NetworkProviderType
		err         error
	)

	BeforeEach(func() {
		ctx = pkgcfg.NewContext()
		withFuncs = interceptor.Funcs{}
		withObjects = nil
	})

	JustBeforeEach(func() {
		reader = builder.NewFakeClientWithInterceptors(withFuncs, withObjects...)
		result, err = netsetutil.GetSupportedProviderTypes(ctx, reader, namespace)
	})

	When("PerNamespaceNetworkProvider capability is disabled", func() {
		BeforeEach(func() {
			pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
				config.Features.PerNamespaceNetworkProvider = false
				config.NetworkProviderType = pkgcfg.NetworkProviderTypeVDS
			})
		})

		It("returns a single-element slice with the global network provider config value", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(ConsistOf(pkgcfg.NetworkProviderTypeVDS))
		})
	})

	When("PerNamespaceNetworkProvider capability is enabled", func() {
		BeforeEach(func() {
			pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
				config.Features.PerNamespaceNetworkProvider = true
			})
		})

		When("NetworkSettings/default does not exist", func() {
			It("returns a not-found error", func() {
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(netsetutil.ErrNetworkSettingsNotFound))
				Expect(result).To(BeNil())
			})
		})

		When("the client returns an unexpected error", func() {
			BeforeEach(func() {
				withFuncs.Get = func(
					ctx context.Context,
					client ctrlclient.WithWatch,
					key ctrlclient.ObjectKey,
					obj ctrlclient.Object,
					opts ...ctrlclient.GetOption) error {

					if _, ok := obj.(*netopv1alpha1.NetworkSettings); ok {
						return apierrors.NewServiceUnavailable("fake error")
					}
					return client.Get(ctx, key, obj, opts...)
				}
			})

			It("propagates the error", func() {
				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsNotFound(err)).To(BeFalse())
				Expect(result).To(BeNil())
			})
		})

		When("NetworkSettings/default has provider vsphere-distributed and no legacy provider", func() {
			BeforeEach(func() {
				withObjects = append(withObjects, &netopv1alpha1.NetworkSettings{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "default",
						Namespace: namespace,
					},
					Provider: netopv1alpha1.NetworkProviderVSphereDistributed,
				})
			})

			It("returns a single-element slice with NetworkProviderTypeVDS", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(ConsistOf(pkgcfg.NetworkProviderTypeVDS))
			})
		})

		When("NetworkSettings/default has provider nsx-tier1 and no legacy provider", func() {
			BeforeEach(func() {
				withObjects = append(withObjects, &netopv1alpha1.NetworkSettings{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "default",
						Namespace: namespace,
					},
					Provider: netopv1alpha1.NetworkProviderNSXTier1,
				})
			})

			It("returns a single-element slice with NetworkProviderTypeNSXT", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(ConsistOf(pkgcfg.NetworkProviderTypeNSXT))
			})
		})

		When("NetworkSettings/default has provider vpc and no legacy provider", func() {
			BeforeEach(func() {
				withObjects = append(withObjects, &netopv1alpha1.NetworkSettings{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "default",
						Namespace: namespace,
					},
					Provider: netopv1alpha1.NetworkProviderVPC,
				})
			})

			It("returns a single-element slice with NetworkProviderTypeVPC", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(ConsistOf(pkgcfg.NetworkProviderTypeVPC))
			})
		})

		When("NetworkSettings/default has provider vpc and legacy provider vsphere-distributed", func() {
			BeforeEach(func() {
				withObjects = append(withObjects, &netopv1alpha1.NetworkSettings{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "default",
						Namespace: namespace,
					},
					Provider:       netopv1alpha1.NetworkProviderVPC,
					LegacyProvider: netopv1alpha1.NetworkProviderVSphereDistributed,
				})
			})

			It("returns NetworkProviderTypeVPC and NetworkProviderTypeVDS", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(ConsistOf(
					pkgcfg.NetworkProviderTypeVPC,
					pkgcfg.NetworkProviderTypeVDS,
				))
			})
		})

		When("NetworkSettings/default has provider vpc and legacy provider nsx-tier1", func() {
			BeforeEach(func() {
				withObjects = append(withObjects, &netopv1alpha1.NetworkSettings{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "default",
						Namespace: namespace,
					},
					Provider:       netopv1alpha1.NetworkProviderVPC,
					LegacyProvider: netopv1alpha1.NetworkProviderNSXTier1,
				})
			})

			It("returns NetworkProviderTypeVPC and NetworkProviderTypeNSXT", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(ConsistOf(
					pkgcfg.NetworkProviderTypeVPC,
					pkgcfg.NetworkProviderTypeNSXT,
				))
			})
		})

		When("NetworkSettings/default has an unknown provider value", func() {
			BeforeEach(func() {
				withObjects = append(withObjects, &netopv1alpha1.NetworkSettings{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "default",
						Namespace: namespace,
					},
					Provider: "unknown-provider",
				})
			})

			It("returns an error", func() {
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(ContainSubstring("unknown network provider")))
				Expect(result).To(BeNil())
			})
		})

		When("NetworkSettings/default has a valid provider and an unknown legacy provider value", func() {
			BeforeEach(func() {
				withObjects = append(withObjects, &netopv1alpha1.NetworkSettings{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "default",
						Namespace: namespace,
					},
					Provider:       netopv1alpha1.NetworkProviderVPC,
					LegacyProvider: "unknown-legacy-provider",
				})
			})

			It("returns an error", func() {
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(ContainSubstring("unknown network provider")))
				Expect(result).To(BeNil())
			})
		})
	})
})
