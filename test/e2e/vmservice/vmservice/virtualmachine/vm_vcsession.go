// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/session"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	"golang.org/x/crypto/ssh"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	e2eframework "k8s.io/kubernetes/test/e2e/framework"
	capiutil "sigs.k8s.io/cluster-api/util"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"

	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
	e2essh "github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/ssh"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/testbed"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/common"
	e2eConfig "github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/consts"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/lib/vmoperator"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/skipper"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/vmservice"
	"github.com/vmware-tanzu/vm-operator/test/e2e/wcpframework"
)

const (
	// vpxdRestartCmd restarts the vCenter Server service. This command needs
	// validating on a real testbed; "service-control --restart vmware-vpxd"
	// is the documented fallback if vmon-cli is unavailable or restarts too
	// much, so correcting it here is a one-line change.
	vpxdRestartCmd = "vmon-cli -r vpxd"

	// vcSessionRecoveryTimeout bounds the VM operation used to observe
	// session recovery. A VM power-state change normally completes in well
	// under a minute. With inline re-login the session is restored on the
	// first faulting call, so the only added cost is one round trip. With
	// the legacy timer-driven keepalive the operation cannot succeed until
	// the next 5m tick. 90s is comfortably above the former and comfortably
	// below the latter.
	vcSessionRecoveryTimeout = 90 * time.Second

	// vpxdRestartWaitTimeout bounds how long vpxd itself may take to come
	// back after a restart, which can take several minutes on a loaded
	// testbed. The recovery clock starts once vCenter answers again.
	vpxdRestartWaitTimeout = 15 * time.Minute
)

// VCSessionRecoverySpecInput is the input for the VC session recovery spec.
type VCSessionRecoverySpecInput struct {
	ClusterProxy     wcpframework.WCPClusterProxyInterface
	Config           *e2eConfig.E2EConfig
	ArtifactFolder   string
	SkipCleanup      bool
	WCPNamespaceName string
}

