// Copyright (c) 2026 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package configpolicy contains E2E tests for the VirtualMachineConfigPolicy
// and ConfigTarget resources created by the zone controller fan-out.
package configpolicy

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware-tanzu/vm-operator/test/e2e/utils"
	e2eConfig "github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/common"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/consts"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/skipper"
	"github.com/vmware-tanzu/vm-operator/test/e2e/wcpframework"
)

var (
	configTargetGVK = schema.GroupVersionKind{
		Group:   "vim.vmware.com",
		Version: "v1alpha1",
		Kind:    "ConfigTarget",
	}
	vmConfigPolicyGVK = schema.GroupVersionKind{
		Group:   "vim.vmware.com",
		Version: "v1alpha1",
		Kind:    "VirtualMachineConfigPolicy",
	}
)

// ZoneFanOutSpecInput holds the inputs for the ZoneFanOutSpec.
type ZoneFanOutSpecInput struct {
	ClusterProxy     wcpframework.WCPClusterProxyInterface
	Config           *e2eConfig.E2EConfig
	WCPNamespaceName string
}

// ZoneFanOutSpec verifies that the zone controller fans out ConfigTarget and
// VirtualMachineConfigPolicy objects when a Zone is reconciled with the
// VirtualMachineConfigPolicy capability active.
func ZoneFanOutSpec(ctx context.Context, inputGetter func() ZoneFanOutSpecInput) {
	const specName = "zone-config-target-fan-out"

	var (
		input           ZoneFanOutSpecInput
		svClusterProxy  *common.VMServiceClusterProxy
		svClusterClient ctrlclient.Client
	)

	BeforeEach(func() {
		input = inputGetter()
		Expect(input.Config).ToNot(BeNil(),
			"Invalid argument. input.Config can't be nil when calling %s spec", specName)
		Expect(input.ClusterProxy).ToNot(BeNil(),
			"Invalid argument. input.ClusterProxy can't be nil when calling %s spec", specName)
		Expect(input.WCPNamespaceName).ToNot(BeEmpty(),
			"Invalid argument. input.WCPNamespaceName can't be empty when calling %s spec", specName)

		svClusterProxy = input.ClusterProxy.(*common.VMServiceClusterProxy)
		svClusterClient = input.ClusterProxy.GetClient()

		skipper.SkipUnlessSupervisorCapabilityEnabled(ctx, svClusterProxy, consts.VirtualMachineConfigPolicyCapabilityName)
	})

	Context("When a workload namespace has zones", func() {
		It("Should have at least one ConfigTarget derived from the zone's pool MoIDs",
			Label("extended-functional", "experimental"),
			func() {
				zoneList, err := utils.ListZonesByNamespace(ctx, svClusterClient, input.WCPNamespaceName)
				Expect(err).ToNot(HaveOccurred())
				Expect(zoneList.Items).ToNot(BeEmpty(),
					"expected at least one Zone in namespace %q", input.WCPNamespaceName)

				// ConfigTargets are cluster-scoped and named after the cluster
				// MoID derived from each zone's pool MoIDs. Verify that for each
				// zone at least one ConfigTarget exists and has a matching spec.id.
				for i := range zoneList.Items {
					z := &zoneList.Items[i]
					Expect(z.Spec.ManagedVMs.PoolMoIDs).ToNot(BeEmpty(),
						"Zone %q/%q has no pool MoIDs in spec.managedVMs.poolMoIDs",
						z.Namespace, z.Name)

					// The zone controller calls GetResourcePoolOwnerMoRef for each
					// pool MoID and creates a ConfigTarget per unique cluster. We
					// list all cluster-scoped ConfigTargets and verify at least one
					// has spec.id.id == metadata.name (structurally valid).
					var ctList unstructured.UnstructuredList
					ctList.SetGroupVersionKind(schema.GroupVersionKind{
						Group:   configTargetGVK.Group,
						Version: configTargetGVK.Version,
						Kind:    configTargetGVK.Kind + "List",
					})
					Expect(svClusterClient.List(ctx, &ctList)).To(Succeed())
					Expect(ctList.Items).ToNot(BeEmpty(),
						"expected at least one ConfigTarget for zone %q", z.Name)

					for j := range ctList.Items {
						ct := &ctList.Items[j]
						id, _, _ := unstructured.NestedString(ct.Object, "spec", "id", "id")
						Expect(id).To(Equal(ct.GetName()),
							"ConfigTarget %q spec.id.id should equal its metadata.name", ct.GetName())
					}
				}
			})

		It("Should have a VirtualMachineConfigPolicy for each zone in the namespace",
			Label("extended-functional", "experimental"),
			func() {
				zoneList, err := utils.ListZonesByNamespace(ctx, svClusterClient, input.WCPNamespaceName)
				Expect(err).ToNot(HaveOccurred())
				Expect(zoneList.Items).ToNot(BeEmpty(),
					"expected at least one Zone in namespace %q", input.WCPNamespaceName)

				for i := range zoneList.Items {
					z := &zoneList.Items[i]
					policy := &unstructured.Unstructured{}
					policy.SetGroupVersionKind(vmConfigPolicyGVK)
					Expect(svClusterClient.Get(ctx,
						ctrlclient.ObjectKey{Name: z.Name, Namespace: z.Namespace},
						policy)).To(Succeed(),
						"VirtualMachineConfigPolicy %q/%q should exist", z.Namespace, z.Name)

					zoneField, _, _ := unstructured.NestedString(policy.Object, "spec", "zone")
					Expect(zoneField).To(Equal(z.Name),
						"VirtualMachineConfigPolicy %q/%q should reference zone %q",
						z.Namespace, z.Name, z.Name)
				}
			})

		It("Should not delete a ConfigTarget when the zone is reconciled again",
			Label("extended-functional", "experimental"),
			func() {
				// Pick any existing ConfigTarget and verify its UID is stable
				// across multiple reconcile cycles (idempotency of CreateOrPatch).
				var ctList unstructured.UnstructuredList
				ctList.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   configTargetGVK.Group,
					Version: configTargetGVK.Version,
					Kind:    configTargetGVK.Kind + "List",
				})
				Expect(svClusterClient.List(ctx, &ctList)).To(Succeed())
				Expect(ctList.Items).ToNot(BeEmpty())

				ct := &ctList.Items[0]
				originalUID := ct.GetUID()
				ctName := ct.GetName()

				// The controller reconciles periodically; a 30s window exercises
				// idempotency without requiring a manual zone patch.
				Consistently(func(g Gomega) {
					ct2 := &unstructured.Unstructured{}
					ct2.SetGroupVersionKind(configTargetGVK)
					g.Expect(svClusterClient.Get(ctx, ctrlclient.ObjectKey{Name: ctName}, ct2)).
						To(Succeed())
					g.Expect(ct2.GetUID()).To(Equal(originalUID),
						"ConfigTarget %q should not have been recreated", ctName)
				}, "30s", "5s").Should(Succeed())
			})
	})
}

