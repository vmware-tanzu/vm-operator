// Copyright (c) 2023-2025 Broadcom. All Rights Reserved.
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
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	e2eframework "k8s.io/kubernetes/test/e2e/framework"
	capiutil "sigs.k8s.io/cluster-api/util"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	imgregv1a2 "github.com/vmware-tanzu/vm-operator/external/image-registry-operator/api/v1alpha2"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
	libssh "github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/ssh"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/testbed"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/wcp"
	"github.com/vmware-tanzu/vm-operator/test/e2e/manifestbuilders"
	"github.com/vmware-tanzu/vm-operator/test/e2e/testutils"
	"github.com/vmware-tanzu/vm-operator/test/e2e/utils"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/common"
	e2eConfig "github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/consts"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/lib/vmoperator"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/skipper"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/vmservice"
	"github.com/vmware-tanzu/vm-operator/test/e2e/wcpframework"
)

const (
	vmPubSpecName       = "vmpub"
	vmPubTargetItemName = "vm-publish-request-target-item-name"
)

type VMPublishRequestSpecInput struct {
	ClusterProxy     wcpframework.WCPClusterProxyInterface
	Config           *e2eConfig.E2EConfig
	WCPClient        wcp.WorkloadManagementAPI
	ArtifactFolder   string
	SkipCleanup      bool
	LinuxVMName      string
	WCPNamespaceName string
}

