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

	var (
		ctx         context.Context
		reader      ctrlclient.Reader
		withObjects []ctrlclient.Object
		withFuncs   interceptor.Funcs
		result      []pkgcfg.NetworkProviderType
		namespace   string
		err         error
	)

	BeforeEach(func() {
		ctx = pkgcfg.NewContext()
		namespace = "test-ns"
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

		Context("reserved supervisor namespace", func() {
			BeforeEach(func() {
				namespace = "vmware-system-supervisor"
			})

			Context("default provider is VDS", func() {
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

			Context("default provider is not VDS", func() {
				BeforeEach(func() {
					withObjects = append(withObjects, &netopv1alpha1.NetworkSettings{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "default",
							Namespace: namespace,
						},
						Provider: netopv1alpha1.NetworkProviderVPC,
					})
				})

				It("returns NetworkProviderTypeVPC and NetworkProviderTypeVDS", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(result).To(ConsistOf(pkgcfg.NetworkProviderTypeVPC, pkgcfg.NetworkProviderTypeVDS))
				})
			})
		})
	})
})

var _ = Describe("GetClusterSupportedProviderTypes", func() {
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
		result, err = netsetutil.GetClusterSupportedProviderTypes(ctx, reader)
	})

	When("WorkloadNetworkConfiguration capability is disabled", func() {
		BeforeEach(func() {
			pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
				config.Features.WorkloadNetworkConfiguration = false
				config.NetworkProviderType = pkgcfg.NetworkProviderTypeVPC
			})
		})

		It("returns a single-element slice with the global network provider config value", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(ConsistOf(pkgcfg.NetworkProviderTypeVPC))
		})
	})

	When("WorkloadNetworkConfiguration capability is enabled", func() {
		BeforeEach(func() {
			pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
				config.Features.WorkloadNetworkConfiguration = true
			})
		})

		When("WorkloadNetworkConfiguration/default does not exist", func() {
			It("returns a not-found error", func() {
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(netsetutil.ErrWorkloadNetworkConfigurationNotFound))
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

					if _, ok := obj.(*netopv1alpha1.WorkloadNetworkConfiguration); ok {
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

		When("WorkloadNetworkConfiguration has a single vsphere-distributed provider", func() {
			BeforeEach(func() {
				withObjects = append(withObjects, &netopv1alpha1.WorkloadNetworkConfiguration{
					ObjectMeta: metav1.ObjectMeta{
						Name: "default",
					},
					Spec: netopv1alpha1.WorkloadNetworkConfigurationSpec{
						Providers: []netopv1alpha1.NetworkProviderEntry{
							{
								Type: netopv1alpha1.NetworkProviderVSphereDistributed,
							},
						},
					},
				})
			})

			It("returns NetworkProviderTypeVDS", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(ConsistOf(pkgcfg.NetworkProviderTypeVDS))
			})
		})

		When("WorkloadNetworkConfiguration has a single nsx-tier1 provider", func() {
			BeforeEach(func() {
				withObjects = append(withObjects, &netopv1alpha1.WorkloadNetworkConfiguration{
					ObjectMeta: metav1.ObjectMeta{
						Name: "default",
					},
					Spec: netopv1alpha1.WorkloadNetworkConfigurationSpec{
						Providers: []netopv1alpha1.NetworkProviderEntry{
							{
								Type: netopv1alpha1.NetworkProviderNSXTier1,
							},
						},
					},
				})
			})

			It("returns NetworkProviderTypeNSXT", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(ConsistOf(pkgcfg.NetworkProviderTypeNSXT))
			})
		})

		When("WorkloadNetworkConfiguration has a single vpc provider", func() {
			BeforeEach(func() {
				withObjects = append(withObjects, &netopv1alpha1.WorkloadNetworkConfiguration{
					ObjectMeta: metav1.ObjectMeta{
						Name: "default",
					},
					Spec: netopv1alpha1.WorkloadNetworkConfigurationSpec{
						Providers: []netopv1alpha1.NetworkProviderEntry{
							{
								Type: netopv1alpha1.NetworkProviderVPC,
							},
						},
					},
				})
			})

			It("returns NetworkProviderTypeVPC", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(ConsistOf(pkgcfg.NetworkProviderTypeVPC))
			})
		})

		When("WorkloadNetworkConfiguration has multiple providers", func() {
			BeforeEach(func() {
				withObjects = append(withObjects, &netopv1alpha1.WorkloadNetworkConfiguration{
					ObjectMeta: metav1.ObjectMeta{
						Name: "default",
					},
					Spec: netopv1alpha1.WorkloadNetworkConfigurationSpec{
						Providers: []netopv1alpha1.NetworkProviderEntry{
							{
								Type: netopv1alpha1.NetworkProviderVPC,
							},
							{
								Type: netopv1alpha1.NetworkProviderVSphereDistributed,
							},
							{
								Type: netopv1alpha1.NetworkProviderNSXTier1,
							},
						},
					},
				})
			})

			It("returns all three provider types", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(ConsistOf(
					pkgcfg.NetworkProviderTypeVPC,
					pkgcfg.NetworkProviderTypeVDS,
					pkgcfg.NetworkProviderTypeNSXT,
				))
			})
		})

		When("WorkloadNetworkConfiguration has an unknown provider type", func() {
			BeforeEach(func() {
				withObjects = append(withObjects, &netopv1alpha1.WorkloadNetworkConfiguration{
					ObjectMeta: metav1.ObjectMeta{
						Name: "default",
					},
					Spec: netopv1alpha1.WorkloadNetworkConfigurationSpec{
						Providers: []netopv1alpha1.NetworkProviderEntry{
							{
								Type: netopv1alpha1.NetworkProviderVPC,
							},
							{
								Type: "unknown-provider",
							},
						},
					},
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

var _ = Describe("GetClusterSupportedProviderTypesFromConfig", func() {
	var (
		ctx    context.Context
		result []pkgcfg.NetworkProviderType
	)

	BeforeEach(func() {
		ctx = pkgcfg.NewContext()
	})

	JustBeforeEach(func() {
		result = netsetutil.GetClusterSupportedProviderTypesFromConfig(ctx)
	})

	When("ClusterNetworkProviderTypes is empty", func() {
		BeforeEach(func() {
			pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
				config.ClusterNetworkProviderTypes = ""
			})
		})

		It("returns nil", func() {
			Expect(result).To(BeNil())
		})
	})

	When("ClusterNetworkProviderTypes has a single provider type", func() {
		BeforeEach(func() {
			pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
				config.ClusterNetworkProviderTypes = string(pkgcfg.NetworkProviderTypeVDS)
			})
		})

		It("returns a single-element slice with that provider type", func() {
			Expect(result).To(ConsistOf(pkgcfg.NetworkProviderTypeVDS))
		})
	})

	When("ClusterNetworkProviderTypes has multiple provider types", func() {
		BeforeEach(func() {
			pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
				config.ClusterNetworkProviderTypes = pkgcfg.SliceToString([]string{
					string(pkgcfg.NetworkProviderTypeVPC),
					string(pkgcfg.NetworkProviderTypeVDS),
					string(pkgcfg.NetworkProviderTypeNSXT),
				})
			})
		})

		It("returns all provider types", func() {
			Expect(result).To(ConsistOf(
				pkgcfg.NetworkProviderTypeVPC,
				pkgcfg.NetworkProviderTypeVDS,
				pkgcfg.NetworkProviderTypeNSXT,
			))
		})
	})

	When("ClusterNetworkProviderTypes has an unknown provider type", func() {
		BeforeEach(func() {
			pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
				config.ClusterNetworkProviderTypes = "unknown-provider"
			})
		})

		It("returns the raw value without validating it", func() {
			Expect(result).To(ConsistOf(pkgcfg.NetworkProviderType("unknown-provider")))
		})
	})
})

