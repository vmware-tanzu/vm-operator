// Copyright (c) 2020-2025 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmservicee2e

import (
	"context"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	// Configure testing.
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	// Configure logging.
	logs "github.com/sirupsen/logrus"
	klog "k8s.io/klog/v2"
	_ "k8s.io/kubernetes/test/e2e/framework"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	conformancetestdata "k8s.io/kubernetes/test/conformance/testdata"
	"k8s.io/kubernetes/test/e2e/framework/testfiles"
	e2etestingmanifests "k8s.io/kubernetes/test/e2e/testing-manifests"
	testfixtures "k8s.io/kubernetes/test/fixtures"
	capiutil "sigs.k8s.io/cluster-api/util"

	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/wcp"
	"github.com/vmware-tanzu/vm-operator/test/e2e/manifestbuilders"
	"github.com/vmware-tanzu/vm-operator/test/e2e/utils"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/common"
	e2eConfig "github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/consts"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/lib/vmoperator"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/vmservice"
	"github.com/vmware-tanzu/vm-operator/test/e2e/wcpframework"
)

const (
	defaultBaseLogDirectoryPath = "test_logs"
	runCanonicalTestEnvValue    = "true"
)

var (
	echoPodLogs                 bool
	skipCleanup                 bool
	workloadIsolationFSSEnabled bool
	vksTesting                  bool
	configFilePath              string
	artifactFolder              string
	skipTests                   string
	repoRoot                    string
	testSuiteName               string
	wcpNamespaceName            string
	windowsServerVMName         string
	linuxVMName                 string
	config                      *e2eConfig.E2EConfig
	svClusterProxy              wcpframework.WCPClusterProxyInterface
	wcpClient                   wcp.WorkloadManagementAPI
	wcpNamespaceCtx             wcpframework.NamespaceContext
)

func init() {
	flag.BoolVar(&skipCleanup, "e2e.skip-resource-cleanup", false, "if true, the resource cleanup after tests will be skipped")
	flag.StringVar(&repoRoot, "e2e.repo-root", "../../", "Root directory of kubernetes repository, for finding test files.")
	flag.StringVar(&artifactFolder, "e2e.artifactFolder", defaultBaseLogDirectoryPath, "folder where e2e test artifact should be stored")
	flag.BoolVar(&echoPodLogs, "e2e.echo-pod-logs", false, "Emit pod logs and events to STDOUT")
	flag.StringVar(&configFilePath, "e2e.e2e-config", "", "path to the e2e config file")
	flag.StringVar(&skipTests, "e2e.test-skip", "", "if set, skip the tests matching the given regex")
	flag.StringVar(&wcpNamespaceName, "e2e.wcp-namespace-name", "", "name of the WCP namespace that already exists")

	// Enable embedded FS file lookup as fallback
	testfiles.AddFileSource(e2etestingmanifests.GetE2ETestingManifestsFS())
	testfiles.AddFileSource(testfixtures.GetTestFixturesFS())
	testfiles.AddFileSource(conformancetestdata.GetConformanceTestdataFS())
}