func VMPublishRequestSpec(ctx context.Context, inputGetter func() VMPublishRequestSpecInput) {
	Context("VirtualMachinePublishRequest", Ordered, func() {
		var (
			input            VMPublishRequestSpecInput
			wcpClient        wcp.WorkloadManagementAPI
			config           *e2eConfig.E2EConfig
			clusterProxy     *common.VMServiceClusterProxy
			svClusterConfig  *e2eConfig.ManagementClusterConfig
			svClusterClient  ctrlclient.Client
			clusterResources *e2eConfig.Resources
			vimClient        *vim25.Client

			targetContentLibraryName string
			vmPublishRequestName     string
		)

		BeforeAll(func() {
			input = inputGetter()
			Expect(input.Config).ToNot(BeNil(), "Invalid argument. input.E2EConfig can't be nil when calling %s spec", CurrentSpecReport)
			Expect(input.Config.InfraConfig).ToNot(BeNil(), "Invalid argument. input.E2EConfig.InfraConfig can't be nil when calling %s spec", CurrentSpecReport)
			skipper.SkipUnlessInfraIs(input.Config.InfraConfig.InfraName, consts.WCP)

			Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.SVClusterProxy can't be nil when calling %s spec", CurrentSpecReport)
			Expect(input.WCPNamespaceName).ToNot(BeEmpty(), "Invalid argument. input.WCPNamespaceName can't be empty when calling %s spec", CurrentSpecReport)
			Expect(input.LinuxVMName).ToNot(BeEmpty(), "Invalid argument. input.LinuxVMName can't be empty when calling %s spec", CurrentSpecReport)
			Expect(os.MkdirAll(input.ArtifactFolder, 0755)).To(Succeed(), "Invalid argument. input.ArtifactFolder can't be created for %s spec", CurrentSpecReport)

			wcpClient = input.WCPClient
			config = input.Config
			clusterProxy = input.ClusterProxy.(*common.VMServiceClusterProxy)
			svClusterConfig = config.InfraConfig.ManagementClusterConfig
			clusterResources = svClusterConfig.Resources

			// Skip testing if WCP_VM_Image_Registry FSS is not enabled.
			svClusterClient = clusterProxy.GetClient()
			skipper.SkipUnlessVMImageRegistryFSSEnabled(ctx, svClusterClient, config)

			cancelPodWatches := framework.WatchPodLogsAndEventsInNamespaces(ctx, []string{config.GetVariable("VMOPNamespace")}, input.ClusterProxy.GetClientSet(), filepath.Join(input.ArtifactFolder, vmPubSpecName))
			DeferCleanup(cancelPodWatches)
		})

		BeforeEach(func() {
			targetContentLibraryName = fmt.Sprintf("%s-%s-%s", vmPubSpecName, "content-library", capiutil.RandomString(4))
			vmPublishRequestName = fmt.Sprintf("%s-%s", vmPubSpecName, capiutil.RandomString(4))
		})

		AfterEach(func() {
			if CurrentSpecReport().Failed() {
				vmoperator.DescribeResourceIfExists(ctx, svClusterClient, clusterProxy.GetKubeconfigPath(), input.WCPNamespaceName, input.LinuxVMName, "vm")
			}
		})

		Context("CLS Content Library", Ordered, func() {
			var (
				targetLocationCLID           string
				tarLocationCLIsAttached      bool
				keepTargetLocationCLAttached bool
			)

			BeforeEach(func() {
				if targetLocationCLID == "" {
					targetLocationCLID = vmservice.CreateLocalContentLibrary(targetContentLibraryName, wcpClient)
				}
			})

			AfterEach(func() {
				// Detach the content library from the namespace if attached and not keeping it attached.
				if tarLocationCLIsAttached && !keepTargetLocationCLAttached {
					Expect(wcpClient.DisassociateImageRegistryContentLibrariesFromNamespace(input.WCPNamespaceName, targetLocationCLID)).To(Succeed(), "failed to detach content library '%s' from namespace '%s'", targetLocationCLID, input.WCPNamespaceName)

					// Wait for the namespace update to fully propagate before deleting the CL.
					// The dcli namespace update is asynchronous — vCenter marks the CL "in use"
					// until the namespace config_status returns to RUNNING.
					wcp.WaitForNamespaceReady(wcpClient, input.WCPNamespaceName)

					tarLocationCLIsAttached = false
				}

				// Delete the content library if exists and not keeping it attached to the namespace.
				if targetLocationCLID != "" && !keepTargetLocationCLAttached {
					if err := wcpClient.DeleteLocalContentLibrary(targetLocationCLID); err != nil {
						// Log but don't fail — the CL may be temporarily in use by a parallel
						// runner (NotAllowedInCurrentState) or already deleted (NotFound).
						GinkgoWriter.Printf("Warning: failed to delete publish content library %s: %v\n", targetLocationCLID, err)
					}
					targetLocationCLID = ""
				}

				vmoperator.DeleteVirtualMachinePublishRequest(ctx, svClusterClient, input.WCPNamespaceName, vmPublishRequestName)
				vmoperator.WaitForVirtualMachinePublishRequestToBeDeleted(ctx, config, svClusterClient, input.WCPNamespaceName, vmPublishRequestName)
			})

			It("should set default values for source VM name and target item", func() {
				vmPubReqBuilder := generateVMPublishRequestBuilder(input.WCPNamespaceName, vmPublishRequestName, "", "", "fake-cl")
				createVMPublishRequest(ctx, *config, svClusterClient, *clusterProxy, vmPubReqBuilder)

				Expect(vmoperator.GetVirtualMachinePublishRequestSourceName(ctx, config, svClusterClient, input.WCPNamespaceName, vmPublishRequestName)).To(Equal(vmPublishRequestName))
				Expect(vmoperator.GetVirtualMachinePublishRequestTargetItemName(ctx, config, svClusterClient, input.WCPNamespaceName, vmPublishRequestName)).To(Equal(vmPublishRequestName + "-image"))
			})

			It("should have expected condition when the source VM doesn't exist", func() {
				// This will attach the content library to the namespace without affecting the existing ones.
				// And it will be detached from the namespace in AfterEach().
				Expect(wcpClient.AssociateImageRegistryContentLibrariesToNamespace(input.WCPNamespaceName, wcp.ContentLibrarySpec{
					ContentLibrary: targetLocationCLID,
					Writable:       true,
				})).To(Succeed(), "failed to attach content library '%s' to namespace '%s'", targetLocationCLID, input.WCPNamespaceName)

				tarLocationCLIsAttached = true

				targetLocationK8sCLName, err := vmservice.GetK8sContentLibraryNameByUUID(ctx, config, svClusterClient, input.WCPNamespaceName, targetLocationCLID)
				Expect(err).NotTo(HaveOccurred(), "failed to get the CL that is attached to the namespace")

				vmPubReqBuilder := generateVMPublishRequestBuilder(input.WCPNamespaceName, vmPublishRequestName, "fake-vm", vmPubTargetItemName, targetLocationK8sCLName)
				createVMPublishRequest(ctx, *config, svClusterClient, *clusterProxy, vmPubReqBuilder)

				vmPubCondition := metav1.Condition{
					Type:   vmopv1.VirtualMachinePublishRequestConditionSourceValid,
					Status: metav1.ConditionFalse,
					Reason: vmopv1.SourceVirtualMachineNotExistReason,
				}
				vmoperator.VerifyVirtualMachinePublishRequestCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmPublishRequestName, vmPubCondition)
			})

			It("should have expected condition when target location content library does not exist", func() {
				vmPubReqBuilder := generateVMPublishRequestBuilder(input.WCPNamespaceName, vmPublishRequestName, input.LinuxVMName, vmPubTargetItemName, "non-existing-content-library")
				createVMPublishRequest(ctx, *config, svClusterClient, *clusterProxy, vmPubReqBuilder)

				vmPubCondition := metav1.Condition{
					Type:   vmopv1.VirtualMachinePublishRequestConditionTargetValid,
					Status: metav1.ConditionFalse,
					Reason: vmopv1.TargetContentLibraryNotExistReason,
				}
				vmoperator.VerifyVirtualMachinePublishRequestCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmPublishRequestName, vmPubCondition)
			})

			It("should have expected condition when target location content library exists but not writable", func() {
				// This will attach the content library to the namespace without affecting the existing ones.
				// And it will be detached from the namespace in AfterEach().
				Expect(wcpClient.AssociateImageRegistryContentLibrariesToNamespace(input.WCPNamespaceName, wcp.ContentLibrarySpec{
					ContentLibrary: targetLocationCLID,
					Writable:       false,
				})).To(Succeed(), "failed to attach content library '%s' to namespace '%s'", targetLocationCLID, input.WCPNamespaceName)

				tarLocationCLIsAttached = true

				targetLocationK8sCLName, err := vmservice.GetK8sContentLibraryNameByUUID(ctx, config, svClusterClient, input.WCPNamespaceName, targetLocationCLID)
				Expect(err).NotTo(HaveOccurred(), "failed to get the CL that is attached to the namespace")

				vmPubReqBuilder := generateVMPublishRequestBuilder(input.WCPNamespaceName, vmPublishRequestName, input.LinuxVMName, vmPubTargetItemName, targetLocationK8sCLName)
				createVMPublishRequest(ctx, *config, svClusterClient, *clusterProxy, vmPubReqBuilder)

				vmPubCondition := metav1.Condition{
					Type:   vmopv1.VirtualMachinePublishRequestConditionTargetValid,
					Status: metav1.ConditionFalse,
					Reason: vmopv1.TargetContentLibraryNotWritableReason,
				}
				vmoperator.VerifyVirtualMachinePublishRequestCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmPublishRequestName, vmPubCondition)
			})

			It("should publish the VM when all conditions meet and successfully deploy from the published VM", Label("smoke"), func() {
				Expect(wcpClient.AssociateImageRegistryContentLibrariesToNamespace(input.WCPNamespaceName, wcp.ContentLibrarySpec{
					ContentLibrary: targetLocationCLID,
					Writable:       true,
				})).To(Succeed(), "failed to attach content library '%s' to namespace '%s'", targetLocationCLID, input.WCPNamespaceName)

				tarLocationCLIsAttached = true

				targetLocationK8sCLName, err := vmservice.GetK8sContentLibraryNameByUUID(ctx, config, svClusterClient, input.WCPNamespaceName, targetLocationCLID)
				Expect(err).NotTo(HaveOccurred(), "failed to get the CL that is attached to the namespace")

				vmPubReqBuilder := generateVMPublishRequestBuilder(input.WCPNamespaceName, vmPublishRequestName, input.LinuxVMName, vmPubTargetItemName, targetLocationK8sCLName)
				createVMPublishRequest(ctx, *config, svClusterClient, *clusterProxy, vmPubReqBuilder)

				vmPubCondition := metav1.Condition{
					Type:   vmopv1.VirtualMachinePublishRequestConditionComplete,
					Status: metav1.ConditionTrue,
				}
				vmoperator.VerifyVirtualMachinePublishRequestCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmPublishRequestName, vmPubCondition)

				// Ensure the published image is available with expected display name under the namespace.
				expectedPublishedImageCRName, err := vmoperator.GetVirtualMachinePublishRequestTargetItemName(ctx, config, svClusterClient, input.WCPNamespaceName, vmPublishRequestName)
				Expect(err).NotTo(HaveOccurred(), "failed to get the published target item name in namespace %q", input.WCPNamespaceName)
				publishedImageCRName := vmoperator.WaitForVirtualMachineImageName(ctx, &config.Config, svClusterClient, input.WCPNamespaceName, expectedPublishedImageCRName)
				Expect(publishedImageCRName).NotTo(BeEmpty(), "published VM Image resource name is empty")
				vmoperator.WaitForOVFVirtualMachineImageReady(ctx, &config.Config, svClusterClient, input.WCPNamespaceName, publishedImageCRName)

				// Keep the published image attached to the namespace for the next test case.
				keepTargetLocationCLAttached = true

				// Use the published the vmi to deploy a new VM should succeed with VM powered on and IP assigned.
				newVmName := fmt.Sprintf("%s-%s", vmPubSpecName+"-vm", capiutil.RandomString(4))
				newVMBuilder := generateVMBuilder(input.WCPNamespaceName, newVmName, publishedImageCRName, *clusterResources)
				newVmYaml := manifestbuilders.GetVirtualMachineYamlA2(newVMBuilder)
				Expect(clusterProxy.CreateWithArgs(ctx, newVmYaml)).NotTo(HaveOccurred(), "failed to create virtualmachine from the published image", string(newVmYaml))
				vmoperator.WaitForVirtualMachineCreation(ctx, config, svClusterClient, input.WCPNamespaceName, newVmName)
				vmoperator.DeleteVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, newVmName)
			})

			It("should have expected condition when the published target item already exists in the content library", func() {
				// Attach the target CL as writable — this test is self-sufficient and
				// does not rely on the smoke test having run first.
				if !tarLocationCLIsAttached {
					Expect(wcpClient.AssociateImageRegistryContentLibrariesToNamespace(input.WCPNamespaceName, wcp.ContentLibrarySpec{
						ContentLibrary: targetLocationCLID,
						Writable:       true,
					})).To(Succeed(), "failed to attach content library '%s' to namespace '%s'", targetLocationCLID, input.WCPNamespaceName)
					tarLocationCLIsAttached = true
				}

				// Reset this before any error occurs below to ensure the CL will be deleted in AfterEach().
				keepTargetLocationCLAttached = false

				targetLocationK8sCLName, err := vmservice.GetK8sContentLibraryNameByUUID(ctx, config, svClusterClient, input.WCPNamespaceName, targetLocationCLID)
				Expect(err).NotTo(HaveOccurred(), "failed to get the CL that is attached to the namespace")

				// Publish the VM once so that the target item exists in the CL.
				firstPubReqName := fmt.Sprintf("%s-first", vmPublishRequestName)
				firstPubReqBuilder := generateVMPublishRequestBuilder(input.WCPNamespaceName, firstPubReqName, input.LinuxVMName, vmPubTargetItemName, targetLocationK8sCLName)
				createVMPublishRequest(ctx, *config, svClusterClient, *clusterProxy, firstPubReqBuilder)
				vmoperator.VerifyVirtualMachinePublishRequestCondition(ctx, config, svClusterClient, input.WCPNamespaceName, firstPubReqName, metav1.Condition{
					Type:   vmopv1.VirtualMachinePublishRequestConditionComplete,
					Status: metav1.ConditionTrue,
				})
				DeferCleanup(func() {
					vmoperator.DeleteVirtualMachinePublishRequest(ctx, svClusterClient, input.WCPNamespaceName, firstPubReqName)
					vmoperator.WaitForVirtualMachinePublishRequestToBeDeleted(ctx, config, svClusterClient, input.WCPNamespaceName, firstPubReqName)
				})

				// Now publish again with the same target item name — expect duplicate error.
				vmPubReqBuilder := generateVMPublishRequestBuilder(input.WCPNamespaceName, vmPublishRequestName, input.LinuxVMName, vmPubTargetItemName, targetLocationK8sCLName)
				createVMPublishRequest(ctx, *config, svClusterClient, *clusterProxy, vmPubReqBuilder)

				vmPubCondition := metav1.Condition{
					Type:   vmopv1.VirtualMachinePublishRequestConditionTargetValid,
					Status: metav1.ConditionFalse,
					Reason: vmopv1.TargetItemAlreadyExistsReason,
				}
				vmoperator.VerifyVirtualMachinePublishRequestCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmPublishRequestName, vmPubCondition)
			})

			It("should preserve vAppConfig properties on a VM deployed from the published image", Label("extended-functional","experimental"), func() {
				skipper.SkipUnlessV1a2FSSEnabled(ctx, svClusterClient, config)

				if !tarLocationCLIsAttached {
					Expect(wcpClient.AssociateImageRegistryContentLibrariesToNamespace(input.WCPNamespaceName, wcp.ContentLibrarySpec{
						ContentLibrary: targetLocationCLID,
						Writable:       true,
					})).To(Succeed(), "failed to attach content library '%s' to namespace '%s'", targetLocationCLID, input.WCPNamespaceName)
					tarLocationCLIsAttached = true
				}

				targetLocationK8sCLName, err := vmservice.GetK8sContentLibraryNameByUUID(ctx, config, svClusterClient, input.WCPNamespaceName, targetLocationCLID)
				Expect(err).NotTo(HaveOccurred(), "failed to get the CL that is attached to the namespace")

				sourceImageName, err := vmoperator.WaitForVirtualMachineImageName(ctx, &config.Config, svClusterClient, input.WCPNamespaceName, vmservice.GetDefaultImageDisplayName(clusterResources))
				Expect(err).NotTo(HaveOccurred(), "failed to get the default VM Image name in namespace %q", input.WCPNamespaceName)

				// Explicit vAppConfig properties to set on the source VM before publishing.
				expectedVAppProperties := []manifestbuilders.KeyValueOrSecretKeySelectorPair{
					{
						Key: "prop-1",
						Value: manifestbuilders.ValueOrSecretKeySelector{
							Value: "my-val-1",
						},
					},
				}

				sourceVMName := fmt.Sprintf("%s-%s", vmPubSpecName+"-vapp-src", capiutil.RandomString(4))
				sourceVMBuilder := manifestbuilders.VirtualMachineYaml{
					Namespace:        input.WCPNamespaceName,
					Name:             sourceVMName,
					ImageName:        sourceImageName,
					VMClassName:      clusterResources.VMClassName,
					StorageClassName: clusterResources.StorageClassName,
					ResourcePolicy:   clusterResources.VMResourcePolicyName,
					PowerState:       "PoweredOn",
					Bootstrap: manifestbuilders.Bootstrap{
						// LinuxPrep is needed here for the VM to get a valid IP address.
						LinuxPrep: &manifestbuilders.LinuxPrep{},
						VAppConfig: &manifestbuilders.VAppConfig{
							Properties: &expectedVAppProperties,
						},
					},
				}
				sourceVMYaml := manifestbuilders.GetVirtualMachineYamlA2(sourceVMBuilder)
				Expect(clusterProxy.CreateWithArgs(ctx, sourceVMYaml)).NotTo(HaveOccurred(), "failed to create source virtualmachine with vAppConfig", string(sourceVMYaml))
				vmoperator.WaitForVirtualMachineCreation(ctx, config, svClusterClient, input.WCPNamespaceName, sourceVMName)
				DeferCleanup(func() {
					vmoperator.DeleteVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, sourceVMName)
				})

				vAppPubReqName := fmt.Sprintf("%s-vapp", vmPublishRequestName)
				vAppTargetItemName := fmt.Sprintf("%s-vapp-item", vmPubTargetItemName)
				vmPubReqBuilder := generateVMPublishRequestBuilder(input.WCPNamespaceName, vAppPubReqName, sourceVMName, vAppTargetItemName, targetLocationK8sCLName)
				createVMPublishRequest(ctx, *config, svClusterClient, *clusterProxy, vmPubReqBuilder)
				DeferCleanup(func() {
					vmoperator.DeleteVirtualMachinePublishRequest(ctx, svClusterClient, input.WCPNamespaceName, vAppPubReqName)
					vmoperator.WaitForVirtualMachinePublishRequestToBeDeleted(ctx, config, svClusterClient, input.WCPNamespaceName, vAppPubReqName)
				})

				vmoperator.VerifyVirtualMachinePublishRequestCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vAppPubReqName, metav1.Condition{
					Type:   vmopv1a2.VirtualMachinePublishRequestConditionComplete,
					Status: metav1.ConditionTrue,
				})

				expectedPublishedImageCRName, err := vmoperator.GetVirtualMachinePublishRequestTargetItemName(ctx, config, svClusterClient, input.WCPNamespaceName, vAppPubReqName)
				Expect(err).NotTo(HaveOccurred(), "failed to get the published target item name in namespace %q", input.WCPNamespaceName)
				publishedImageCRName, err := vmoperator.WaitForVirtualMachineImageName(ctx, &config.Config, svClusterClient, input.WCPNamespaceName, expectedPublishedImageCRName)
				Expect(err).NotTo(HaveOccurred(), "failed to get the VMI name in namespace %q", input.WCPNamespaceName)
				Expect(publishedImageCRName).NotTo(BeEmpty(), "published VM Image resource name is empty")
				vmoperator.WaitForOVFVirtualMachineImageReady(ctx, &config.Config, svClusterClient, input.WCPNamespaceName, publishedImageCRName)

				// Deploy a new VM from the published image without any bootstrap overrides so the
				// deployed VM's vApp properties come purely from the defaults captured during publish.
				deployedVMName := fmt.Sprintf("%s-%s", vmPubSpecName+"-vapp-vm", capiutil.RandomString(4))
				deployedVMBuilder := generateVMBuilder(input.WCPNamespaceName, deployedVMName, publishedImageCRName, *clusterResources)
				deployedVMYaml := manifestbuilders.GetVirtualMachineYamlA2(deployedVMBuilder)
				Expect(clusterProxy.CreateWithArgs(ctx, deployedVMYaml)).NotTo(HaveOccurred(), "failed to create virtualmachine from the published image", string(deployedVMYaml))
				vmoperator.WaitForVirtualMachineCreation(ctx, config, svClusterClient, input.WCPNamespaceName, deployedVMName)
				DeferCleanup(func() {
					vmoperator.DeleteVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, deployedVMName)
				})

				vmmoid := vmoperator.GetVirtualMachineMOID(ctx, svClusterClient, input.WCPNamespaceName, deployedVMName)
				vCenterClient := vcenter.NewVimClientFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath())
				verifyVAppConfigs(ctx, vCenterClient, vmmoid, expectedVAppProperties)
			})
		})

		Context("Inventory Content Library", Ordered, func() {
			var (
				inventoryFolderName string

				inventoryFolder *object.Folder
				inventoryCL     *imgregv1a2.ContentLibrary

				// dupPublishRequestName holds the name of any secondary publish request
				// created during a test. AfterEach deletes it so that cleanup is
				// complete even when the It body itself did not delete it.
				dupPublishRequestName string

				user           *vcenter.User
				nonAdminClient ctrlclient.Client
			)

			BeforeAll(func() {
				skipper.SkipUnlessSupervisorCapabilityEnabled(ctx, clusterProxy, consts.InventoryContentLibraryCapabilityName)

				kubeConfig := clusterProxy.GetKubeconfigPath()
				svClusterClient = clusterProxy.GetClient()

				var err error

				vCenterHostname := vcenter.GetVCPNIDFromKubeconfig(ctx, kubeConfig)
				vimClient, err = vcenter.NewVimClient(
					vCenterHostname,
					testbed.AdminUsername,
					testbed.AdminPassword,
				)
				Expect(err).NotTo(HaveOccurred())

				sshCommandRunner, _, _ := testutils.GetHelpersFromKubeconfig(ctx, kubeConfig)
				user, nonAdminClient = setupNonAdminUserForTests(ctx, vimClient, sshCommandRunner, svClusterClient, clusterProxy)
			})

			AfterAll(func() {
				By("Deleting non admin user")
				vcenter.DeleteUserOrFail(user)
			})

			BeforeEach(func() {
				if inventoryFolder == nil {
					inventoryFolderName = fmt.Sprintf("%s-%s-%s", vmPubSpecName, "folder", capiutil.RandomString(4))

					By("Creating library folder")

					finder := find.NewFinder(vimClient, false)
					_, inventoryFolder = createLibraryFolder(ctx, finder, inventoryFolderName)
				}

				if inventoryCL == nil {
					By("Creating an inventory content library", func() {
						inventoryCL = createInventoryContentLibraryCR(ctx, nonAdminClient, imgregv1a2.ResourceNamingStrategyPreferItemSourceID, input.WCPNamespaceName, targetContentLibraryName, inventoryFolder.Reference().Value, true, true)
						// Validate CL itself exists and reconciled.
						validateContentLibraryV2(ctx, nonAdminClient, inventoryCL, inventoryFolder, targetContentLibraryName, input.WCPNamespaceName, "")
					})
				}
			})

			AfterEach(func() {
				if inventoryFolder != nil {
					vcenter.DeleteFolder(ctx, inventoryFolder)

					inventoryFolderName = ""
					targetContentLibraryName = ""

					inventoryFolder = nil
					inventoryCL = nil
				}

				vmoperator.DeleteVirtualMachinePublishRequest(ctx, svClusterClient, input.WCPNamespaceName, vmPublishRequestName)
				vmoperator.WaitForVirtualMachinePublishRequestToBeDeleted(ctx, config, svClusterClient, input.WCPNamespaceName, vmPublishRequestName)

				if dupPublishRequestName != "" {
					vmoperator.DeleteVirtualMachinePublishRequest(ctx, svClusterClient, input.WCPNamespaceName, dupPublishRequestName)
					vmoperator.WaitForVirtualMachinePublishRequestToBeDeleted(ctx, config, svClusterClient, input.WCPNamespaceName, dupPublishRequestName)
					dupPublishRequestName = ""
				}
			})

			It("should publish the VM to an inventory library, deploy from the published image, and reject a duplicate publish", Label("smoke"), func() {
				// Publish the VM to the inventory library.
				vmPubReqBuilder := generateVMPublishRequestBuilder(input.WCPNamespaceName, vmPublishRequestName, input.LinuxVMName, vmPubTargetItemName, inventoryCL.Name)
				createVMPublishRequest(ctx, *config, svClusterClient, *clusterProxy, vmPubReqBuilder)
				vmoperator.VerifyVirtualMachinePublishRequestCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmPublishRequestName, metav1.Condition{
					Type:   vmopv1.VirtualMachinePublishRequestConditionComplete,
					Status: metav1.ConditionTrue,
				})

				// Ensure the published image is available with expected display name under the namespace.
				expectedPublishedImageCRName, err := vmoperator.GetVirtualMachinePublishRequestTargetItemName(ctx, config, svClusterClient, input.WCPNamespaceName, vmPublishRequestName)
				Expect(err).NotTo(HaveOccurred(), "failed to get the published target item name in namespace %q", input.WCPNamespaceName)
				publishedImageCRName := vmoperator.WaitForVirtualMachineImageName(ctx, &config.Config, svClusterClient, input.WCPNamespaceName, expectedPublishedImageCRName)
				Expect(publishedImageCRName).NotTo(BeEmpty(), "published VM Image resource name is empty")
				vmoperator.WaitForVirtualMachineImageStatusDisks(ctx, &config.Config, svClusterClient, input.WCPNamespaceName, publishedImageCRName)

				// Deploy a new VM from the published image and verify it powers on with an IP.
				newVmName := fmt.Sprintf("%s-%s", vmPubSpecName+"-vm", capiutil.RandomString(4))
				newVMBuilder := generateVMBuilder(input.WCPNamespaceName, newVmName, publishedImageCRName, *clusterResources)
				newVmYaml := manifestbuilders.GetVirtualMachineYamlA2(newVMBuilder)
				Expect(clusterProxy.CreateWithArgs(ctx, newVmYaml)).NotTo(HaveOccurred(), "failed to create virtualmachine from the published image", string(newVmYaml))
				vmoperator.WaitForVirtualMachineCreation(ctx, config, svClusterClient, input.WCPNamespaceName, newVmName)
				vmoperator.DeleteVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, newVmName)

				// Clean up the first publish request; the published item remains in the inventory library.
				vmoperator.DeleteVirtualMachinePublishRequest(ctx, svClusterClient, input.WCPNamespaceName, vmPublishRequestName)
				vmoperator.WaitForVirtualMachinePublishRequestToBeDeleted(ctx, config, svClusterClient, input.WCPNamespaceName, vmPublishRequestName)

				// Publishing again to the same target item name must fail with TargetItemAlreadyExists.
				// Store the name in a context-level variable so AfterEach can clean it up.
				dupPublishRequestName = fmt.Sprintf("%s-%s", vmPubSpecName, capiutil.RandomString(4))
				dupPubReqBuilder := generateVMPublishRequestBuilder(input.WCPNamespaceName, dupPublishRequestName, input.LinuxVMName, vmPubTargetItemName, inventoryCL.Name)
				createVMPublishRequest(ctx, *config, svClusterClient, *clusterProxy, dupPubReqBuilder)
				vmoperator.VerifyVirtualMachinePublishRequestCondition(ctx, config, svClusterClient, input.WCPNamespaceName, dupPublishRequestName, metav1.Condition{
					Type:   vmopv1.VirtualMachinePublishRequestConditionTargetValid,
					Status: metav1.ConditionFalse,
					Reason: vmopv1.TargetItemAlreadyExistsReason,
				})
			})
		})

		// This context creates its own Inventory-type ContentLibrary directly with the
		// admin client rather than reusing the "Inventory Content Library" context's
		// non-admin-user setup. The requestedCapacity computation doesn't exercise
		// non-admin RBAC, so there's no need to pay for (or depend on) the SSO
		// kubectl-vsphere login that the RBAC-focused context above requires.
		Context("Requested Capacity Estimation", Ordered, func() {
			var (
				inventoryFolderName string
				inventoryFolder     *object.Folder
				inventoryCL         *imgregv1a2.ContentLibrary
			)

			BeforeAll(func() {
				skipper.SkipUnlessSupervisorCapabilityEnabled(ctx, clusterProxy, consts.InventoryContentLibraryCapabilityName)

				kubeConfig := clusterProxy.GetKubeconfigPath()
				svClusterClient = clusterProxy.GetClient()

				var err error

				vCenterHostname := vcenter.GetVCPNIDFromKubeconfig(ctx, kubeConfig)
				vimClient, err = vcenter.NewVimClient(
					vCenterHostname,
					testbed.AdminUsername,
					testbed.AdminPassword,
				)
				Expect(err).NotTo(HaveOccurred())
			})

			BeforeEach(func() {
				if inventoryFolder == nil {
					inventoryFolderName = fmt.Sprintf("%s-%s-%s", vmPubSpecName, "quota-folder", capiutil.RandomString(4))

					By("Creating library folder")

					finder := find.NewFinder(vimClient, false)
					_, inventoryFolder = createLibraryFolder(ctx, finder, inventoryFolderName)
				}

				if inventoryCL == nil {
					By("Creating an inventory content library", func() {
						inventoryCL = createInventoryContentLibraryCR(ctx, svClusterClient, imgregv1a2.ResourceNamingStrategyPreferItemSourceID, input.WCPNamespaceName, targetContentLibraryName, inventoryFolder.Reference().Value, true, true)
						// Validate CL itself exists and reconciled.
						validateContentLibraryV2(ctx, svClusterClient, inventoryCL, inventoryFolder, targetContentLibraryName, input.WCPNamespaceName, "")
					})
				}
			})

			AfterEach(func() {
				if inventoryFolder != nil {
					vcenter.DeleteFolder(ctx, inventoryFolder)

					inventoryFolderName = ""
					targetContentLibraryName = ""

					inventoryFolder = nil
					inventoryCL = nil
				}

				vmoperator.DeleteVirtualMachinePublishRequest(ctx, svClusterClient, input.WCPNamespaceName, vmPublishRequestName)
				vmoperator.WaitForVirtualMachinePublishRequestToBeDeleted(ctx, config, svClusterClient, input.WCPNamespaceName, vmPublishRequestName)
			})

			It("should compute the requestedCapacity annotation from the VM's actual used storage, not its provisioned disk size", Label("extended-functional","experimental"), func() {
				// Labeling the target ContentLibrary opts the publish request into the
				// async storage-quota check, which is normally driven by an external VCFA
				// component. This lets us exercise the controller's capacity estimation
				// logic (see checkContentLibraryQuota) without that external dependency.
				// Because nothing in this testbed ever clears the check, the request will
				// not reach the Complete condition — we only assert on the annotation.
				Eventually(func(g Gomega) {
					var cl imgregv1a2.ContentLibrary
					g.Expect(svClusterClient.Get(ctx, ctrlclient.ObjectKey{Namespace: input.WCPNamespaceName, Name: inventoryCL.Name}, &cl)).To(Succeed())

					if cl.Labels == nil {
						cl.Labels = map[string]string{}
					}
					cl.Labels[pkgconst.AsyncQuotaPerformCheckAnnotationKey] = "true"

					g.Expect(svClusterClient.Update(ctx, &cl)).To(Succeed())
				}).Should(Succeed(), "failed to label content library %q for async quota validation", inventoryCL.Name)

				vmPubReqBuilder := generateVMPublishRequestBuilder(input.WCPNamespaceName, vmPublishRequestName, input.LinuxVMName, vmPubTargetItemName, inventoryCL.Name)
				createVMPublishRequest(ctx, *config, svClusterClient, *clusterProxy, vmPubReqBuilder)

				// Independently compute the VM's actual used storage from vCenter's file
				// layout, mirroring the controller's own calculation (see
				// checkContentLibraryQuota), to build the expected requestedCapacity value
				// the same way the controller does.
				vmmoid := vmoperator.GetVirtualMachineMOID(ctx, svClusterClient, input.WCPNamespaceName, input.LinuxVMName)
				vmMoRef := vimtypes.ManagedObjectReference{
					Type:  string(vimtypes.ManagedObjectTypeVirtualMachine),
					Value: vmmoid,
				}

				var vmMO mo.VirtualMachine
				propCollector := property.DefaultCollector(vimClient)
				Expect(propCollector.RetrieveOne(ctx, vmMoRef, []string{"layoutEx", "config.hardware"}, &vmMO)).To(Succeed(), "failed to retrieve VM layout and hardware from vCenter")
				Expect(vmMO.LayoutEx).ToNot(BeNil(), "VM should have a layoutEx")
				Expect(vmMO.Config).ToNot(BeNil(), "VM should have a config")

				var actualUsedBytes int64
				for _, f := range vmMO.LayoutEx.File {
					switch vimtypes.VirtualMachineFileLayoutExFileType(f.Type) {
					case vimtypes.VirtualMachineFileLayoutExFileTypeConfig,
						vimtypes.VirtualMachineFileLayoutExFileTypeNvram,
						vimtypes.VirtualMachineFileLayoutExFileTypeDiskDescriptor,
						vimtypes.VirtualMachineFileLayoutExFileTypeDiskExtent:
						actualUsedBytes += f.Size
					}
				}
				requestedCapacity := resource.NewQuantity(actualUsedBytes, resource.BinarySI)

				// Provisioned disk capacity is only computed for visibility in the failure
				// message below — it is not itself asserted on.
				var provisionedBytes int64
				for _, device := range vmMO.Config.Hardware.Device {
					if disk, ok := device.(*vimtypes.VirtualDisk); ok {
						provisionedBytes += disk.CapacityInBytes
					}
				}
				Expect(provisionedBytes).To(BeNumerically(">", 0), "expected the source VM to report a non-zero provisioned disk capacity")
				GinkgoWriter.Printf("expected requestedCapacity: %s, provisioned disk capacity: %d bytes\n", requestedCapacity.String(), provisionedBytes)

				Eventually(func(g Gomega) {
					vmPub, err := utils.GetVirtualMachinePublishRequest(ctx, svClusterClient, input.WCPNamespaceName, vmPublishRequestName)
					g.Expect(err).NotTo(HaveOccurred())

					g.Expect(vmPub.Annotations).To(HaveKeyWithValue(
						pkgconst.AsyncQuotaCheckRequestedCapacityAnnotationKey, requestedCapacity.String()),
						"requestedCapacity annotation should equal the VM's actual used storage, not its provisioned disk capacity (%d bytes)", provisionedBytes)
				}, config.GetIntervals("default", "wait-virtual-machine-publish-request-condition")...).Should(Succeed(),
					"Timed out waiting for the requestedCapacity annotation to be set on VirtualMachinePublishRequest %s", vmPublishRequestName)
			})
		})
	})
}

