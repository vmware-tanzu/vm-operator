// Copyright (c) 2025 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"k8s.io/apimachinery/pkg/util/sets"
	capiutil "sigs.k8s.io/cluster-api/util"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1a5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	imgregv1a2 "github.com/vmware-tanzu/vm-operator/external/image-registry-operator/api/v1alpha2"
	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/testbed"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/wcp"
	"github.com/vmware-tanzu/vm-operator/test/e2e/manifestbuilders"
	"github.com/vmware-tanzu/vm-operator/test/e2e/testutils"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/common"
	e2eConfig "github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/consts"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/lib/vmoperator"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/skipper"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/vmservice"
	"github.com/vmware-tanzu/vm-operator/test/e2e/wcpframework"
)

const (
	vmGroupPubSpecName = "vm-group-publish"
)

type VMGroupPublishRequestSpecInput struct {
	Config           *e2eConfig.E2EConfig
	ClusterProxy     wcpframework.WCPClusterProxyInterface
	WCPClient        wcp.WorkloadManagementAPI
	ArtifactFolder   string
	WCPNamespaceName string
}

func VMGroupPublishRequestSpec(ctx context.Context, inputGetter func() VMGroupPublishRequestSpecInput) {
	var (
		vmGroupPubInput       VMGroupPublishRequestSpecInput
		vmSvcClusterProxy     *common.VMServiceClusterProxy
		vmSvcE2EConfig        *e2eConfig.E2EConfig
		vmSvcClusterResources *e2eConfig.Resources
		vmSvcNamespace        string
		skipCleanup           bool

		vm0Name          string
		vm1Name          string
		vmChildGroupName string
		vmGroupName      string
		vmGroupPubName   string
		targetCLID       string

		inventoryFolder *object.Folder
		inventoryCLName string

		user           *vcenter.User
		nonAdminClient ctrlclient.Client
	)

	createAndAttachWritableLocalCL := func() {
		targetCLID = vmservice.CreateLocalContentLibrary(vmGroupPubName+"-content-library", vmGroupPubInput.WCPClient)
		Expect(vmGroupPubInput.WCPClient.AssociateImageRegistryContentLibrariesToNamespace(
			vmSvcNamespace,
			wcp.ContentLibrarySpec{ContentLibrary: targetCLID, Writable: true}),
		).To(Succeed(), "failed to attach content library '%q' to namespace '%q'", targetCLID, vmSvcNamespace)
	}

	createInventoryContentLibrary := func() {
		kubeConfig := vmSvcClusterProxy.GetKubeconfigPath()

		vCenterHostname := vcenter.GetVCPNIDFromKubeconfig(ctx, kubeConfig)
		vimClient, err := vcenter.NewVimClient(
			vCenterHostname,
			testbed.AdminUsername,
			testbed.AdminPassword,
		)
		Expect(err).NotTo(HaveOccurred())

		sshCommandRunner, _, _ := testutils.GetHelpersFromKubeconfig(ctx, kubeConfig)
		user, nonAdminClient = setupNonAdminUserForTests(ctx, vimClient, sshCommandRunner, vmSvcClusterProxy.GetClient(), vmSvcClusterProxy)

		inventoryFolderName := fmt.Sprintf("%s-%s-%s", vmPubSpecName, "folder", capiutil.RandomString(4))
		inventoryCLName = fmt.Sprintf("%s-%s-%s", vmPubSpecName, "content-library", capiutil.RandomString(4))

		By("Creating library folder")

		finder := find.NewFinder(vimClient, false)
		_, inventoryFolder = createLibraryFolder(ctx, finder, inventoryFolderName)

		By("Creating an inventory content library", func() {
			inventoryCL := createInventoryContentLibraryCR(ctx, nonAdminClient, imgregv1a2.ResourceNamingStrategyPreferItemSourceID, vmSvcNamespace, inventoryCLName, inventoryFolder.Reference().Value, true, true)
			validateContentLibraryV2(ctx, nonAdminClient, inventoryCL, inventoryFolder, inventoryCLName, vmSvcNamespace, "")
		})
	}

	createVMGroups := func() {
		vmservice.CreateVMGroup(ctx, vmSvcClusterProxy, manifestbuilders.VirtualMachineGroupYaml{
			Namespace: vmSvcNamespace,
			Name:      vmChildGroupName,
			GroupName: vmGroupName,
			Members:   []vmopv1a5.GroupMember{{Kind: "VirtualMachine", Name: vm1Name}},
		})
		vmservice.CreateVMGroup(ctx, vmSvcClusterProxy, manifestbuilders.VirtualMachineGroupYaml{
			Namespace: vmSvcNamespace,
			Name:      vmGroupName,
			Members: []vmopv1a5.GroupMember{
				{Kind: "VirtualMachine", Name: vm0Name},
				{Kind: "VirtualMachineGroup", Name: vmChildGroupName},
			},
		})
	}

	createVMs := func() {
		vmservice.DeployVMWithCloudInit(ctx, vmSvcClusterProxy, vmSvcClusterResources, vmSvcNamespace, vm1Name, vmChildGroupName, nil)
		vmservice.DeployVMWithCloudInit(ctx, vmSvcClusterProxy, vmSvcClusterResources, vmSvcNamespace, vm0Name, vmGroupName, nil)
		vmoperator.VerifyVirtualMachineGroupLinked(ctx, vmSvcE2EConfig, vmSvcClusterProxy.GetClient(), vmSvcNamespace, vmGroupName, sets.New([]vmopv1a5.GroupMember{
			{Kind: "VirtualMachine", Name: vm0Name},
			{Kind: "VirtualMachineGroup", Name: vmChildGroupName},
		}...))
	}

	generateRequiredResources := func() {
		vmGroupPubName = vmGroupPubSpecName + "-" + capiutil.RandomString(4)
		vmGroupName = "vm-group-" + capiutil.RandomString(4)
		vmChildGroupName = vmGroupName + "-child-group"
		vm0Name = vmGroupName + "-vm-0"
		vm1Name = vmGroupName + "-vm-1"

		createAndAttachWritableLocalCL()
		createVMGroups()
		createVMs()
	}

	skipChecks := func() {
		skipper.SkipUnlessInfraIs(vmGroupPubInput.Config.InfraConfig.InfraName, consts.WCP)
		skipper.SkipUnlessVMImageRegistryFSSEnabled(ctx, vmSvcClusterProxy.GetClient(), vmSvcE2EConfig)
		skipper.SkipUnlessSupervisorCapabilityEnabled(ctx, vmSvcClusterProxy, consts.VMGroupsCapabilityName)
		skipper.SkipUnlessSupervisorCapabilityEnabled(ctx, vmSvcClusterProxy, consts.InventoryContentLibraryCapabilityName)

		skipCleanup = false
	}

	BeforeEach(func() {
		vmGroupPubInput = inputGetter()
		Expect(vmGroupPubInput.Config).ToNot(BeNil(), "Invalid argument. input.E2EConfig can't be nil when calling %s spec", vmGroupPubSpecName)
		Expect(vmGroupPubInput.Config.InfraConfig).ToNot(BeNil(), "Invalid argument. input.E2EConfig.InfraConfig can't be nil when calling %s spec", vmGroupPubSpecName)
		Expect(vmGroupPubInput.Config.InfraConfig.ManagementClusterConfig).ToNot(BeNil(), "Invalid argument. input.E2EConfig.InfraConfig.ManagementClusterConfig can't be nil when calling %s spec", vmGroupPubSpecName)
		Expect(vmGroupPubInput.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.SVClusterProxy can't be nil when calling %s spec", vmGroupPubSpecName)
		Expect(vmGroupPubInput.WCPNamespaceName).ToNot(BeEmpty(), "Invalid argument. input.WCPNamespaceName can't be empty when calling %s spec", vmGroupPubSpecName)
		Expect(os.MkdirAll(vmGroupPubInput.ArtifactFolder, 0755)).To(Succeed(), "Invalid argument. input.ArtifactFolder can't be created for %s spec", vmGroupPubSpecName)

		vmSvcClusterProxy = vmGroupPubInput.ClusterProxy.(*common.VMServiceClusterProxy)
		vmSvcE2EConfig = vmGroupPubInput.Config
		vmSvcClusterResources = vmSvcE2EConfig.InfraConfig.ManagementClusterConfig.Resources
		vmSvcNamespace = vmGroupPubInput.WCPNamespaceName
		skipCleanup = true

		skipChecks()

		vmGroupCancelPodWatches := framework.WatchPodLogsAndEventsInNamespaces(
			ctx,
			[]string{vmSvcE2EConfig.GetVariable("VMOPNamespace")},
			vmSvcClusterProxy.GetClientSet(),
			filepath.Join(vmGroupPubInput.ArtifactFolder, vmGroupPubSpecName))
		DeferCleanup(vmGroupCancelPodWatches)

		generateRequiredResources()
	})

	deleteVMGroups := func() {
		vmservice.DeleteVMGroup(ctx, vmSvcClusterProxy, vmSvcE2EConfig, manifestbuilders.VirtualMachineGroupYaml{
			Namespace: vmSvcNamespace,
			Name:      vmChildGroupName,
		})
		vmservice.DeleteVMGroup(ctx, vmSvcClusterProxy, vmSvcE2EConfig, manifestbuilders.VirtualMachineGroupYaml{
			Namespace: vmSvcNamespace,
			Name:      vmGroupName,
		})
	}

	detachAndDeleteWritableLocalCL := func() {
		Expect(vmGroupPubInput.WCPClient.DisassociateImageRegistryContentLibrariesFromNamespace(vmSvcNamespace, targetCLID)).To(
			Succeed(), "failed to detach content library '%s' from namespace '%s'", targetCLID, vmSvcNamespace)
		Expect(vmGroupPubInput.WCPClient.DeleteLocalContentLibrary(targetCLID)).To(
			Succeed(), "failed to delete the publish content library, CL ID: %s", targetCLID)
	}

	deleteInventoryFolder := func() {
		if inventoryFolder != nil {
			vcenter.DeleteFolder(ctx, inventoryFolder)
			inventoryFolder = nil
			inventoryCLName = ""
		}
	}

	cleanupRequiredResources := func() {
		deleteVMGroups()
		detachAndDeleteWritableLocalCL()
		deleteInventoryFolder()
	}

	AfterEach(func() {
		if skipCleanup {
			return
		}

		cleanupRequiredResources()
	})

	vmGroupPublish := func(groupPubName, target string, vms []string) {
		vmservice.CreateVMGroupPub(ctx, vmSvcClusterProxy, manifestbuilders.VirtualMachineGroupPublishRequestYaml{
			Namespace:       vmSvcNamespace,
			Name:            groupPubName,
			Source:          vmGroupName,
			Target:          target,
			VirtualMachines: vms,
		}, "")
		vmoperator.VerifyVirtualMachineGroupPublishRequestCompleted(
			ctx,
			vmSvcE2EConfig,
			vmSvcClusterProxy.GetClient(),
			vmSvcNamespace,
			groupPubName)

		By("set spec.TTLSecondsAfterFinished should delete the vm group publish request after TTL")
		vmservice.UpdateVMGroupPub(ctx, vmSvcClusterProxy, manifestbuilders.VirtualMachineGroupPublishRequestYaml{
			Namespace:               vmSvcNamespace,
			Name:                    groupPubName,
			Source:                  vmGroupName,
			Target:                  target,
			TTLSecondsAfterFinished: int64(1),
		})
		vmoperator.VerifyVirtualMachineGroupPublishRequestDeleted(
			ctx,
			vmSvcE2EConfig,
			vmSvcClusterProxy.GetClient(),
			vmSvcNamespace,
			groupPubName)
	}

	It("should succeed when vm group publish request with created with proper inputs", Label("smoke"), func() {
		By("create a vm group publish without a content library should fail when there is no default content library")
		vmservice.CreateVMGroupPub(ctx, vmSvcClusterProxy, manifestbuilders.VirtualMachineGroupPublishRequestYaml{
			Namespace: vmSvcNamespace,
			Name:      vmGroupPubName,
			Source:    vmGroupName,
			Target:    "",
		}, "cannot find a default content library with the \"imageregistry.vmware.com/default\" label")

		targetCLName, err := vmservice.GetK8sContentLibraryNameByUUID(
			ctx,
			vmSvcE2EConfig,
			vmSvcClusterProxy.GetClient(),
			vmSvcNamespace,
			targetCLID)
		Expect(err).NotTo(HaveOccurred(), "failed to get content library name from it's ID")
		Expect(targetCLName).NotTo(BeEmpty(), "new publishing content library ID is empty")

		By("create a vm group publish without a source vm group should fail when there is no vm group with the same name as the request")
		vmservice.CreateVMGroupPub(ctx, vmSvcClusterProxy, manifestbuilders.VirtualMachineGroupPublishRequestYaml{
			Namespace: vmSvcNamespace,
			Name:      vmGroupPubName,
			Source:    "",
			Target:    targetCLName,
		}, fmt.Sprintf("VirtualMachineGroup.vmoperator.vmware.com %q not found", vmGroupPubName))

		By("create a vm group publish with vms that are not in the vm group should fail")
		vmservice.CreateVMGroupPub(ctx, vmSvcClusterProxy, manifestbuilders.VirtualMachineGroupPublishRequestYaml{
			Namespace: vmSvcNamespace,
			Name:      vmGroupPubName,
			Source:    vmGroupName,
			Target:    targetCLName,
			VirtualMachines: []string{
				fmt.Sprintf("vm-%s", capiutil.RandomString(4)),
				fmt.Sprintf("vm-%s", capiutil.RandomString(4))},
		}, "virtual machines must be a direct or indirect member of the source group")

		By("create a vm group publish with source vm group and content library defined per created resource should succeed")
		// all VMs are published when the spec.virtualMachines is omitted
		vmGroupPublish(vmGroupPubName, targetCLName, []string{})

		By("create a vm group publish with a subset of VMs from the source group should succeed")
		vmGroupPublish(vmGroupPubName+"-subset", targetCLName, []string{vm0Name})

		By("create a vm group publish with target inventory library should succeed")
		createInventoryContentLibrary()
		vmGroupPublish(vmGroupPubName+"-inventory", inventoryCLName, []string{vm0Name})

		By("Deleting non admin user")
		vcenter.DeleteUserOrFail(user)
	})
}
