// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package workloadnetworkconfig_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	netopv1alpha1 "github.com/vmware-tanzu/net-operator-api/api/v1alpha1"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

// dummySystemConfiguration returns a NamespaceNetworkConfig that satisfies
// the CEL validation rules on WorkloadNetworkConfiguration for the given
// network provider type.
func dummySystemConfiguration(providerType netopv1alpha1.NetworkProvider) *netopv1alpha1.NamespaceNetworkConfig {
	switch providerType {
	case netopv1alpha1.NetworkProviderVSphereDistributed:
		return &netopv1alpha1.NamespaceNetworkConfig{
			VSphereDistributedConfig: netopv1alpha1.VSphereDistributedConfig{
				Networks: []netopv1alpha1.VSphereDistributedNetworkRef{
					{Name: "dummy-network"},
				},
				DefaultNetwork: "dummy-network",
			},
		}
	case netopv1alpha1.NetworkProviderVPC:
		return &netopv1alpha1.NamespaceNetworkConfig{
			VPCConfig: netopv1alpha1.VPCConfig{
				AutoCreateConfig: netopv1alpha1.AutoCreateVPCConfig{
					NSXProject:             "dummy-project",
					VPCConnectivityProfile: "dummy-profile",
				},
			},
		}
	default:
		return &netopv1alpha1.NamespaceNetworkConfig{}
	}
}