func generateVMBuilder(
	namespace, name, imageName string,
	clusterResources e2eConfig.Resources) manifestbuilders.VirtualMachineYaml {
	return manifestbuilders.VirtualMachineYaml{
		Namespace:        namespace,
		Name:             name,
		ImageName:        imageName,
		VMClassName:      clusterResources.VMClassName,
		StorageClassName: clusterResources.StorageClassName,
		ResourcePolicy:   clusterResources.VMResourcePolicyName,
		PowerState:       "PoweredOn",
	}
}

func generateVMPublishRequestBuilder(
	namespace,
	vmpubName,
	sourceName,
	targetItemName,
	targetLocationName string) manifestbuilders.VirtualMachinePublishRequestYaml {
	return manifestbuilders.VirtualMachinePublishRequestYaml{
		Namespace: namespace,
		Name:      vmpubName,
		Source: manifestbuilders.VirtualMachinePublishRequestSource{
			Name: sourceName,
		},
		Target: manifestbuilders.VirtualMachinePublishRequestTarget{
			Item: manifestbuilders.VirtualMachinePublishRequestTargetItem{
				Name:        targetItemName, // if empty, will be set to default vmPubReqName + "-image"
				Description: "Test VM publish request target item description",
			},
			Location: manifestbuilders.VirtualMachinePublishRequestTargetLocation{
				Name: targetLocationName,
			},
		},
	}
}