func TestVMService(t *testing.T) {
	// TODO: Deprecating repo-root over time... instead just use gobindata_util.go , see #23987.
	// Right now it is still needed, for example by
	// test/e2e/framework/ingress/ingress_utils.go
	// for providing the optional secret.yaml file and by
	// test/e2e/framework/util.go for cluster/log-dump.
	if repoRoot != "" {
		testfiles.AddFileSource(testfiles.RootFileSource{Root: repoRoot})
	}

	klog.SetOutput(GinkgoWriter)
	logf.SetLogger(klog.Background())
	logs.SetFormatter(&logs.JSONFormatter{})
	logs.SetOutput(GinkgoWriter)

	defer klog.Flush()

	RegisterFailHandler(Fail)
	RunSpecs(t, "VM Service E2E suite")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	Expect(configFilePath).To(BeAnExistingFile(), "Invalid test suite argument. e2e.config should be an existing file.")
	Expect(os.MkdirAll(artifactFolder, 0775)).To(Succeed(), "Invalid test suite argument. Can't create e2e.artifacts-folder %q", artifactFolder)

	testSuiteName = "vmsvc-e2e"
	framework.Byf("Testsuite Name: %s", testSuiteName)

	By("Initializing a runtime.Scheme with all the GVK relevant for this test")

	scheme := common.InitScheme()

	framework.Byf("Loading the e2e test configuration from %q", configFilePath)
	config = e2eConfig.LoadE2EConfig(configFilePath)
	Expect(config).ToNot(BeNil(), "e2eConfig can't be nil when calling %s test suite", testSuiteName)

	By("Setting up supervisor cluster cluster proxy")

	kubeconfigPath := config.InfraConfig.KubeconfigPath
	svClusterProxy = setupSupervisorClusterProxy(kubeconfigPath, scheme, config)
	Expect(svClusterProxy).ToNot(BeNil(), "svClusterProxy can't be nil when calling %s test suite", testSuiteName)

	By("Setting up cluster role bindings for e2e tests")

	vmsvcClusterProxy := svClusterProxy.(*common.VMServiceClusterProxy)
	err := vmservice.SetupClusterRoleBindings(vmsvcClusterProxy)
	Expect(err).ToNot(HaveOccurred(), "failed to setup cluster role bindings")

	By("Creating a new WCP namespace associated with the default storage class, VM class, and vmservice content library")

	ctx := context.TODO()

	By("Creating the VM Service content library")
	// Create the VM Service content library if it doesn't exist
	subscriptionURL := config.InfraConfig.ManagementClusterConfig.Resources.ContentLibrarySubscriptionURL
	Expect(subscriptionURL).ToNot(BeEmpty(), "contentLibrarySubscriptionURL is not set in %s", configFilePath)
	vmservice.EnsureVMServiceContentLibrary(ctx, wcpClient, subscriptionURL)

	vmClassName := config.InfraConfig.ManagementClusterConfig.Resources.VMClassName
	storageClassName := config.InfraConfig.ManagementClusterConfig.Resources.StorageClassName
	workerStorageClassName := config.InfraConfig.ManagementClusterConfig.Resources.WorkerStorageClassName

	if wcpNamespaceName == "" {
		wcpNamespaceName = config.GetVariable("E2ENamespace")
	}

	if wcpNamespaceName == "" {
		namespaceName := fmt.Sprintf("%s-%s", testSuiteName, capiutil.RandomString(6))
		wcpNamespaceCtx = createWCPNamespaceCtx(ctx, namespaceName, consts.VMServiceCLName, vmClassName, storageClassName, workerStorageClassName)
		wcpNamespaceName = wcpNamespaceCtx.GetNamespace().Name
		wcp.WaitForNamespaceReady(wcpClient, wcpNamespaceName)
	} else {
		_, err := wcpClient.GetNamespace(wcpNamespaceName)
		if err != nil {
			framework.Byf("Namespace %q does not exist, creating it", wcpNamespaceName)
			wcpNamespaceCtx = createWCPNamespaceCtx(ctx, wcpNamespaceName, consts.VMServiceCLName, vmClassName, storageClassName, workerStorageClassName)
			wcp.WaitForNamespaceReady(wcpClient, wcpNamespaceName)
		} else {
			framework.Byf("Namespace %q already exists, skipping creation", wcpNamespaceName)
		}
	}

	if workerStorageClassName != "" {
		svClient := svClusterProxy.GetClientSet()
		primarySC, err := svClient.StorageV1().StorageClasses().Get(ctx, storageClassName, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred(), "primary storage class %q must exist for worker StorageClass setup", storageClassName)
		wcp.EnsureWorkerKubernetesStorageClassIfMissing(ctx, kubeconfigPath, svClient, primarySC, workerStorageClassName, config, wcpNamespaceName, wcpClient)
	}

	By("Ensure the storage class is available in the WCP namespace")

	podVMOnStretchedSupervisorEnabled := utils.IsFssEnabled(ctx, svClusterProxy.GetClient(), config.GetVariable("VMOPNamespace"), config.GetVariable("VMOPDeploymentName"), config.GetVariable("VMOPManagerCommand"), config.GetVariable("EnvFSSPodVMOnStretchedSupervisor"))
	vmoperator.EnsureStorageClassInNamespace(ctx, svClusterProxy.GetClient(), wcpNamespaceName, storageClassName, podVMOnStretchedSupervisorEnabled)

	if workerStorageClassName != "" {
		vmoperator.EnsureStorageClassInNamespace(ctx, svClusterProxy.GetClient(), wcpNamespaceName, workerStorageClassName, podVMOnStretchedSupervisorEnabled)
	}

	By("Ensure the VM Class is available in the WCP namespace")
	Expect(vmservice.EnsureVMClassPresent(wcpClient, vmClassName)).To(Succeed())
	Expect(vmservice.EnsureNamespaceHasAccess(wcpClient, vmClassName, wcpNamespaceName)).To(Succeed())

	if os.Getenv("RUN_CANONICAL_TEST") == runCanonicalTestEnvValue {
		// Deploy an Ubuntu VM for verifying Canonical image
		By("Ensure the Canonical Ubuntu image is available in the WCP namespace")

		ubuntuImageDisplayName := config.InfraConfig.ManagementClusterConfig.Resources.UbuntuImageDisplayName
		Expect(ubuntuImageDisplayName).ToNot(BeEmpty(), "ubuntuImageDisplayName is not set in %s", configFilePath)

		ubuntuVMIName, err := vmoperator.WaitForVirtualMachineImageName(ctx, &config.Config, svClusterProxy.GetClient(), wcpNamespaceName, ubuntuImageDisplayName)
		Expect(err).ToNot(HaveOccurred(), "failed to get the Canonical Ubuntu VMI name in namespace %q", wcpNamespaceName)

		By("Deploying an Canonical Ubuntu VM under the common WCP namespace to make it available for all test specs")
		// We only deploy the VM here and not block to wait for it to be ready.
		linuxVMName = fmt.Sprintf("common-ubuntu-vm-%s", capiutil.RandomString(4))
		deployVMWithCloudInit(ctx, wcpNamespaceName, linuxVMName, ubuntuVMIName)
	} else {
		// Deploy a Photon VM
		By("Ensure the Photon image is available in the WCP namespace")

		photonImageDisplayName := config.InfraConfig.ManagementClusterConfig.Resources.PhotonImageDisplayName
		photonVMIName, err := vmoperator.WaitForVirtualMachineImageName(ctx, &config.Config, svClusterProxy.GetClient(), wcpNamespaceName, photonImageDisplayName)
		Expect(err).ToNot(HaveOccurred(), "failed to get the VMI name in namespace %q", wcpNamespaceName)

		By("Deploying a Photon VM under the common WCP namespace to make it available for all test specs")

		linuxVMName = fmt.Sprintf("common-photon-vm-%s", capiutil.RandomString(4))
		// We only deploy the VM here and not block to wait for it to be ready.
		deployVMWithCloudInit(ctx, wcpNamespaceName, linuxVMName, photonVMIName)

		if fssEnabled := utils.IsFssEnabled(ctx, svClusterProxy.GetClient(),
			config.GetVariable("VMOPNamespace"), config.GetVariable("VMOPDeploymentName"),
			config.GetVariable("VMOPManagerCommand"), config.GetVariable("EnvFSSWindowsSysprep")); fssEnabled {
			// The Windows VM deployment and customization could take a while to complete.
			// We only deploy the VM here and skip checking the VM status.
			// The actual verification will be done in vm_guestcustomization spec.
			By("Get the Windows VMI name from the image display name")

			windowsImageDisplayName := config.InfraConfig.ManagementClusterConfig.Resources.WindowsImageDisplayName
			windowsVMIName, err := vmoperator.WaitForVirtualMachineImageName(ctx, &config.Config, svClusterProxy.GetClient(), wcpNamespaceName, windowsImageDisplayName)
			Expect(err).ToNot(HaveOccurred(), "failed to get the VMI name in namespace %q", wcpNamespaceName)

			By("Deploying a Windows VM with the minimal Sysprep config")
			// Keep Windows VM name under 15 chars so hostName inherited from vmName can adhere to
			// the format specified in RFC-1034, Section 3.5 for DNS labels.
			windowsServerVMName = fmt.Sprintf("%s-%s", "windows", capiutil.RandomString(4))
			deployWindowsVMWithSysprep(ctx, wcpNamespaceName, windowsServerVMName, windowsVMIName)
		}
	}

	// Deploy a jumpbox PodVM in the WCP namespace to be used across multiple tests in NSX setup.
	if config.InfraConfig.NetworkingTopology == consts.NSX {
		By("Deploying a jumpbox PodVM in the WCP namespace")
		deployJumpboxPodVM(ctx)
	}

	// If this is a stretch supervisor cluster and namespace scoped zone is introduced (>= VC 9.0), bind zones with supervisor.
	// By default, zone-1 is already bound.
	workloadIsolationFSSEnabled = utils.IsFssEnabled(ctx, svClusterProxy.GetClient(),
		config.GetVariable("VMOPNamespace"), config.GetVariable("VMOPDeploymentName"),
		config.GetVariable("VMOPManagerCommand"), config.GetVariable("EnvWorkloadIsolation"))

	stretchedTestbed := false

	if workloadIsolationFSSEnabled && os.Getenv("STRETCHED_SUPERVISOR") == "true" {
		supervisorID := vcenter.GetSupervisorIDFromKubeconfig(ctx, kubeconfigPath)
		err := svClusterProxy.CreateMissingZoneBindingsWithSupervisor(supervisorID, []string{"zone-2", "zone-3"})
		Expect(err).ToNot(HaveOccurred(), "failed to update zone bindings with supervisor")

		// The VKS test we're using doesn't work with stretched since it does not set the
		// zones on the worker pools.
		stretchedTestbed = true
	}

	if !checkSkipVKSTests(skipTests) && !stretchedTestbed {
		vksTesting = true
	}

	return []byte(
		strings.Join([]string{
			artifactFolder,
			kubeconfigPath,
			configFilePath,
		}, ","),
	)
}, func(data []byte) {
})

