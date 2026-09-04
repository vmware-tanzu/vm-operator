package skipper

import (
	"context"
	"os"

	. "github.com/onsi/gomega"
	"golang.org/x/crypto/ssh"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware-tanzu/vm-operator/test/e2e/appple2e/util"
	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
	e2essh "github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/ssh"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/testbed"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/test/e2e/utils"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/common"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/lib/vmoperator"
)

func SkipUnlessNetworkingIsVPC(ctx context.Context, client ctrlclient.Client, config *config.E2EConfig) {
	if !vmoperator.IsNetworkNsxtVPC(ctx, client, config) {
		framework.SkipInternalf(1, "skip if not VPC networking environment")
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

func SkipUnlessSupervisorCapabilityEnabled(ctx context.Context, vmSvcClusterProxy *common.VMServiceClusterProxy, capabilityName string) {
	sshCommandRunner, _ := e2essh.NewSSHCommandRunner(
		vcenter.GetVCPNIDFromKubeconfigFile(ctx, vmSvcClusterProxy.GetKubeconfigPath()),
		vcenter.VCSSHPort,
		testbed.RootUsername,
		[]ssh.AuthMethod{
			ssh.Password(testbed.RootPassword),
		},
	)
	isAsyncSvUpgradeEnabled, _ := util.IsFSSEnabled(sshCommandRunner, utils.SupervisorAsyncUpgradeFSS)

	if !utils.IsSupervisorCapabilityEnabled(
		ctx,
		vmSvcClusterProxy.GetClient(),
		capabilityName,
		isAsyncSvUpgradeEnabled,
	) {
		framework.SkipInternalf(1, "skip the test due to Supervisor capability %q is not enabled", capabilityName)
	}
}

func SkipUnlessSupervisorHasAtleastOneZoneWithHostCount(ctx context.Context, vmSvcClusterProxy *common.VMServiceClusterProxy, minHostCount int) {
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