func createVMPublishRequest(
	ctx context.Context,
	config e2eConfig.E2EConfig,
	client ctrlclient.Client,
	clusterProxy common.VMServiceClusterProxy,
	vmPubReqBuilder manifestbuilders.VirtualMachinePublishRequestYaml) {
	vmPubReqYaml := manifestbuilders.GetVirtualMachinePublishRequestYaml(vmPubReqBuilder)
	e2eframework.Logf("%v", string(vmPubReqYaml))
	Expect(clusterProxy.CreateWithArgs(ctx, vmPubReqYaml)).NotTo(HaveOccurred(), "failed to create VirtualMachinePublishRequest")

	namespace, name := vmPubReqBuilder.Namespace, vmPubReqBuilder.Name
	Eventually(func() bool {
		vmPub, err := utils.GetVirtualMachinePublishRequest(ctx, client, namespace, name)
		if err != nil {
			e2eframework.Logf("retry due to: %v", err)
			return false
		}

		return vmPub != nil
	}, config.GetIntervals("default", "wait-virtual-machine-publish-request-creation")...).Should(Equal(true), "Timed out waiting for VirtualMachinePublishRequest %s to be created", name)
}

func createLibraryFolder(ctx context.Context, finder *find.Finder, libFolder string) (*object.DatacenterFolders, *object.Folder) {
	dc, err := finder.DatacenterList(ctx, "*")
	Expect(err).NotTo(HaveOccurred())
	finder.SetDatacenter(dc[0])
	rootFolder, err := dc[0].Folders(ctx)
	Expect(err).NotTo(HaveOccurred())
	folderObj, err := rootFolder.VmFolder.CreateFolder(ctx, libFolder)
	Expect(err).NotTo(HaveOccurred())

	return rootFolder, folderObj
}

