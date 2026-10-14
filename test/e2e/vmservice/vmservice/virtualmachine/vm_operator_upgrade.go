// Copyright (c) 2026 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	e2eframework "k8s.io/kubernetes/test/e2e/framework"
	capiutil "sigs.k8s.io/cluster-api/util"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/wcp"
	"github.com/vmware-tanzu/vm-operator/test/e2e/manifestbuilders"
	"github.com/vmware-tanzu/vm-operator/test/e2e/utils"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/common"
	e2eConfig "github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/consts"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/lib/vmoperator"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/skipper"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/vmservice"
	"github.com/vmware-tanzu/vm-operator/test/e2e/wcpframework"
)

// testBlobURL is the UTS deliverable-blob endpoint used to hand the seed
// manifest off from UpgradeSeedSpec (pre-upgrade run) to UpgradeVerifySpec
// (post-upgrade run). The two specs run in separate process invocations
// around an out-of-band Supervisor/vm-operator upgrade step, so in-memory
// state can't cross that boundary -- this mirrors the publish/fetch pattern
// already used by testbed-provision/provision_helper.star for testbedInfo.json.
const testBlobURL = "https://uts-testdata.lvn.broadcom.net/v1/api/testdata/test_blob"

const upgradeSeedHTTPTimeout = 30 * time.Second

// upgradeSeedManifest is the deliverable_blob payload published by
// UpgradeSeedSpec and consumed by UpgradeVerifySpec.
type upgradeSeedManifest struct {
	Namespace string   `json:"namespace"`
	VMNames   []string `json:"vmNames"`
}

// UpgradeSeedSpecInput is the input for UpgradeSeedSpec.
type UpgradeSeedSpecInput struct {
	Config         *e2eConfig.E2EConfig
	ClusterProxy   wcpframework.WCPClusterProxyInterface
	WCPClient      wcp.WorkloadManagementAPI
	ArtifactFolder string
}