var _ = SynchronizedAfterSuite(func() {
}, func() {
	// Clean up cluster role bindings (best effort)
	if svClusterProxy != nil && !skipCleanup {
		vmsvcClusterProxy := svClusterProxy.(*common.VMServiceClusterProxy)

		err := vmservice.CleanupClusterRoleBindings(vmsvcClusterProxy)
		if err != nil {
			framework.Byf("Warning: Failed to cleanup cluster role bindings: %v", err)
		}
	}

	// Do not delete the WCP namespace if the test failed for debugging purpose.
	if !CurrentSpecReport().Failed() &&
		svClusterProxy != nil &&
		wcpNamespaceCtx.GetNamespace() != nil {
		svClusterProxy.DeleteWCPNamespace(wcpNamespaceCtx)
	}

	if workloadIsolationFSSEnabled &&
		os.Getenv("STRETCHED_SUPERVISOR") == "true" &&
		svClusterProxy != nil {
		kubeconfigPath := config.InfraConfig.KubeconfigPath
		supervisorID := vcenter.GetSupervisorIDFromKubeconfig(context.Background(), kubeconfigPath)
		currentZoneList, err := svClusterProxy.GetZonesBoundWithSupervisor(supervisorID)
		Expect(err).ToNot(HaveOccurred(), "failed to get zones bound with Supervisor")

		var zonesToDelete []string

		for _, item := range currentZoneList.Zones {
			// Skip zone-1 as it's default zone which can not be deleted.
			// Can only delete workload not management zones.
			if item.Zone != "zone-1" && item.Type == "WORKLOAD" {
				zonesToDelete = append(zonesToDelete, item.Zone)
			}
		}

		err = svClusterProxy.DeleteZoneBindingsWithSupervisor(supervisorID, zonesToDelete)
		if err != nil && !strings.Contains(err.Error(), "are not bound to the Supervisor") {
			Expect(err).NotTo(HaveOccurred(), "failed to delete zone bindings with supervisor")
		}
	}
})