// VCSessionRecoverySpec validates that VM Operator recovers from a lost
// vCenter session on the next call that needs it, instead of waiting for the
// timer-driven keepalive.
//
// Both It blocks are Serial because they take VM Operator's vCenter session
// away from every other spec. Both are skipped unless the
// supports_vc_session_inline_relogin Supervisor capability is activated, so
// a legacy-mode testbed is unaffected: a recovery test that passes in both
// modes is testing nothing.
func VCSessionRecoverySpec(ctx context.Context, inputGetter func() VCSessionRecoverySpecInput) {
	const specName = "vm-vcsession"

	var (
		input           VCSessionRecoverySpecInput
		config          *e2eConfig.E2EConfig
		clusterProxy    *common.VMServiceClusterProxy
		svClusterClient ctrlclient.Client
		vmClassName     string
		storageClass    string
		linuxVMIName    string
		vmopUserName    string
	)

	BeforeEach(func() {
		input = inputGetter()

		Expect(input.Config).NotTo(BeNil(),
			"Invalid argument. input.Config can't be nil when calling %s spec", specName)
		Expect(input.Config.InfraConfig).NotTo(BeNil(),
			"Invalid argument. input.Config.InfraConfig can't be nil when calling %s spec", specName)
		Expect(input.ClusterProxy).NotTo(BeNil(),
			"Invalid argument. input.ClusterProxy can't be nil when calling %s spec", specName)
		Expect(input.WCPNamespaceName).NotTo(BeEmpty(),
			"Invalid argument. input.WCPNamespaceName can't be empty when calling %s spec", specName)
		Expect(os.MkdirAll(input.ArtifactFolder, 0o755)).To(Succeed(),
			"Invalid argument. input.ArtifactFolder can't be created for %s spec", specName)

		skipper.SkipUnlessInfraIs(input.Config.InfraConfig.InfraName, consts.WCP)

		config = input.Config
		clusterProxy = input.ClusterProxy.(*common.VMServiceClusterProxy)
		svClusterClient = clusterProxy.GetClient()

		skipper.SkipUnlessSupervisorCapabilityEnabled(ctx, clusterProxy, consts.VCSessionInlineReloginCapabilityName)

		cancelPodWatches := framework.WatchPodLogsAndEventsInNamespaces(
			ctx,
			[]string{config.GetVariable("VMOPNamespace")},
			clusterProxy.GetRESTConfig(),
			filepath.Join(input.ArtifactFolder, specName),
		)
		DeferCleanup(cancelPodWatches)

		clusterResources := config.InfraConfig.ManagementClusterConfig.Resources
		vmClassName = clusterResources.VMClassName
		storageClass = clusterResources.StorageClassName

		linuxImageDisplayName := vmservice.GetDefaultImageDisplayName(clusterResources)
		linuxVMIName = vmoperator.WaitForVirtualMachineImageName(
			ctx, &config.Config, svClusterClient,
			input.WCPNamespaceName, linuxImageDisplayName)

		// The VM Operator solution user's sessions are what get disrupted.
		vmopNamespace := config.GetVariable("VMOPNamespace")
		secret, err := getVCSessionSpecSecret(ctx, svClusterClient, vmopNamespace)
		Expect(err).NotTo(HaveOccurred())
		vmopUserName = string(secret.Data["username"])
		Expect(vmopUserName).NotTo(BeEmpty())
	})

	// newAdminVimClient returns an authenticated vCenter admin client.
	newAdminVimClient := func() *vim25.Client {
		return vcenter.NewVimClientFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath())
	}

	// terminateVMOPSessions terminates every vCenter session that belongs to
	// the VM Operator solution user.
	terminateVMOPSessions := func(adminClient *vim25.Client) {
		var sm mo.SessionManager
		Expect(property.DefaultCollector(adminClient).RetrieveOne(
			ctx,
			*adminClient.ServiceContent.SessionManager,
			[]string{"sessionList"},
			&sm)).To(Succeed())

		var keys []string
		for _, sess := range sm.SessionList {
			if sess.UserName == vmopUserName {
				keys = append(keys, sess.Key)
			}
		}
		Expect(keys).NotTo(BeEmpty(), "no sessions found for user %q", vmopUserName)

		Expect(session.NewManager(adminClient).TerminateSession(ctx, keys)).To(Succeed())
	}

	// managerBaseline records the manager deployment's ready replicas and
	// the summed restart counts of its pods. A stable pair is what proves
	// recovery happened in process rather than via a crash-loop, and it
	// holds regardless of timing.
	type managerBaseline struct {
		readyReplicas int32
		restartCount  int32
	}

	managerBaselineOf := func() managerBaseline {
		vmopNamespace := config.GetVariable("VMOPNamespace")
		deployment := &appsv1.Deployment{}
		Expect(svClusterClient.Get(ctx,
			ctrlclient.ObjectKey{
				Namespace: vmopNamespace,
				Name:      config.GetVariable("VMOPDeploymentName"),
			},
			deployment)).To(Succeed())

		pods := &corev1.PodList{}
		Expect(svClusterClient.List(ctx, pods,
			ctrlclient.InNamespace(vmopNamespace),
			ctrlclient.MatchingLabels(deployment.Spec.Selector.MatchLabels))).To(Succeed())

		var total int32
		for i := range pods.Items {
			for _, cs := range pods.Items[i].Status.ContainerStatuses {
				total += cs.RestartCount
			}
		}
		return managerBaseline{
			readyReplicas: deployment.Status.ReadyReplicas,
			restartCount:  total,
		}
	}

	// assertManagerNotCrashLooped asserts the manager deployment's ready
	// replicas and pod restart counts stay unchanged from the baseline.
	// Consistently, not Eventually: this is a stability claim, and
	// Eventually would return on its first matching poll and never see a
	// restart that happens a few seconds later.
	assertManagerNotCrashLooped := func(baseline managerBaseline) {
		Consistently(func(g Gomega) {
			current := managerBaselineOf()
			g.Expect(current.restartCount).To(Equal(baseline.restartCount))
			g.Expect(current.readyReplicas).To(Equal(baseline.readyReplicas))
		}, 10*time.Second, time.Second).Should(Succeed())
	}

	// createPoweredOnVM creates a VM in the test namespace and waits until it
	// is reported powered on, so the recovery assertion measures VM
	// Operator's reaction and not first-boot latency.
	createPoweredOnVM := func(vmName string) types.NamespacedName {
		vmKey := types.NamespacedName{Name: vmName, Namespace: input.WCPNamespaceName}

		By("Creating a powered-on VM")
		vm := &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      vmName,
				Namespace: input.WCPNamespaceName,
			},
			Spec: vmopv1.VirtualMachineSpec{
				ClassName:        vmClassName,
				ImageName:        linuxVMIName,
				StorageClass:     storageClass,
				PowerState:       vmopv1.VirtualMachinePowerStateOn,
				PromoteDisksMode: vmopv1.VirtualMachinePromoteDisksModeDisabled,
				Bootstrap: &vmopv1.VirtualMachineBootstrapSpec{
					Disabled: true,
				},
			},
		}
		Expect(svClusterClient.Create(ctx, vm)).To(Succeed(), "failed to create VM %s", vmName)
		DeferCleanup(func() {
			if !input.SkipCleanup {
				vmoperator.DeleteVirtualMachine(ctx, svClusterClient, vmKey.Namespace, vmKey.Name)
				vmoperator.WaitForVirtualMachineToBeDeleted(ctx, config, svClusterClient, vmKey.Namespace, vmKey.Name)
			}
		})

		By("Waiting for VM to be created in vSphere")
		vmoperator.WaitForVirtualMachineConditionCreated(
			ctx, config, svClusterClient, input.WCPNamespaceName, vmName)

		By("Waiting for VM to be powered on")
		vmoperator.WaitForVirtualMachinePowerState(
			ctx, config, svClusterClient, input.WCPNamespaceName, vmName,
			string(vmopv1.VirtualMachinePowerStateOn))

		return vmKey
	}

	// drivePowerStateChangeToOff drives a VM power-state change to PoweredOff
	// and returns once the change is observed, bounded by
	// vcSessionRecoveryTimeout. The timing bound is the whole assertion: with
	// inline re-login the operation succeeds on the first faulting call,
	// while the legacy timer-driven keepalive cannot succeed until the next
	// 5m tick.
	drivePowerStateChangeToOff := func(vmKey types.NamespacedName) time.Duration {
		start := time.Now()

		Eventually(func(g Gomega) {
			vm := &vmopv1.VirtualMachine{}
			g.Expect(svClusterClient.Get(ctx, vmKey, vm)).To(Succeed())

			// Request the change once. On later polls the spec already
			// carries it and only the status is still catching up.
			if vm.Spec.PowerState != vmopv1.VirtualMachinePowerStateOff {
				vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
				g.Expect(svClusterClient.Update(ctx, vm)).To(Succeed())
			}

			// Assert on the status, never on the update: the poll must not
			// succeed until VM Operator has actually reached vCenter and
			// reported the new power state back. Succeeding on the update
			// alone would make the timing bound measure a Kubernetes write.
			g.Expect(vm.Status.PowerState).To(
				Equal(vmopv1.VirtualMachinePowerStateOff))
		}, vcSessionRecoveryTimeout, 3*time.Second).Should(Succeed(),
			"Timed out waiting for the VM %s power-state change to complete within %s",
			vmKey.Name, vcSessionRecoveryTimeout)

		return time.Since(start)
	}

	It("recovers when the VM Operator session is terminated", Serial,
		Label("core-functional", "experimental"), func() {
			By("Creating a powered-on VM")
			vmName := fmt.Sprintf("%s-terminate-%s", specName, capiutil.RandomString(4))
			vmKey := createPoweredOnVM(vmName)

			baselineManager := managerBaselineOf()

			By("Terminating the VM Operator solution user's vCenter sessions")
			adminClient := newAdminVimClient()
			terminateVMOPSessions(adminClient)

			By("Driving a VM power-state change and expecting recovery")
			elapsed := drivePowerStateChangeToOff(vmKey)
			e2eframework.Logf("VM %s recovered from a terminated session in %s",
				vmName, elapsed)

			By("Asserting the manager did not crash-loop")
			assertManagerNotCrashLooped(baselineManager)
		})

	It("recovers when vpxd restarts", Serial,
		Label("extended-functional", "disruptive", "experimental"), func() {
			By("Creating a powered-on VM")
			vmName := fmt.Sprintf("%s-vpxd-%s", specName, capiutil.RandomString(4))
			vmKey := createPoweredOnVM(vmName)

			baselineManager := managerBaselineOf()

			By("Recording a live session to verify disruption")
			adminClient := newAdminVimClient()
			DeferCleanup(func() {
				logoutTolerant(adminClient)
			})
			adminSession := session.NewManager(adminClient)
			sess, err := adminSession.UserSession(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(sess).NotTo(BeNil(), "the pre-restart session must be active")

			By("Restarting vpxd over SSH")
			runner, err := e2essh.NewSSHCommandRunner(
				vcenter.GetVCPNIDFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath()),
				vcenter.VCSSHPort,
				testbed.RootUsername,
				[]ssh.AuthMethod{
					ssh.Password(testbed.RootPassword),
				})
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				Expect(runner.Close()).To(Succeed())
			})
			_, err = runner.RunCommand(vpxdRestartCmd)
			// SSH may disconnect during a successful restart. Require the
			// live session above to be invalidated as well as a fresh login
			// to succeed, so a failed or no-op command cannot pass the test.
			e2eframework.Logf("ran %q: err=%v", vpxdRestartCmd, err)

			By("Waiting for vCenter to serve again with the old session invalidated")
			var t0 time.Time
			Eventually(func(g Gomega) {
				client, err := vcenter.NewVimClient(
					vcenter.GetVCPNIDFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath()),
					testbed.AdminUsername,
					testbed.AdminPassword)
				g.Expect(err).NotTo(HaveOccurred())
				defer logoutTolerant(client)
				oldSession, err := adminSession.UserSession(ctx)
				g.Expect(err).NotTo(HaveOccurred(), "transport failures do not prove session invalidation")
				g.Expect(oldSession).To(BeNil(), "vpxd restart did not invalidate the pre-existing session")
				t0 = time.Now()
			}, vpxdRestartWaitTimeout, 30*time.Second).Should(Succeed(),
				"Timed out waiting for vCenter recovery and proof of session disruption")

			By(fmt.Sprintf("Driving a VM power-state change and expecting recovery (VC back at %s)", t0))
			elapsed := drivePowerStateChangeToOff(vmKey)
			e2eframework.Logf("VM %s recovered from a vpxd restart in %s",
				vmName, elapsed)

			By("Asserting the manager did not crash-loop")
			assertManagerNotCrashLooped(baselineManager)
		})
}

// getVCSessionSpecSecret returns the secret that carries the VM Operator
// solution user's vCenter credentials.
func getVCSessionSpecSecret(
	ctx context.Context,
	client ctrlclient.Client,
	vmopNamespace string) (*corev1.Secret, error) {

	secret := &corev1.Secret{}
	err := client.Get(
		ctx,
		ctrlclient.ObjectKey{
			Namespace: vmopNamespace,
			Name:      "wcp-vmop-sa-vc-auth",
		},
		secret)
	return secret, err
}

// logoutTolerant logs out a vCenter client, ignoring a session that is
// already gone, e.g. after a vpxd restart.
func logoutTolerant(client *vim25.Client) {
	if client == nil || client.RoundTripper == nil {
		return
	}
	_, _ = methods.Logout(
		context.Background(),
		client.RoundTripper,
		&vimtypes.Logout{This: *client.ServiceContent.SessionManager})
}