var _ = Describe("NetworkProviderToType", func() {
	When("provider is vsphere-distributed", func() {
		It("returns NetworkProviderTypeVDS", func() {
			result, err := netsetutil.NetworkProviderToType(netopv1alpha1.NetworkProviderVSphereDistributed)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(pkgcfg.NetworkProviderTypeVDS))
		})
	})

	When("provider is nsx-tier1", func() {
		It("returns NetworkProviderTypeNSXT", func() {
			result, err := netsetutil.NetworkProviderToType(netopv1alpha1.NetworkProviderNSXTier1)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(pkgcfg.NetworkProviderTypeNSXT))
		})
	})

	When("provider is vpc", func() {
		It("returns NetworkProviderTypeVPC", func() {
			result, err := netsetutil.NetworkProviderToType(netopv1alpha1.NetworkProviderVPC)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(pkgcfg.NetworkProviderTypeVPC))
		})
	})

	When("provider is unknown", func() {
		It("returns an error", func() {
			result, err := netsetutil.NetworkProviderToType("unknown-provider")
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(ContainSubstring("unknown network provider")))
			Expect(result).To(BeEmpty())
		})
	})
})

var _ = Describe("TypeToNetworkProvider", func() {
	When("type is NetworkProviderTypeVDS", func() {
		It("returns NetworkProviderVSphereDistributed", func() {
			result, err := netsetutil.TypeToNetworkProvider(pkgcfg.NetworkProviderTypeVDS)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(netopv1alpha1.NetworkProviderVSphereDistributed))
		})
	})

	When("type is NetworkProviderTypeNSXT", func() {
		It("returns NetworkProviderNSXTier1", func() {
			result, err := netsetutil.TypeToNetworkProvider(pkgcfg.NetworkProviderTypeNSXT)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(netopv1alpha1.NetworkProviderNSXTier1))
		})
	})

	When("type is NetworkProviderTypeVPC", func() {
		It("returns NetworkProviderVPC", func() {
			result, err := netsetutil.TypeToNetworkProvider(pkgcfg.NetworkProviderTypeVPC)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(netopv1alpha1.NetworkProviderVPC))
		})
	})

	When("type is unknown", func() {
		It("returns an error", func() {
			result, err := netsetutil.TypeToNetworkProvider("unknown-type")
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(ContainSubstring("unknown network provider type")))
			Expect(result).To(BeEmpty())
		})
	})
})