// UpgradeSeedSpec creates a small, fixed set of VirtualMachines in a
// dedicated namespace before the Supervisor/vm-operator is upgraded, and
// publishes their identity to the UTS test_blob API so that UpgradeVerifySpec
// (run after the upgrade, in a separate process) can find and verify them.
func UpgradeSeedSpec(ctx context.Context, inputGetter func() UpgradeSeedSpecInput) {
	const (
		specName = "vm-operator-upgrade-seed"
		vmCount  = 2
	)

	var (
		input            UpgradeSeedSpecInput
		wcpClient        wcp.WorkloadManagementAPI
		config           *e2eConfig.E2EConfig
		clusterProxy     *common.VMServiceClusterProxy
		svClusterClient  ctrlclient.Client
		clusterResources *e2eConfig.Resources
		namespaceName    string
	)

	BeforeEach(func() {
		input = inputGetter()
		Expect(input.Config).ToNot(BeNil(), "Invalid argument. input.Config can't be nil when calling %s spec", specName)
		Expect(input.Config.InfraConfig).ToNot(BeNil(), "Invalid argument. input.Config.InfraConfig can't be nil when calling %s spec", specName)
		skipper.SkipUnlessInfraIs(input.Config.InfraConfig.InfraName, consts.WCP)

		Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling %s spec", specName)
		Expect(input.WCPClient).ToNot(BeNil(), "Invalid argument. input.WCPClient can't be nil when calling %s spec", specName)
		Expect(os.MkdirAll(input.ArtifactFolder, 0755)).To(Succeed(), "Invalid argument. input.ArtifactFolder can't be created for %s spec", specName)

		wcpClient = input.WCPClient
		config = input.Config
		clusterResources = config.InfraConfig.ManagementClusterConfig.Resources
		clusterProxy = input.ClusterProxy.(*common.VMServiceClusterProxy)
		svClusterClient = clusterProxy.GetClient()

		cancelPodWatches := framework.WatchPodLogsAndEventsInNamespaces(ctx,
			[]string{config.GetVariable("VMOPNamespace")}, clusterProxy.GetClientSet(), filepath.Join(input.ArtifactFolder, specName))
		DeferCleanup(cancelPodWatches)

		namespaceName = fmt.Sprintf("%s-%s", specName, capiutil.RandomString(6))
	})

	// Deliberately no AfterEach cleanup here: the namespace and VMs created by
	// this spec must survive the Supervisor/vm-operator upgrade step that runs
	// (out-of-band, as a separate k8spod) between this spec and
	// UpgradeVerifySpec. UpgradeVerifySpec owns tearing this state down once
	// it has finished asserting against it.

	It("Should create a fixed set of VirtualMachines and publish their identity for post-upgrade verification", Label("upgrade-seed"), func() {
		vmserviceCLID := vmservice.GetContentLibraryUUIDByName(consts.VMServiceCLName, wcpClient)
		vmsvcSpecs := wcp.NewVMServiceSpecDetails([]string{clusterResources.VMClassName}, []string{vmserviceCLID})

		namespaceCtx, err := clusterProxy.CreateWCPNamespace(ctx, config, vmsvcSpecs,
			clusterResources.StorageClassName, clusterResources.WorkerStorageClassName, namespaceName, input.ArtifactFolder)
		Expect(err).ToNot(HaveOccurred(), "failed to create wcp namespace %s", namespaceName)
		wcp.WaitForNamespaceReady(wcpClient, namespaceName)
		// Stop watching the namespace's own create-time resources; the namespace
		// itself must outlive this spec, so we don't want its watches held open
		// past this process's lifetime.
		if cancel := namespaceCtx.GetCancelNsWatches(); cancel != nil {
			cancel()
		}

		linuxImageDisplayName := vmservice.GetDefaultImageDisplayName(clusterResources)
		linuxVMIName := vmoperator.WaitForVirtualMachineImageName(ctx, &config.Config, svClusterClient, namespaceName, linuxImageDisplayName)

		vmNames := make([]string, 0, vmCount)
		for i := 0; i < vmCount; i++ {
			vmName := fmt.Sprintf("%s-%s", specName, capiutil.RandomString(4))

			vmParameters := manifestbuilders.VirtualMachineYaml{
				Namespace:        namespaceName,
				Name:             vmName,
				ImageName:        linuxVMIName,
				VMClassName:      clusterResources.VMClassName,
				StorageClassName: clusterResources.StorageClassName,
				ResourcePolicy:   clusterResources.VMResourcePolicyName,
				PowerState:       "PoweredOn",
			}
			vmYaml := manifestbuilders.GetVirtualMachineYamlA2(vmParameters)
			Expect(clusterProxy.CreateWithArgs(ctx, vmYaml)).To(Succeed(), "failed to create virtualmachine:\n %s", string(vmYaml))

			vmoperator.WaitForVirtualMachineToExist(ctx, config, svClusterClient, namespaceName, vmName)
			vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, namespaceName, vmName, "PoweredOn")
			vmoperator.WaitForVirtualMachineIP(ctx, config, svClusterClient, namespaceName, vmName)

			vmNames = append(vmNames, vmName)
		}

		By("Publishing the seed manifest to the UTS test_blob API for UpgradeVerifySpec to consume")
		manifest := upgradeSeedManifest{
			Namespace: namespaceName,
			VMNames:   vmNames,
		}
		Expect(publishUpgradeSeedManifest(manifest)).To(Succeed(), "failed to publish upgrade seed manifest")
	})
}

// UpgradeVerifySpecInput is the input for UpgradeVerifySpec.
type UpgradeVerifySpecInput struct {
	Config         *e2eConfig.E2EConfig
	ClusterProxy   wcpframework.WCPClusterProxyInterface
	WCPClient      wcp.WorkloadManagementAPI
	ArtifactFolder string
}