// setupSupervisorClusterProxy creates a WCPClusterProxyInterface instance with kubeconfig and runtime scheme.
func setupSupervisorClusterProxy(kubeconfigPath string, sc *runtime.Scheme, config *e2eConfig.E2EConfig) wcpframework.WCPClusterProxyInterface {
	var supervisorClusterProxy wcpframework.WCPClusterProxyInterface
	if framework.InfraIs(config.InfraConfig.InfraName, consts.KIND) {
		supervisorClusterProxy = wcpframework.NewSimulatedWCPClusterProxy("supervisor", kubeconfigPath, sc)
	} else {
		supervisorClusterProxy = common.NewVMServiceClusterProxy("supervisor", kubeconfigPath, sc)
		wcpClient = supervisorClusterProxy.(*common.VMServiceClusterProxy).GetWorkloadManagementAPI()
		Expect(wcpClient).ToNot(BeNil(), "wcpClient can't be nil when calling %s test suite", testSuiteName)
	}

	Expect(supervisorClusterProxy).ToNot(BeNil(), "failed to get a supervisor cluster proxy")

	return supervisorClusterProxy
}

// createWCPNamespaceCtx creates a WCP namespace with the default associations.
func createWCPNamespaceCtx(ctx context.Context, name, clName, vmClassName, storageClassName, workerStorageClassName string) wcpframework.NamespaceContext {
	clID := vmservice.GetContentLibraryUUIDByName(clName, wcpClient)
	vmsvcSpecs := wcp.NewVMServiceSpecDetails([]string{vmClassName}, []string{clID})
	wcpNamespaceCtx, err := svClusterProxy.CreateWCPNamespace(ctx, config, vmsvcSpecs, storageClassName, workerStorageClassName, name, artifactFolder)
	Expect(err).ToNot(HaveOccurred(), "Failed to create the test WCP namespace")

	return wcpNamespaceCtx
}

