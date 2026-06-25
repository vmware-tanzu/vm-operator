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

	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/test/e2e/utils"
	e2eConfig "github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/common"
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
	const (
		specName      = "zone-config-target-fan-out"
		capabilityKey = "supports_vm_service_vm_config_policy"
	)

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

		skipper.SkipUnlessSupervisorCapabilityEnabled(ctx, svClusterProxy, capabilityKey)
	})

	Context("When a workload namespace has zones", func() {
		It("Should have a ConfigTarget for each cluster MoID in the zone's spec.managedVMs.clusterMoIDs",
			Label("extended-functional"),
			func() {
				zoneList, err := utils.ListZonesByNamespace(ctx, svClusterClient, input.WCPNamespaceName)
				Expect(err).ToNot(HaveOccurred())
				Expect(zoneList.Items).ToNot(BeEmpty(),
					"expected at least one Zone in namespace %q", input.WCPNamespaceName)

				for i := range zoneList.Items {
					z := &zoneList.Items[i]
					clusterMoIDs := collectClusterMoIDs(z)
					Expect(clusterMoIDs).ToNot(BeEmpty(),
						"Zone %q/%q has no cluster MoIDs in spec.managedVMs.clusterMoIDs",
						z.Namespace, z.Name)

					for _, clusterMoID := range clusterMoIDs {
						ct := &unstructured.Unstructured{}
						ct.SetGroupVersionKind(configTargetGVK)
						Expect(svClusterClient.Get(ctx,
							ctrlclient.ObjectKey{Name: clusterMoID}, ct)).
							To(Succeed(),
								"ConfigTarget %q should exist for zone %q", clusterMoID, z.Name)
					}
				}
			})

		It("Should have a VirtualMachineConfigPolicy for each zone in the namespace",
			Label("extended-functional"),
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
			Label("extended-functional"),
			func() {
				zoneList, err := utils.ListZonesByNamespace(ctx, svClusterClient, input.WCPNamespaceName)
				Expect(err).ToNot(HaveOccurred())
				Expect(zoneList.Items).ToNot(BeEmpty())

				z := zoneList.Items[0]
				clusterMoIDs := collectClusterMoIDs(&z)
				Expect(clusterMoIDs).ToNot(BeEmpty())

				clusterMoID := clusterMoIDs[0]

				ct := &unstructured.Unstructured{}
				ct.SetGroupVersionKind(configTargetGVK)
				Expect(svClusterClient.Get(ctx, ctrlclient.ObjectKey{Name: clusterMoID}, ct)).
					To(Succeed())
				originalUID := ct.GetUID()

				// Patch the zone to trigger another reconcile.
				patch := ctrlclient.MergeFrom(z.DeepCopy())
				if z.Labels == nil {
					z.Labels = map[string]string{}
				}
				z.Labels["vmop.test/touched"] = "true"
				Expect(svClusterClient.Patch(ctx, &z, patch)).To(Succeed())

				// Verify the ConfigTarget was not recreated.
				Eventually(func(g Gomega) {
					ct2 := &unstructured.Unstructured{}
					ct2.SetGroupVersionKind(configTargetGVK)
					g.Expect(svClusterClient.Get(ctx, ctrlclient.ObjectKey{Name: clusterMoID}, ct2)).
						To(Succeed())
					g.Expect(ct2.GetUID()).To(Equal(originalUID),
						"ConfigTarget %q should not have been recreated", clusterMoID)
				}).Should(Succeed())
			})
	})
}

// collectClusterMoIDs returns the deduplicated cluster MoIDs from
// zone.spec.managedVMs.clusterMoIDs.
func collectClusterMoIDs(z *topologyv1.Zone) []string {
	seen := make(map[string]struct{})
	var ids []string
	for _, id := range z.Spec.ManagedVMs.ClusterMoIDs {
		if id != "" {
			if _, ok := seen[id]; !ok {
				seen[id] = struct{}{}
				ids = append(ids, id)
			}
		}
	}
	return ids
}