// UpgradeVerifySpec runs after the Supervisor/vm-operator has been upgraded
// (by a separate k8spod step that is not part of this file). It fetches the
// manifest UpgradeSeedSpec published, confirms the seeded VirtualMachines and
// their VirtualMachineClass survived the upgrade, and then tears down the
// namespace/VMs that UpgradeSeedSpec deliberately left behind.
func UpgradeVerifySpec(ctx context.Context, inputGetter func() UpgradeVerifySpecInput) {
	const specName = "vm-operator-upgrade-verify"

	var (
		input            UpgradeVerifySpecInput
		wcpClient        wcp.WorkloadManagementAPI
		config           *e2eConfig.E2EConfig
		clusterProxy     *common.VMServiceClusterProxy
		svClusterClient  ctrlclient.Client
		clusterResources *e2eConfig.Resources
		manifest         *upgradeSeedManifest
	)

	BeforeEach(func() {
		input = inputGetter()
		Expect(input.Config).ToNot(BeNil(), "Invalid argument. input.Config can't be nil when calling %s spec", specName)
		Expect(input.Config.InfraConfig).ToNot(BeNil(), "Invalid argument. input.Config.InfraConfig can't be nil when calling %s spec", specName)
		skipper.SkipUnlessInfraIs(input.Config.InfraConfig.InfraName, consts.WCP)

		Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling %s spec", specName)
		Expect(input.WCPClient).ToNot(BeNil(), "Invalid argument. input.WCPClient can't be nil when calling %s spec", specName)
		Expect(os.MkdirAll(input.ArtifactFolder, 0755)).To(Succeed(), "Invalid argument. input.ArtifactFolder can't be created for %s spec", specName)

		wcpClient = input.WCPClient
		config = input.Config
		clusterResources = config.InfraConfig.ManagementClusterConfig.Resources
		clusterProxy = input.ClusterProxy.(*common.VMServiceClusterProxy)
		svClusterClient = clusterProxy.GetClient()

		cancelPodWatches := framework.WatchPodLogsAndEventsInNamespaces(ctx,
			[]string{config.GetVariable("VMOPNamespace")}, clusterProxy.GetClientSet(), filepath.Join(input.ArtifactFolder, specName))
		DeferCleanup(cancelPodWatches)

		blobURL := os.Getenv("UPGRADE_SEED_BLOB_URL")
		Expect(blobURL).ToNot(BeEmpty(), "UPGRADE_SEED_BLOB_URL environment variable is not set")

		var err error
		manifest, err = fetchUpgradeSeedManifest(blobURL)
		Expect(err).ToNot(HaveOccurred(), "failed to fetch upgrade seed manifest from %s", blobURL)
		Expect(manifest.Namespace).ToNot(BeEmpty(), "upgrade seed manifest has no namespace")
		Expect(manifest.VMNames).ToNot(BeEmpty(), "upgrade seed manifest has no VM names")
	})

	AfterEach(func() {
		if manifest == nil {
			return
		}

		if CurrentSpecReport().Failed() {
			for _, vmName := range manifest.VMNames {
				vmoperator.DescribeResourceIfExists(ctx, svClusterClient, clusterProxy.GetKubeconfigPath(), manifest.Namespace, vmName, "vm")
			}
		}

		// This spec is the last one to touch the state UpgradeSeedSpec created,
		// so it owns teardown. We only have the namespace name (not the
		// in-process wcpframework.NamespaceContext from the seed run, which
		// doesn't survive across process invocations), so delete via the
		// lower-level wcp API directly rather than clusterProxy.DeleteWCPNamespace.
		for _, vmName := range manifest.VMNames {
			vmYaml := manifestbuilders.GetVirtualMachineYamlA2(manifestbuilders.VirtualMachineYaml{
				Namespace: manifest.Namespace,
				Name:      vmName,
			})
			if err := clusterProxy.DeleteWithArgs(ctx, vmYaml); err != nil {
				e2eframework.Logf("failed to delete virtualmachine %s/%s: %v", manifest.Namespace, vmName, err)
				continue
			}
			vmoperator.WaitForVirtualMachineToBeDeleted(ctx, config, svClusterClient, manifest.Namespace, vmName)
		}

		wcp.DeleteNamespace(wcp.NamespaceDeleteInput{
			WCPClient: wcpClient,
			Namespace: &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: manifest.Namespace}},
		})
		wcp.WaitForNamespaceDeleted(wcpClient, manifest.Namespace)
	})

	It("Should confirm the seeded VirtualMachines and their VirtualMachineClass survived the Supervisor upgrade", Label("upgrade-verify"), func() {
		for _, vmName := range manifest.VMNames {
			vmoperator.WaitForVirtualMachineToExist(ctx, config, svClusterClient, manifest.Namespace, vmName)
			vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, manifest.Namespace, vmName, "PoweredOn")
		}

		By("Verifying the VirtualMachineClass used to create the seeded VMs is still present and healthy")
		vmClass, err := utils.GetVirtualMachineClass(ctx, svClusterClient, manifest.Namespace, clusterResources.VMClassName)
		Expect(err).ToNot(HaveOccurred(), "failed to get VirtualMachineClass %s after upgrade", clusterResources.VMClassName)
		Expect(vmClass.Spec.Hardware.Cpus).To(BeNumerically(">", 0), "VirtualMachineClass %s has no CPUs configured", clusterResources.VMClassName)

		By("Best-effort check that the Supervisor/vm-operator was actually upgraded (see TODO)")
		if err := assertSupervisorUpgraded(ctx, svClusterClient, config); err != nil {
			e2eframework.Logf("WARNING: %v", err)
		}
	})
}

