package skipper

import (
	"context"
	"os"
	"strconv"

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

func SkipUnlessHAFSSEnabled(ctx context.Context, client ctrlclient.Client, config *config.E2EConfig) {
	skipUnlessFSSEnabled(ctx, client, config.GetVariable("VMOPNamespace"), config.GetVariable("VMOPDeploymentName"), config.GetVariable("VMOPManagerCommand"), config.GetVariable("EnvFSSHA"))
}

func SkipUnlessVMImageRegistryFSSEnabled(ctx context.Context, client ctrlclient.Client, config *config.E2EConfig) {
	skipUnlessFSSEnabled(ctx, client, config.GetVariable("VMOPNamespace"), config.GetVariable("VMOPDeploymentName"), config.GetVariable("VMOPManagerCommand"), config.GetVariable("EnvFSSVMImageRegistry"))
}

func SkipUnlessNamespacedVMClassFSSEnabled(ctx context.Context, client ctrlclient.Client, config *config.E2EConfig) {
	skipUnlessFSSEnabled(ctx, client, config.GetVariable("VMOPNamespace"), config.GetVariable("VMOPDeploymentName"), config.GetVariable("VMOPManagerCommand"), config.GetVariable("EnvFSSNamespacedVMClass"))
}

func SkipUnlessV1a2FSSEnabled(ctx context.Context, client ctrlclient.Client, config *config.E2EConfig) {
	skipUnlessFSSEnabled(ctx, client, config.GetVariable("VMOPNamespace"), config.GetVariable("VMOPDeploymentName"), config.GetVariable("VMOPManagerCommand"), config.GetVariable("EnvFSSV1alpha2"))
}

func SkipUnlessNetworkingIsVPC(ctx context.Context, client ctrlclient.Client, config *config.E2EConfig) {
	if !vmoperator.IsNetworkNsxtVPC(ctx, client, config) {
		framework.SkipInternalf(1, "skip if not VPC networking environment")
	}
}

func SkipUnlessWindowsFSSEnabled(ctx context.Context, client ctrlclient.Client, config *config.E2EConfig) {
	skipUnlessFSSEnabled(ctx, client, config.GetVariable("VMOPNamespace"), config.GetVariable("VMOPDeploymentName"), config.GetVariable("VMOPManagerCommand"), config.GetVariable("EnvFSSWindowsSysprep"))
}

func skipUnlessFSSEnabled(ctx context.Context, client ctrlclient.Client, deploymentNS, deploymentName, command, fss string) {
	envs, err := utils.GetCommandEnvVars(ctx, client, deploymentNS, deploymentName, command)
	Expect(err).To(Succeed(), "%q FSS cannot not be fetched for command %q", fss, command)

	mmEnabled, _ := strconv.ParseBool(envs[fss])
	if !mmEnabled {
		framework.SkipInternalf(1, "%q FSS is not enabled for command %q", fss, command)
	}
}

func SkipUnlessInfraIs(clusterInfra, requiredInfra string) {
	if !framework.InfraIs(clusterInfra, requiredInfra) {
		framework.SkipInternalf(1, "required infrastructure environment: %s for test does not match with provided infrastructure environment:%s", clusterInfra, requiredInfra)
	}
}

func SkipIfStretchSupervisorIsEnabled() {
	if os.Getenv("STRETCHED_SUPERVISOR") == "true" {
		framework.SkipInternalf(1, "skip the test due to StretchSupervisor is enabled")
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
		vmSvcClusterProxy.GetClientSet(),
		vmSvcClusterProxy.GetDynamicClient(),
		capabilityName,
		isAsyncSvUpgradeEnabled,
	) {
		framework.SkipInternalf(1, "skip the test due to Supervisor capability %q is not enabled", capabilityName)
	}
}

func SkipUnlessSnapshotFSSEnabled(ctx context.Context, vmSvcClusterProxy *common.VMServiceClusterProxy, config *config.E2EConfig) {
	sshCommandRunner, _ := e2essh.NewSSHCommandRunner(
		vcenter.GetVCPNIDFromKubeconfigFile(ctx, vmSvcClusterProxy.GetKubeconfigPath()),
		vcenter.VCSSHPort,
		testbed.RootUsername,
		[]ssh.AuthMethod{ssh.Password(testbed.RootPassword)})
	isVMVMSnapshotEnabled, _ := util.IsFSSEnabled(sshCommandRunner, utils.SupervisorVMSnapshotFSS)

	if !isVMVMSnapshotEnabled {
		framework.SkipInternalf(1, "skip the test due to %s is disabled.", utils.SupervisorVMSnapshotFSS)
	}
}