func createInventoryContentLibraryCR(ctx context.Context, c ctrlclient.Client, resourceNamingStrategy imgregv1a2.ResourceNamingStrategy, namespace, clName, clID string, allowPublish, allowDelete bool) *imgregv1a2.ContentLibrary {
	cl := &imgregv1a2.ContentLibrary{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clName,
			Namespace: namespace,
		},
		Spec: imgregv1a2.ContentLibrarySpec{
			BaseContentLibrarySpec: imgregv1a2.BaseContentLibrarySpec{
				ID:                     clID,
				Type:                   imgregv1a2.LibraryTypeInventory,
				ResourceNamingStrategy: resourceNamingStrategy,
			},
			AllowPublish: allowPublish,
			AllowDelete:  allowDelete,
		},
	}

	err := c.Create(ctx, cl)
	Expect(err).ToNot(HaveOccurred())

	return cl
}

func validateContentLibraryV2(ctx context.Context, svClusterClient ctrlclient.Client, cl *imgregv1a2.ContentLibrary, folder *object.Folder, clName, clNs, clDescription string) {
	var foundCL imgregv1a2.ContentLibrary

	Eventually(func(g Gomega) {
		g.Expect(svClusterClient.Get(ctx, ctrlclient.ObjectKey{
			Namespace: clNs,
			Name:      cl.Name,
		}, &foundCL)).To(Succeed())

		g.Expect(foundCL.Spec.ID).To(Equal(folder.Reference().Value))
		g.Expect(foundCL.Spec.AllowImport).To(BeFalse())

		folderName, err := folder.ObjectName(ctx)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(foundCL.Status.Name).To(Equal(folderName))
		g.Expect(foundCL.ObjectMeta.Name).To(Equal(clName))

		if clDescription != "" {
			g.Expect(foundCL.Status.Description).To(Equal(clDescription))
		}
	}).WithTimeout(5 * time.Minute).Should(Succeed())
}