// assertSupervisorUpgraded is a best-effort, intentionally incomplete
// placeholder: it only sanity-checks that the vm-operator manager Deployment
// is reachable and logs its container image, it does NOT yet assert that the
// image tag or Supervisor/WCP version actually reflects the expected
// post-upgrade state.
// TODO: once the credential/API contract from wcp_vc_upgrade.py /
// wcp_cluster_upgrade.py (vcf/tera:bora/vpx/wcp/support/cicd/) is confirmed,
// replace this with a real assertion comparing the manager Deployment's image
// tag and/or the output of `dcli namespacemanagement software clusters get`
// against the expected post-upgrade version.
func assertSupervisorUpgraded(ctx context.Context, k8sClient ctrlclient.Client, config *e2eConfig.E2EConfig) error {
	vmopNamespace := config.GetVariable("VMOPNamespace")
	vmopDeploymentName := config.GetVariable("VMOPDeploymentName")

	deployment, err := utils.GetDeployment(ctx, k8sClient, vmopNamespace, vmopDeploymentName)
	if err != nil {
		return fmt.Errorf("best-effort supervisor-upgrade check: failed to get vm-operator manager Deployment %s/%s: %w", vmopNamespace, vmopDeploymentName, err)
	}
	if len(deployment.Spec.Template.Spec.Containers) == 0 {
		return fmt.Errorf("best-effort supervisor-upgrade check: vm-operator manager Deployment %s/%s has no containers", vmopNamespace, vmopDeploymentName)
	}

	e2eframework.Logf("best-effort supervisor-upgrade check: vm-operator manager image is %q (TODO: compare against expected post-upgrade version)",
		deployment.Spec.Template.Spec.Containers[0].Image)
	return nil
}

// publishUpgradeSeedManifest posts the seed manifest to the UTS test_blob API
// using the same {test_fk, deliverable_blob} envelope as the existing
// jq/curl usage in testbed-provision/provision_helper.star, keyed off
// UTS_TEST_RUN_ID so UpgradeVerifySpec's downstream test_blob_url reference
// resolves to this run's manifest.
func publishUpgradeSeedManifest(manifest upgradeSeedManifest) error {
	testRunID := os.Getenv("UTS_TEST_RUN_ID")
	if testRunID == "" {
		return fmt.Errorf("UTS_TEST_RUN_ID environment variable is not set")
	}

	payload, err := json.Marshal(struct {
		TestFK          string              `json:"test_fk"`
		DeliverableBlob upgradeSeedManifest `json:"deliverable_blob"`
	}{
		TestFK:          testRunID,
		DeliverableBlob: manifest,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal upgrade seed manifest: %w", err)
	}

	httpClient := &http.Client{Timeout: upgradeSeedHTTPTimeout}
	resp, err := httpClient.Post(testBlobURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to POST upgrade seed manifest to %s: %w", testBlobURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d posting upgrade seed manifest to %s: %s", resp.StatusCode, testBlobURL, string(body))
	}

	return nil
}

// fetchUpgradeSeedManifest fetches the seed manifest UpgradeSeedSpec
// published, via the templated test_blob_url the UTS testsuite dependency
// wiring exposes as UPGRADE_SEED_BLOB_URL (analogous to how downstream e2e
// tests consume TESTBED_DATA_URL from testdata.depends_on...test_blob_url).
func fetchUpgradeSeedManifest(blobURL string) (*upgradeSeedManifest, error) {
	httpClient := &http.Client{Timeout: upgradeSeedHTTPTimeout}
	resp, err := httpClient.Get(blobURL)
	if err != nil {
		return nil, fmt.Errorf("failed to GET upgrade seed manifest from %s: %w", blobURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d fetching upgrade seed manifest from %s: %s", resp.StatusCode, blobURL, string(body))
	}

	var manifest upgradeSeedManifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, fmt.Errorf("failed to decode upgrade seed manifest from %s: %w", blobURL, err)
	}

	return &manifest, nil
}