var _ = Describe(
	"Reconcile",
	Label(
		testlabels.Controller,
		testlabels.EnvTest,
		testlabels.API,
	),
	func() {

		var (
			ctx    *builder.IntegrationTestContext
			wnc    *netopv1alpha1.WorkloadNetworkConfiguration
			wncKey ctrlclient.ObjectKey
			dep    appsv1.Deployment
			depKey ctrlclient.ObjectKey
		)

		BeforeEach(func() {
			ctx = suite.NewIntegrationTestContext()
			wnc = &netopv1alpha1.WorkloadNetworkConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
			}
			wncKey = ctrlclient.ObjectKeyFromObject(wnc)

			dep = appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "vmop-deployment-",
					Namespace:    ctx.PodNamespace,
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "controller-manager",
						},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "controller-manager",
							Namespace: ctx.PodNamespace,
							Labels: map[string]string{
								"app": "controller-manager",
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "controller-manager",
									Image: "vmop:latest",
								},
							},
						},
					},
				},
			}

			Expect(ctx.Client.Create(ctx, &dep)).To(Succeed())
			depKey = ctrlclient.ObjectKeyFromObject(&dep)

			pkgcfg.SetContext(
				suite.Context,
				func(config *pkgcfg.Config) {
					config.DeploymentName = dep.Name
					config.PodNamespace = ctx.PodNamespace
				},
			)
		})

		JustBeforeEach(func() {
			Expect(ctx.Client.Create(ctx, wnc)).To(Succeed())
		})

		AfterEach(func() {
			err := ctx.Client.Delete(ctx, wnc)
			Expect(ctrlclient.IgnoreNotFound(err)).To(Succeed())
			Eventually(func(g Gomega) {
				err := ctx.Client.Get(ctx, wncKey, wnc)
				g.Expect(err).To(HaveOccurred())
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}, time.Second*5).Should(Succeed())

			ctx.AfterEach()
			ctx = nil
		})

		cfgClusterSupportedTypesStr := func(types ...pkgcfg.NetworkProviderType) string {
			s := make([]string, 0, len(types))
			for i := range types {
				s = append(s, string(types[i]))
			}
			return pkgcfg.SliceToString(s)
		}

		When("the WorkloadNetworkConfiguration is created", func() {
			BeforeEach(func() {
				wnc.Spec = netopv1alpha1.WorkloadNetworkConfigurationSpec{
					Providers: []netopv1alpha1.NetworkProviderEntry{
						{
							Type:                netopv1alpha1.NetworkProviderVSphereDistributed,
							SystemConfiguration: dummySystemConfiguration(netopv1alpha1.NetworkProviderVSphereDistributed),
						},
					},
					ActiveSystemProvider: netopv1alpha1.NetworkProviderVSphereDistributed,
				}
				pkgcfg.SetContext(suite.Context, func(config *pkgcfg.Config) {
					config.ClusterNetworkProviderTypes = cfgClusterSupportedTypesStr(pkgcfg.NetworkProviderTypeVDS)
				})
			})

			It("does not restart the pod", func() {
				// Config.ClusterNetworkProviderTypes was resolved from this
				// same CR's state at startup, so the initial sync of the
				// existing object must not also trigger a restart.
				Consistently(func(g Gomega) {
					var dep appsv1.Deployment
					g.Expect(ctx.Client.Get(ctx, depKey, &dep)).To(Succeed())
					g.Expect(dep.Spec.Template.Annotations).ToNot(
						HaveKey(pkgconst.LastRestartTimeAnnotationKey))
				}, time.Second*3).Should(Succeed())
			})
		})

		When("a provider is added to the providers list", func() {
			BeforeEach(func() {
				wnc.Spec = netopv1alpha1.WorkloadNetworkConfigurationSpec{
					Providers: []netopv1alpha1.NetworkProviderEntry{
						{
							Type:                netopv1alpha1.NetworkProviderVSphereDistributed,
							SystemConfiguration: dummySystemConfiguration(netopv1alpha1.NetworkProviderVSphereDistributed),
						},
					},
					ActiveSystemProvider: netopv1alpha1.NetworkProviderVSphereDistributed,
				}
				pkgcfg.SetContext(suite.Context, func(config *pkgcfg.Config) {
					config.ClusterNetworkProviderTypes = cfgClusterSupportedTypesStr(pkgcfg.NetworkProviderTypeVDS)
				})
			})

			It("restarts the pod", func() {
				// Update the CR to add a new provider.
				var wnc netopv1alpha1.WorkloadNetworkConfiguration
				Expect(ctx.Client.Get(ctx, wncKey, &wnc)).To(Succeed())
				wnc.Spec.Providers = append(wnc.Spec.Providers, netopv1alpha1.NetworkProviderEntry{
					Type:                netopv1alpha1.NetworkProviderVPC,
					SystemConfiguration: dummySystemConfiguration(netopv1alpha1.NetworkProviderVPC),
				})
				Expect(ctx.Client.Update(ctx, &wnc)).To(Succeed())

				Eventually(func(g Gomega) {
					var dep appsv1.Deployment
					g.Expect(ctx.Client.Get(ctx, depKey, &dep)).To(Succeed())
					lastExitTimeStr := dep.Spec.Template.Annotations[pkgconst.LastRestartTimeAnnotationKey]
					g.Expect(lastExitTimeStr).ToNot(BeEmpty())
					lastExitReason := dep.Spec.Template.Annotations[pkgconst.LastRestartReasonAnnotationKey]
					g.Expect(lastExitReason).To(ContainSubstring("network providers have changed"))
				}, time.Second*5).Should(Succeed())
			})
		})

		When("a provider is removed from the providers list", func() {
			BeforeEach(func() {
				wnc.Spec = netopv1alpha1.WorkloadNetworkConfigurationSpec{
					Providers: []netopv1alpha1.NetworkProviderEntry{
						{
							Type:                netopv1alpha1.NetworkProviderVSphereDistributed,
							SystemConfiguration: dummySystemConfiguration(netopv1alpha1.NetworkProviderVSphereDistributed),
						},
						{
							Type:                netopv1alpha1.NetworkProviderVPC,
							SystemConfiguration: dummySystemConfiguration(netopv1alpha1.NetworkProviderVPC),
						},
					},
					ActiveSystemProvider: netopv1alpha1.NetworkProviderVSphereDistributed,
				}
				pkgcfg.SetContext(suite.Context, func(config *pkgcfg.Config) {
					config.ClusterNetworkProviderTypes = cfgClusterSupportedTypesStr(pkgcfg.NetworkProviderTypeVDS, pkgcfg.NetworkProviderTypeVPC)
				})
			})

			It("restarts the pod", func() {
				// Update the CR to remove a provider.
				var wnc netopv1alpha1.WorkloadNetworkConfiguration
				Expect(ctx.Client.Get(ctx, wncKey, &wnc)).To(Succeed())
				wnc.Spec.Providers = []netopv1alpha1.NetworkProviderEntry{
					{
						Type:                netopv1alpha1.NetworkProviderVSphereDistributed,
						SystemConfiguration: dummySystemConfiguration(netopv1alpha1.NetworkProviderVSphereDistributed),
					},
				}
				Expect(ctx.Client.Update(ctx, &wnc)).To(Succeed())

				Eventually(func(g Gomega) {
					var dep appsv1.Deployment
					g.Expect(ctx.Client.Get(ctx, depKey, &dep)).To(Succeed())
					lastExitTimeStr := dep.Spec.Template.Annotations[pkgconst.LastRestartTimeAnnotationKey]
					g.Expect(lastExitTimeStr).ToNot(BeEmpty())
					lastExitReason := dep.Spec.Template.Annotations[pkgconst.LastRestartReasonAnnotationKey]
					g.Expect(lastExitReason).To(ContainSubstring("network providers have changed"))
				}, time.Second*5).Should(Succeed())
			})
		})

		When("the providers list has not changed", func() {
			BeforeEach(func() {
				wnc.Spec = netopv1alpha1.WorkloadNetworkConfigurationSpec{
					Providers: []netopv1alpha1.NetworkProviderEntry{
						{
							Type:                netopv1alpha1.NetworkProviderVSphereDistributed,
							SystemConfiguration: dummySystemConfiguration(netopv1alpha1.NetworkProviderVSphereDistributed),
						},
						{
							Type:                netopv1alpha1.NetworkProviderVPC,
							SystemConfiguration: dummySystemConfiguration(netopv1alpha1.NetworkProviderVPC),
						},
					},
					ActiveSystemProvider: netopv1alpha1.NetworkProviderVSphereDistributed,
				}
				pkgcfg.SetContext(suite.Context, func(config *pkgcfg.Config) {
					config.ClusterNetworkProviderTypes = cfgClusterSupportedTypesStr(pkgcfg.NetworkProviderTypeVDS, pkgcfg.NetworkProviderTypeVPC)
				})
			})

			It("does not restart the pod", func() {
				// Trigger a second reconcile by updating a non-provider field.
				var wnc netopv1alpha1.WorkloadNetworkConfiguration
				Expect(ctx.Client.Get(ctx, wncKey, &wnc)).To(Succeed())
				wnc.Annotations = map[string]string{"test": "updated"}
				Expect(ctx.Client.Update(ctx, &wnc)).To(Succeed())

				Consistently(func(g Gomega) {
					var dep appsv1.Deployment
					g.Expect(ctx.Client.Get(ctx, depKey, &dep)).To(Succeed())
					g.Expect(dep.Spec.Template.Annotations).ToNot(
						HaveKey(pkgconst.LastRestartTimeAnnotationKey))
				}, time.Second*3).Should(Succeed())
			})
		})

		When("the providers list is reordered but otherwise unchanged", func() {
			BeforeEach(func() {
				wnc.Spec = netopv1alpha1.WorkloadNetworkConfigurationSpec{
					Providers: []netopv1alpha1.NetworkProviderEntry{
						{
							Type:                netopv1alpha1.NetworkProviderVSphereDistributed,
							SystemConfiguration: dummySystemConfiguration(netopv1alpha1.NetworkProviderVSphereDistributed),
						},
						{
							Type:                netopv1alpha1.NetworkProviderVPC,
							SystemConfiguration: dummySystemConfiguration(netopv1alpha1.NetworkProviderVPC),
						},
					},
					ActiveSystemProvider: netopv1alpha1.NetworkProviderVSphereDistributed,
				}
				pkgcfg.SetContext(suite.Context, func(config *pkgcfg.Config) {
					config.ClusterNetworkProviderTypes = cfgClusterSupportedTypesStr(pkgcfg.NetworkProviderTypeVDS, pkgcfg.NetworkProviderTypeVPC)
				})
			})

			It("does not restart the pod", func() {
				// Update the CR to reorder the providers without changing
				// the set of provider types.
				var wnc netopv1alpha1.WorkloadNetworkConfiguration
				Expect(ctx.Client.Get(ctx, wncKey, &wnc)).To(Succeed())
				wnc.Spec.Providers = []netopv1alpha1.NetworkProviderEntry{
					{
						Type:                netopv1alpha1.NetworkProviderVPC,
						SystemConfiguration: dummySystemConfiguration(netopv1alpha1.NetworkProviderVPC),
					},
					{
						Type:                netopv1alpha1.NetworkProviderVSphereDistributed,
						SystemConfiguration: dummySystemConfiguration(netopv1alpha1.NetworkProviderVSphereDistributed),
					},
				}
				Expect(ctx.Client.Update(ctx, &wnc)).To(Succeed())

				Consistently(func(g Gomega) {
					var dep appsv1.Deployment
					g.Expect(ctx.Client.Get(ctx, depKey, &dep)).To(Succeed())
					g.Expect(dep.Spec.Template.Annotations).ToNot(
						HaveKey(pkgconst.LastRestartTimeAnnotationKey))
				}, time.Second*3).Should(Succeed())
			})
		})

		When("WorkloadNetworkConfiguration is deleted", func() {
			BeforeEach(func() {
				wnc.Spec = netopv1alpha1.WorkloadNetworkConfigurationSpec{
					Providers: []netopv1alpha1.NetworkProviderEntry{
						{
							Type:                netopv1alpha1.NetworkProviderVSphereDistributed,
							SystemConfiguration: dummySystemConfiguration(netopv1alpha1.NetworkProviderVSphereDistributed),
						},
					},
					ActiveSystemProvider: netopv1alpha1.NetworkProviderVSphereDistributed,
				}
				pkgcfg.SetContext(suite.Context, func(config *pkgcfg.Config) {
					config.ClusterNetworkProviderTypes = cfgClusterSupportedTypesStr(pkgcfg.NetworkProviderTypeVDS)
				})
			})

			It("does not restart the pod", func() {
				// Delete the CR.
				Expect(ctx.Client.Delete(ctx, wnc)).To(Succeed())

				// Controller should ignore delete events.
				Consistently(func(g Gomega) {
					var dep appsv1.Deployment
					g.Expect(ctx.Client.Get(ctx, depKey, &dep)).To(Succeed())
					g.Expect(dep.Spec.Template.Annotations).ToNot(
						HaveKey(pkgconst.LastRestartTimeAnnotationKey))
				}, time.Second*3).Should(Succeed())
			})
		})
	})