// deployWindowsVMWithSysprep deploys a Windows VM with the minimal Sysprep config passed.
// If WCP_VMService_v1alpha2 is enabled, it additionally deploys a v1a2 Windows VM with the same Sysprep config.
func deployWindowsVMWithSysprep(ctx context.Context, ns, vmName, vmiName string) {
	vmsvcClusterProxy := svClusterProxy.(*common.VMServiceClusterProxy)
	secretName := "sysprep-data"
	secret := manifestbuilders.Secret{
		Namespace: ns,
		Name:      secretName,
	}

	// Check if the secret already exists.
	secretObj := corev1.Secret{}

	err := vmsvcClusterProxy.GetClient().Get(ctx,
		types.NamespacedName{Namespace: ns, Name: secretName}, &secretObj)
	if err == nil {
		return
	}

	secretYaml := manifestbuilders.GetSecretYamlSysprepConfig(secret)
	Expect(vmsvcClusterProxy.CreateWithArgs(ctx, secretYaml)).To(Succeed(), "failed to create the Secret with Sysprep data", string(secretYaml))

	vmParameters := manifestbuilders.VirtualMachineYaml{
		Namespace:        ns,
		Name:             vmName,
		VMClassName:      config.InfraConfig.ManagementClusterConfig.Resources.VMClassName,
		StorageClassName: config.InfraConfig.ManagementClusterConfig.Resources.StorageClassName,
		ResourcePolicy:   config.InfraConfig.ManagementClusterConfig.Resources.VMResourcePolicyName,
		ImageName:        vmiName,
		Transport:        "Sysprep",
		SecretName:       secretName,
		PowerState:       "poweredOn",
	}
	vmYaml := manifestbuilders.GetVirtualMachineYaml(vmParameters)
	Expect(vmsvcClusterProxy.CreateWithArgs(ctx, vmYaml)).To(Succeed(), "failed to create v1alpha1 Windows VM", string(vmYaml))

	if utils.IsFssEnabled(ctx, svClusterProxy.GetClient(),
		config.GetVariable("VMOPNamespace"), config.GetVariable("VMOPDeploymentName"),
		config.GetVariable("VMOPManagerCommand"), config.GetVariable("EnvFSSV1alpha2")) {
		secretName = "inline-sysprep-data"
		secret = manifestbuilders.Secret{
			Namespace: ns,
			Name:      secretName,
		}
		secretYaml = manifestbuilders.GetSecretYamlInlineSysprepData(secret)
		Expect(vmsvcClusterProxy.CreateWithArgs(ctx, secretYaml)).To(Succeed(), "failed to create the Secret with Sysprep data", string(secretYaml))

		inlinedSysprep := fmt.Sprintf(`
        guiUnattended:
          autoLogon: true
          autoLogonCount: 1
          password:
            name: %s
            key: vmsvc-pwd
          timeZone: 004
        identification:
          joinWorkgroup: vmware
        guiRunOnce:
          commands:
          - "dir C:"
          - "echo Hello"
          - 'C:\sysprep\guestcustutil.exe restoreMountedDevices'
          - 'C:\sysprep\guestcustutil.exe flagComplete'
          - 'C:\sysprep\guestcustutil.exe deleteContainingFolder'
        userData:
          fullName: "First User"
          orgName: "Broadcom"`, secretName)
		// Keep Windows VM name under 15 chars so hostName inherited from vmName can adhere to
		// the format specified in RFC-1034, Section 3.5 for DNS labels.
		v1a2vmParameters := manifestbuilders.VirtualMachineYaml{
			Namespace:        ns,
			Name:             vmName + "-a2",
			VMClassName:      config.InfraConfig.ManagementClusterConfig.Resources.VMClassName,
			StorageClassName: config.InfraConfig.ManagementClusterConfig.Resources.StorageClassName,
			ImageName:        vmiName,
			PowerState:       "PoweredOn",
			Bootstrap: manifestbuilders.Bootstrap{
				Sysprep: &manifestbuilders.Sysprep{
					Sysprep: &inlinedSysprep,
				},
			},
		}
		vmYaml := manifestbuilders.GetVirtualMachineYamlA2(v1a2vmParameters)
		Expect(vmsvcClusterProxy.CreateWithArgs(ctx, vmYaml)).To(Succeed(), "failed to create v1alpha2 Windows VM", string(vmYaml))
	}
}

