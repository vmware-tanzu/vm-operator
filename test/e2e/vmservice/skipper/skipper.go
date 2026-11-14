// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package skipper

import (
	"context"
	"os"

	. "github.com/onsi/gomega"

	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
	"github.com/vmware-tanzu/vm-operator/test/e2e/utils"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/consts"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/lib/vmoperator"
)

func SkipUnlessNetworkingIsVPC(ctx context.Context, client ctrlclient.Client, config *config.E2EConfig) {
	if !vmoperator.IsNetworkNsxtVPC(ctx, client, config) {
		framework.SkipInternalf(1, "skip if not VPC networking environment")
	}
}

func SkipIfNetworkTopologyIsNSX(config *config.E2EConfig) {
	if framework.NetworkTopologyIs(config.InfraConfig.NetworkingTopology, consts.NSX) {
		framework.SkipInternalf(1, "skip on NSX networking topology (nsx/vpc)")
	}
}

func SkipUnlessInfraIs(clusterInfra, requiredInfra string) {
	if !framework.InfraIs(clusterInfra, requiredInfra) {
		framework.SkipInternalf(1, "required infrastructure environment: %s for test does not match with provided infrastructure environment:%s", clusterInfra, requiredInfra)
	}
}

func SkipUnlessStretchSupervisorIsEnabled() {
	// Skip the test for 1CP and 1Worker if the stretch supervisor is enabled
	if os.Getenv("STRETCHED_SUPERVISOR") != "true" {
		framework.SkipInternalf(1, "skip the test due to StretchSupervisor is not enabled")
	}
}

// SkipUnlessSupervisorCapabilityEnabled checks the snapshot loaded during suite
// setup and skips when the capability is missing or disabled.
func SkipUnlessSupervisorCapabilityEnabled(capabilities map[string]bool, capabilityName string) {
	Expect(capabilities).NotTo(BeNil(), "Supervisor capabilities must be loaded during suite setup")
	if !capabilities[capabilityName] {
		framework.SkipInternalf(1, "skip the test due to Supervisor capability %q is not enabled", capabilityName)
	}
}

func SkipUnlessSupervisorHasAtleastOneZoneWithHostCount(ctx context.Context, vmSvcClusterProxy framework.ClusterProxyInterface, minHostCount int) {
	zoneHostInfos, err := utils.GetHostsPerZone(ctx, vmSvcClusterProxy.GetClient(), vmSvcClusterProxy.GetKubeconfigPath())
	Expect(err).NotTo(HaveOccurred(), "failed to list zones with hosts")

	minHostsRequirementSatisfied := false
	for _, zoneHostInfo := range zoneHostInfos {
		if len(zoneHostInfo.HostIDs) >= minHostCount {
			minHostsRequirementSatisfied = true
			break
		}
	}
	if !minHostsRequirementSatisfied {
		framework.SkipInternalf(1, "skip the test as minimum host for zone should atleast be %d", minHostCount)
	}
}