func setupNonAdminUserForTests(ctx context.Context, vimClient *vim25.Client, sshCommandRunner libssh.SSHCommandRunner, _ ctrlclient.Client, svClusterProxy *common.VMServiceClusterProxy) (*vcenter.User, ctrlclient.Client) {
	By("Creating non-admin user and assign it to SupervisorProviderAdministrators group")

	user, err := vcenter.CreateUserAndAssignToGrp(ctx, vimClient, sshCommandRunner, "gce2e-test-user", "Admin!23Admin", "SupervisorProviderAdministrators")
	Expect(err).ToNot(HaveOccurred())

	svClusterKubeConfig := svClusterProxy.GetKubeconfigPath()
	_, _, supervisorClusterIP := testutils.GetHelpersFromKubeconfig(ctx, svClusterKubeConfig)

	By("Logging in as non-admin user in supervisor")

	kubectlPlugin := testutils.LoginWithUserWithRetry(user, supervisorClusterIP, "", "")

	By("Creating a k8s client from the non-admin user kubeconfig")

	restCfg, err := clientcmd.BuildConfigFromFlags("", kubectlPlugin.KubeconfigPath())
	Expect(err).NotTo(HaveOccurred())

	nonAdminClient, err := ctrlclient.New(restCfg, ctrlclient.Options{Scheme: svClusterProxy.GetScheme()})
	Expect(err).NotTo(HaveOccurred())

	By("Checking if non-admin user kubeconfig is able to do basic operations")

	pods := &corev1.PodList{}
	err = nonAdminClient.List(ctx, pods)
	Expect(err).NotTo(HaveOccurred())

	return user, nonAdminClient
}
