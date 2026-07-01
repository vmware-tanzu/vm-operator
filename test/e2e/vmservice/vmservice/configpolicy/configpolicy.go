// Copyright (c) 2026 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package configpolicy contains E2E tests for the VirtualMachineConfigPolicy
// feature, covering cluster capability discovery and policy enforcement.
package configpolicy

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vimv1 "github.com/vmware-tanzu/vm-operator/external/vim/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/test/e2e/utils"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/common"
	e2eConfig "github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/consts"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/skipper"
	"github.com/vmware-tanzu/vm-operator/test/e2e/wcpframework"
)

// SpecInput holds the inputs for Spec.
type SpecInput struct {
	ClusterProxy     wcpframework.WCPClusterProxyInterface
	Config           *e2eConfig.E2EConfig
	WCPNamespaceName string
}

// Spec verifies the VirtualMachineConfigPolicy feature end-to-end.
// Currently covers zone controller fan-out (S3); policy enforcement
// (S8/S9) and capability population (S5/S6/S7) will be added here.
func Spec(ctx context.Context, inputGetter func() SpecInput) {
	const specName = "vm-config-policy"

	var (
		input           SpecInput
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
				// MoID derived from each zone's pool MoIDs. List them once and
				// verify that for each zone at least one ConfigTarget exists and
				// has a matching spec.id.
				var ctList vimv1.ConfigTargetList
				Expect(svClusterClient.List(ctx, &ctList)).To(Succeed())

				for i := range zoneList.Items {
					z := &zoneList.Items[i]
					Expect(z.Spec.ManagedVMs.PoolMoIDs).ToNot(BeEmpty(),
						"Zone %q/%q has no pool MoIDs in spec.managedVMs.poolMoIDs",
						z.Namespace, z.Name)

					Expect(ctList.Items).ToNot(BeEmpty(),
						"expected at least one ConfigTarget for zone %q", z.Name)

					for j := range ctList.Items {
						ct := &ctList.Items[j]
						Expect(ct.Spec.ID.ID).To(Equal(ct.Name),
							"ConfigTarget %q spec.id.id should equal its metadata.name", ct.Name)
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
					policy := &vimv1.VirtualMachineConfigPolicy{}
					Expect(svClusterClient.Get(ctx,
						ctrlclient.ObjectKey{Name: z.Name, Namespace: input.WCPNamespaceName},
						policy)).To(Succeed(),
						"VirtualMachineConfigPolicy %q/%q should exist", input.WCPNamespaceName, z.Name)

					Expect(policy.Spec.Zone).To(Equal(z.Name),
						"VirtualMachineConfigPolicy %q/%q should reference zone %q",
						input.WCPNamespaceName, z.Name, z.Name)
				}
			})
	})
}