// deployVMWithCloudInit deploys a VM with the default cloud-init config passed.
func deployVMWithCloudInit(ctx context.Context, ns, vmName, vmiName string) {
	vmsvcClusterProxy := svClusterProxy.(*common.VMServiceClusterProxy)

	secretName := vmName + "-cloud-config-data"
	secret := manifestbuilders.Secret{
		Namespace: ns,
		Name:      secretName,
	}

	// Check if the secret already exists.
	secretObj := corev1.Secret{}

	err := vmsvcClusterProxy.GetClient().Get(ctx,
		types.NamespacedName{Namespace: ns, Name: secretName}, &secretObj)
	if err == nil {
		return
	}

	secretYaml := manifestbuilders.GetSecretYamlCloudConfig(secret)
	Expect(vmsvcClusterProxy.CreateWithArgs(ctx, secretYaml)).To(Succeed(), "failed to create the Secret with cloud-config data", string(secretYaml))

	vmParameters := manifestbuilders.VirtualMachineYaml{
		Namespace:        ns,
		Name:             vmName,
		ImageName:        vmiName,
		VMClassName:      config.InfraConfig.ManagementClusterConfig.Resources.VMClassName,
		StorageClassName: config.InfraConfig.ManagementClusterConfig.Resources.StorageClassName,
		ResourcePolicy:   config.InfraConfig.ManagementClusterConfig.Resources.VMResourcePolicyName,
		Transport:        "CloudInit",
		SecretName:       secretName,
		PowerState:       "poweredOn",
	}
	vmYaml := manifestbuilders.GetVirtualMachineYaml(vmParameters)
	Expect(vmsvcClusterProxy.CreateWithArgs(ctx, vmYaml)).To(Succeed(), "failed to create VM:\n%s", string(vmYaml))
}

func checkSkipVKSTests(skipStr string) bool {
	if skipStr == "" {
		return false
	}

	skippedTests := regexp.MustCompile(`\bVKS\b`)
	// If 'VKS' is specified in test-skip, then do not create a VKS cluster.
	return skippedTests.MatchString(skipStr)
}

func deployJumpboxPodVM(ctx context.Context) {
	podTemplateConfig := manifestbuilders.PodVMTemplateConfig{
		Name:      consts.JumpboxPodVMName,
		Namespace: wcpNamespaceName,
	}
	podVMYamlTemplate, err := manifestbuilders.BuildPodVMYamlTemplate(podTemplateConfig)
	Expect(err).NotTo(HaveOccurred(), "failed to build jumpbox pod template due to %v", err)
	Expect(podVMYamlTemplate).NotTo(BeEmpty())

	vmsvcClusterProxy := svClusterProxy.(*common.VMServiceClusterProxy)
	Expect(vmsvcClusterProxy.ApplyWithArgs(ctx, podVMYamlTemplate)).To(Succeed(), "failed to create jumpbox PodVM")
}
