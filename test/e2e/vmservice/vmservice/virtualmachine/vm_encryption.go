// Copyright (c) 2020-2025 Broadcom. All Rights Reserved.
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
	crypto "github.com/vmware/govmomi/crypto"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	capiutil "sigs.k8s.io/cluster-api/util"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1a3 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"

	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
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

type VMEncryptionInput struct {
	ClusterProxy   wcpframework.WCPClusterProxyInterface
	Config         *e2eConfig.E2EConfig
	WCPClient      wcp.WorkloadManagementAPI
	ArtifactFolder string
}

func VMEncryptionSpec(ctx context.Context, inputGetter func() VMEncryptionInput) {
	const (
		specName = "vm-encryption"

		// Key Providers setup by hack/kms.sh
		standardKeyProviderID = "gce2e-standard"
		nativeKeyProviderID   = "gce2e-native"
	)

	var (
		input                                VMEncryptionInput
		wcpClient                            wcp.WorkloadManagementAPI
		config                               *e2eConfig.E2EConfig
		clusterProxy                         *common.VMServiceClusterProxy
		svClusterConfig                      *e2eConfig.ManagementClusterConfig
		svClusterClient                      ctrlclient.Client
		vCenterClient                        *vim25.Client
		cryptoManager                        *crypto.ManagerKmip
		clusterResources                     *e2eConfig.Resources
		tmpNamespaceCtx                      wcpframework.NamespaceContext
		tmpNamespaceName                     string
		vmYaml                               []byte
		vmName                               string
		vmiName                              string
		createSpecE2eTestBestEffortSmallVTPM wcp.VMClassSpec
		byokFSSEnabled                       bool
		defaultKeyProviderID                 string
		linuxImageDisplayName                string
	)

	BeforeEach(func() {
		var err error

		input = inputGetter()
		Expect(input.Config).ToNot(BeNil(), "Invalid argument. input.E2EConfig can't be nil when calling %s spec", specName)
		Expect(input.Config.InfraConfig).ToNot(BeNil(), "Invalid argument. input.E2EConfig.InfraConfig can't be nil when calling %s spec", specName)
		skipper.SkipUnlessInfraIs(input.Config.InfraConfig.InfraName, consts.WCP)

		Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.SVClusterProxy can't be nil when calling %s spec", specName)
		Expect(os.MkdirAll(input.ArtifactFolder, 0755)).To(Succeed(), "Invalid argument. input.ArtifactFolder can't be created for %s spec", specName)

		wcpClient = input.WCPClient
		config = input.Config
		clusterProxy = input.ClusterProxy.(*common.VMServiceClusterProxy)
		svClusterConfig = config.InfraConfig.ManagementClusterConfig
		clusterResources = svClusterConfig.Resources
		svClusterClient = clusterProxy.GetClient()
		vCenterClient = vcenter.NewVimClientFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath())
		cryptoManager = crypto.NewManagerKmip(vCenterClient)
		cancelPodWatches := framework.WatchPodLogsAndEventsInNamespaces(ctx, []string{config.GetVariable("VMOPNamespace")}, input.ClusterProxy.GetClientSet(), filepath.Join(input.ArtifactFolder, specName))
		DeferCleanup(cancelPodWatches)

		byokFSSEnabled = utils.IsFssEnabled(ctx, svClusterClient, config.GetVariable("VMOPNamespace"), config.GetVariable("VMOPDeploymentName"), config.GetVariable("VMOPManagerCommand"), config.GetVariable("EnvFSSBYOK"))

		linuxImageDisplayName = vmservice.GetDefaultImageDisplayName(clusterResources)

		vmYaml = nil
		vmName = fmt.Sprintf("%s-vm-%s", specName, capiutil.RandomString(4))

		By("Create a new namespace")

		vmserviceCLID := vmservice.GetContentLibraryUUIDByName(consts.VMServiceCLName, wcpClient)
		clIDs := []string{vmserviceCLID}
		vmClassNames := []string{clusterResources.VMClassName}
		vmsvcSpecs := wcp.NewVMServiceSpecDetails(vmClassNames, clIDs)

		tmpNamespaceName = fmt.Sprintf("%s-ns-%s", specName, capiutil.RandomString(4))
		tmpNamespaceCtx, err = clusterProxy.CreateWCPNamespace(ctx, config, vmsvcSpecs, clusterResources.StorageClassName, clusterResources.WorkerStorageClassName, tmpNamespaceName, input.ArtifactFolder)
		Expect(err).NotTo(HaveOccurred(), "failed to create wcp namespace %s", tmpNamespaceName)

		wcp.WaitForNamespaceReady(wcpClient, tmpNamespaceName)

		vmiName, err = vmoperator.WaitForVirtualMachineImageName(ctx, &config.Config, svClusterClient, tmpNamespaceName, linuxImageDisplayName)
		Expect(err).NotTo(HaveOccurred(), "failed to get the VM Image name in namespace %q", tmpNamespaceName)

		By(utils.E2EEncryptionStorageProfileName + " should exist")
		Expect(utils.EnsureE2EEncryptionStorageInNamespace(ctx, vCenterClient, 
			wcpClient, clusterProxy.GetClientSet(), svClusterClient, *config, 
			tmpNamespaceName, clusterResources.StorageClassName)).
			To(Succeed(), "failed to ensure encryption storage in namespace %s", 
				tmpNamespaceName)

		defaultKeyProviderID, err = cryptoManager.GetDefaultKmsClusterID(ctx, nil, true)
		Expect(err).NotTo(HaveOccurred(), "failed to get default Key Provider ID")
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			vmoperator.DescribeResourceIfExists(ctx, svClusterClient, clusterProxy.GetKubeconfigPath(), tmpNamespaceName, vmName, "vm")
		}
		// Delete the virtual machine if it was created.
		if len(vmYaml) > 0 {
			Expect(clusterProxy.DeleteWithArgs(ctx, vmYaml)).To(Succeed(), "failed to delete virtualmachine")
			// Verify that virtual machine does not exist.
			vmoperator.WaitForVirtualMachineToBeDeleted(ctx, config, svClusterClient, tmpNamespaceName, vmName)
		}

		// Delete vTPM class if it was created
		if createSpecE2eTestBestEffortSmallVTPM.ID != "" {
			vmservice.VerifyVMClassDeletion(wcpClient, createSpecE2eTestBestEffortSmallVTPM.ID)
			createSpecE2eTestBestEffortSmallVTPM.ID = ""
		}
		// Delete the temporary namespace if it was created.
		if tmpNamespaceCtx.GetNamespace() != nil {
			clusterProxy.DeleteWCPNamespace(tmpNamespaceCtx)
			wcp.WaitForNamespaceDeleted(wcpClient, tmpNamespaceCtx.GetNamespace().Name)
		}

		_ = cryptoManager.SetDefaultKmsClusterId(ctx, defaultKeyProviderID, nil)

		vcenter.LogoutVimClient(vCenterClient)
	})

	It("Create an Encrypted VirtualMachine using encryption storage policy", Label("smoke"), func() {
		useKeyProvider(ctx, cryptoManager, nativeKeyProviderID)

		By("Create VM using encryption storage policy")

		vmParameters := manifestbuilders.VirtualMachineYaml{
			Namespace:        tmpNamespaceName,
			Name:             vmName,
			ImageName:        vmiName,
			VMClassName:      "best-effort-small",
			StorageClassName: utils.E2EEncryptionStorageClassName,
			ResourcePolicy:   clusterResources.VMResourcePolicyName,
			PowerState:       "PoweredOn",
		}
		vmYaml = manifestbuilders.GetVirtualMachineYamlA2(vmParameters)
		Expect(clusterProxy.CreateWithArgs(ctx, vmYaml)).Should(Succeed(), "failed to create virtualmachine:\n %s", string(vmYaml))

		vmoperator.WaitForVirtualMachineCreation(ctx, config, svClusterClient, tmpNamespaceName, vmName)

		if byokFSSEnabled {
			waitForCryptoCondition(ctx, config, svClusterClient, tmpNamespaceName, vmName, "")
		}
	})

	It("Create an Encrypted VirtualMachine using VM Class configured with vTPM", func() {
		useKeyProvider(ctx, cryptoManager, nativeKeyProviderID)

		By("Create VM Class with vTPM and ensure namespace has access")

		createSpecE2eTestBestEffortSmallVTPM = vmservice.CreateSpecE2eVMClassVTPMConfigSpec()
		vmservice.VerifyVMClassCreate(wcpClient, createSpecE2eTestBestEffortSmallVTPM, createSpecE2eTestBestEffortSmallVTPM)
		vmservice.EnsureVMClassAccess(wcpClient, createSpecE2eTestBestEffortSmallVTPM.ID, tmpNamespaceName)

		By("Create VM using vTPM class")

		vmParameters := manifestbuilders.VirtualMachineYaml{
			Namespace:        tmpNamespaceName,
			Name:             vmName,
			ImageName:        vmiName,
			VMClassName:      createSpecE2eTestBestEffortSmallVTPM.ID,
			StorageClassName: clusterResources.StorageClassName,
			ResourcePolicy:   clusterResources.VMResourcePolicyName,
			PowerState:       "PoweredOn",
			Annotations:      map[string]string{"vmoperator.vmware.com/firmware": "efi"},
		}
		vmYaml = manifestbuilders.GetVirtualMachineYamlA2(vmParameters)
		Expect(clusterProxy.CreateWithArgs(ctx, vmYaml)).Should(Succeed(), "failed to create virtualmachine:\n %s", string(vmYaml))

		vmoperator.WaitForVirtualMachineCreation(ctx, config, svClusterClient, tmpNamespaceName, vmName)

		if byokFSSEnabled {
			waitForCryptoCondition(ctx, config, svClusterClient, tmpNamespaceName, vmName, "")
		}
	})

	It("Create an Encrypted VirtualMachine using vTPM and encryption class", func() {
		if !byokFSSEnabled {
			Skip("BYOK FSS is not enabled")
		}

		// See VCFCON-2837
		adminClusterProxy, err := clusterProxy.NewAdminClusterProxy(ctx)
		Expect(err).ToNot(HaveOccurred())

		defer adminClusterProxy.Dispose(ctx)

		By("Create Encryption Class")

		class := manifestbuilders.EncryptionClass{
			Namespace:   tmpNamespaceName,
			Name:        nativeKeyProviderID,
			KeyProvider: nativeKeyProviderID,
		}
		ecYaml := manifestbuilders.GetEncryptionClassYaml(class)
		Expect(adminClusterProxy.CreateWithArgs(ctx, ecYaml)).Should(Succeed(), "failed to create EncryptionClass:\n %s", string(ecYaml))
		useKeyProvider(ctx, cryptoManager, class.KeyProvider) // TODO: should not need to have a default provider set

		By("Create VM Class with vTPM and ensure namespace has access")

		createSpecE2eTestBestEffortSmallVTPM = vmservice.CreateSpecE2eVMClassVTPMConfigSpec()
		vmservice.VerifyVMClassCreate(wcpClient, createSpecE2eTestBestEffortSmallVTPM, createSpecE2eTestBestEffortSmallVTPM)
		vmservice.EnsureVMClassAccess(wcpClient, createSpecE2eTestBestEffortSmallVTPM.ID, tmpNamespaceName)

		By("Create VM using vTPM class")

		vmParameters := manifestbuilders.VirtualMachineYaml{
			Namespace:        tmpNamespaceName,
			Name:             vmName,
			ImageName:        vmiName,
			VMClassName:      createSpecE2eTestBestEffortSmallVTPM.ID,
			StorageClassName: clusterResources.StorageClassName,
			ResourcePolicy:   clusterResources.VMResourcePolicyName,
			PowerState:       "PoweredOn",
			Annotations:      map[string]string{"vmoperator.vmware.com/firmware": "efi"},
			Crypto: &manifestbuilders.Crypto{
				EncryptionClassName: class.Name,
			},
		}
		vmYaml = manifestbuilders.GetVirtualMachineYamlA2(vmParameters)
		Expect(clusterProxy.CreateWithArgs(ctx, vmYaml)).Should(Succeed(), "failed to create virtualmachine:\n %s", string(vmYaml))

		vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient, tmpNamespaceName, vmName)
		vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, tmpNamespaceName, vmName, "PoweredOn")
		waitForCryptoCondition(ctx, config, svClusterClient, tmpNamespaceName, vmName, "")
	})

	It("Create an Encrypted VirtualMachine using encryption class", func() {
		if !byokFSSEnabled {
			Skip("BYOK FSS is not enabled")
		}

		// See VCFCON-2837
		adminClusterProxy, err := clusterProxy.NewAdminClusterProxy(ctx)
		Expect(err).ToNot(HaveOccurred())

		defer adminClusterProxy.Dispose(ctx)

		By("Create Encryption Class")

		class := manifestbuilders.EncryptionClass{
			Namespace:   tmpNamespaceName,
			Name:        standardKeyProviderID,
			KeyProvider: standardKeyProviderID,
		}
		ecYaml := manifestbuilders.GetEncryptionClassYaml(class)
		Expect(adminClusterProxy.CreateWithArgs(ctx, ecYaml)).Should(Succeed(), "failed to create EncryptionClass:\n %s", string(ecYaml))
		useKeyProvider(ctx, cryptoManager, class.KeyProvider) // Required when using encryption storage policy

		By("Create VM using invalid encryption class name")

		vmParameters := manifestbuilders.VirtualMachineYaml{
			Namespace:        tmpNamespaceName,
			Name:             vmName,
			ImageName:        vmiName,
			VMClassName:      "best-effort-small",
			StorageClassName: utils.E2EEncryptionStorageClassName,
			ResourcePolicy:   clusterResources.VMResourcePolicyName,
			PowerState:       string(vmopv1a3.VirtualMachinePowerStateOn),
			Crypto: &manifestbuilders.Crypto{
				EncryptionClassName: "invalid",
			},
		}
		vmYaml = manifestbuilders.GetVirtualMachineYamlA3(vmParameters)
		Expect(clusterProxy.CreateWithArgs(ctx, vmYaml)).Should(Succeed(), "failed to create virtualmachine:\n %s", string(vmYaml))

		vmoperator.WaitForVirtualMachineToExist(ctx, config, svClusterClient, tmpNamespaceName, vmName)
		waitForCryptoCondition(ctx, config, svClusterClient, tmpNamespaceName, vmName, "EncryptionClassNotFound")

		By("Update VM using valid encryption class name")

		vmParameters.Crypto.EncryptionClassName = class.Name
		vmYaml = manifestbuilders.GetVirtualMachineYamlA3(vmParameters)
		Expect(clusterProxy.ApplyWithArgs(ctx, vmYaml)).Should(Succeed(), "failed to update virtualmachine:\n %s", string(vmYaml))

		vmoperator.WaitForVirtualMachineCreation(ctx, config, svClusterClient, tmpNamespaceName, vmName)

		cryptoStatus := waitForCryptoCondition(ctx, config, svClusterClient, tmpNamespaceName, vmName, "")
		Expect(cryptoStatus.ProviderID).To(Equal(class.KeyProvider))
	})

	It("Create an Encrypted VirtualMachine using an encryption key id", FlakeAttempts(3), func() {
		if !byokFSSEnabled {
			Skip("BYOK FSS is not enabled")
		}

		// See VCFCON-2837
		adminClusterProxy, err := clusterProxy.NewAdminClusterProxy(ctx)
		Expect(err).ToNot(HaveOccurred())

		defer adminClusterProxy.Dispose(ctx)

		By("Create Encryption Class with invalid key")

		keyID, err := cryptoManager.GenerateKey(ctx, standardKeyProviderID)
		Expect(err).To(BeNil())

		class := manifestbuilders.EncryptionClass{
			Namespace:   tmpNamespaceName,
			Name:        standardKeyProviderID,
			KeyProvider: standardKeyProviderID,
			KeyID:       keyID + "-invalid",
		}
		ecYaml := manifestbuilders.GetEncryptionClassYaml(class)
		Expect(adminClusterProxy.CreateWithArgs(ctx, ecYaml)).Should(Succeed(), "failed to create EncryptionClass:\n %s", string(ecYaml))
		useKeyProvider(ctx, cryptoManager, class.KeyProvider)

		By("Create PVC using encryption class")

		clientSet := clusterProxy.GetClientSet()
		pvcName := vmName + "-pvc"
		testutils.AssertCreatePVC(clientSet, pvcName, tmpNamespaceName, utils.E2EEncryptionStorageClassName)
		volumeClaims := clientSet.CoreV1().PersistentVolumeClaims(tmpNamespaceName)
		pvc, err := volumeClaims.Get(ctx, pvcName, metav1.GetOptions{})
		Expect(err).ToNot(HaveOccurred())

		pvc.Annotations["csi.vsphere.encryption-class"] = class.Name
		_, err = volumeClaims.Update(ctx, pvc, metav1.UpdateOptions{})
		Expect(err).ToNot(HaveOccurred())

		By("Create VM using encryption class with invalid key")

		vmParameters := manifestbuilders.VirtualMachineYaml{
			Namespace:        tmpNamespaceName,
			Name:             vmName,
			ImageName:        vmiName,
			VMClassName:      "best-effort-small",
			StorageClassName: utils.E2EEncryptionStorageClassName,
			ResourcePolicy:   clusterResources.VMResourcePolicyName,
			PowerState:       string(vmopv1a3.VirtualMachinePowerStateOn),
			PVCNames:         []string{pvcName},
			Crypto: &manifestbuilders.Crypto{
				EncryptionClassName: class.Name,
			},
		}
		vmYaml = manifestbuilders.GetVirtualMachineYamlA3(vmParameters)
		Expect(clusterProxy.CreateWithArgs(ctx, vmYaml)).Should(Succeed(), "failed to create virtualmachine:\n %s", string(vmYaml))
		waitForCryptoCondition(ctx, config, svClusterClient, tmpNamespaceName, vmName, "EncryptionClassInvalid")

		By("Update encryption class using valid key")

		class.KeyID = keyID
		ecYaml = manifestbuilders.GetEncryptionClassYaml(class)
		Expect(adminClusterProxy.ApplyWithArgs(ctx, ecYaml)).Should(Succeed(), "failed to update EncryptionClass:\n %s", string(ecYaml))

		By("Expect VM to be created using updated encryption class key")
		vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, tmpNamespaceName, vmName, "PoweredOn")
		cryptoStatus := waitForCryptoCondition(ctx, config, svClusterClient, tmpNamespaceName, vmName, "")
		Expect(cryptoStatus.ProviderID).To(Equal(class.KeyProvider))
		Expect(cryptoStatus.KeyID).To(Equal(class.KeyID))
	})

	It("Encrypt VM config and classic disks using encryption storage policy", func() {
		if !byokFSSEnabled {
			Skip("BYOK FSS is not enabled")
		}

		useKeyProvider(ctx, cryptoManager, nativeKeyProviderID)

		adminClusterProxy, err := clusterProxy.NewAdminClusterProxy(ctx)
		Expect(err).ToNot(HaveOccurred())

		defer adminClusterProxy.Dispose(ctx)

		By("Create Encryption Class for the VM")

		class := manifestbuilders.EncryptionClass{
			Namespace:   tmpNamespaceName,
			Name:        nativeKeyProviderID,
			KeyProvider: nativeKeyProviderID,
		}
		ecYaml := manifestbuilders.GetEncryptionClassYaml(class)
		Expect(adminClusterProxy.CreateWithArgs(ctx, ecYaml)).Should(Succeed(), "failed to create EncryptionClass:\n %s", string(ecYaml))

		By("Create VM using encryption storage policy and encryption class")

		vmParameters := manifestbuilders.VirtualMachineYaml{
			Namespace:        tmpNamespaceName,
			Name:             vmName,
			ImageName:        vmiName,
			VMClassName:      "best-effort-small",
			StorageClassName: utils.E2EEncryptionStorageClassName,
			ResourcePolicy:   clusterResources.VMResourcePolicyName,
			PowerState:       string(vmopv1a3.VirtualMachinePowerStateOn),
			Crypto: &manifestbuilders.Crypto{
				EncryptionClassName: class.Name,
			},
		}
		vmYaml = manifestbuilders.GetVirtualMachineYamlA3(vmParameters)
		Expect(clusterProxy.CreateWithArgs(ctx, vmYaml)).Should(Succeed(), "failed to create virtualmachine:\n %s", string(vmYaml))

		vmoperator.WaitForVirtualMachineCreation(ctx, config, svClusterClient, tmpNamespaceName, vmName)
		cryptoStatus := waitForCryptoCondition(ctx, config, svClusterClient, tmpNamespaceName, vmName, "")

		By("Verify VM config and disks are encrypted")
		Expect(cryptoStatus.ProviderID).To(Equal(class.KeyProvider))
		Expect(cryptoStatus.Encrypted).To(ContainElement(vmopv1a3.VirtualMachineEncryptionTypeConfig))
		Expect(cryptoStatus.Encrypted).To(ContainElement(vmopv1a3.VirtualMachineEncryptionTypeDisks))
	})

	It("Encrypt PVC using encryption class annotation on the PVC", func() {
		if !byokFSSEnabled {
			Skip("BYOK FSS is not enabled")
		}

		adminClusterProxy, err := clusterProxy.NewAdminClusterProxy(ctx)
		Expect(err).ToNot(HaveOccurred())

		defer adminClusterProxy.Dispose(ctx)

		By("Create Encryption Class for the PVC")

		class := manifestbuilders.EncryptionClass{
			Namespace:   tmpNamespaceName,
			Name:        standardKeyProviderID,
			KeyProvider: standardKeyProviderID,
		}
		ecYaml := manifestbuilders.GetEncryptionClassYaml(class)
		Expect(adminClusterProxy.CreateWithArgs(ctx, ecYaml)).Should(Succeed(), "failed to create EncryptionClass:\n %s", string(ecYaml))
		useKeyProvider(ctx, cryptoManager, class.KeyProvider)

		By("Create PVC with encryption class annotation")

		clientSet := clusterProxy.GetClientSet()
		pvcName := vmName + "-pvc"
		testutils.AssertCreatePVC(clientSet, pvcName, tmpNamespaceName, utils.E2EEncryptionStorageClassName)
		volumeClaims := clientSet.CoreV1().PersistentVolumeClaims(tmpNamespaceName)
		pvc, err := volumeClaims.Get(ctx, pvcName, metav1.GetOptions{})
		Expect(err).ToNot(HaveOccurred())

		pvc.Annotations["csi.vsphere.encryption-class"] = class.Name
		_, err = volumeClaims.Update(ctx, pvc, metav1.UpdateOptions{})
		Expect(err).ToNot(HaveOccurred())

		By("Create VM with attached PVC")

		vmParameters := manifestbuilders.VirtualMachineYaml{
			Namespace:        tmpNamespaceName,
			Name:             vmName,
			ImageName:        vmiName,
			VMClassName:      "best-effort-small",
			StorageClassName: utils.E2EEncryptionStorageClassName,
			ResourcePolicy:   clusterResources.VMResourcePolicyName,
			PowerState:       string(vmopv1a3.VirtualMachinePowerStateOn),
			PVCNames:         []string{pvcName},
			Crypto: &manifestbuilders.Crypto{
				EncryptionClassName: class.Name,
			},
		}
		vmYaml = manifestbuilders.GetVirtualMachineYamlA3(vmParameters)
		Expect(clusterProxy.CreateWithArgs(ctx, vmYaml)).Should(Succeed(), "failed to create virtualmachine:\n %s", string(vmYaml))

		vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, tmpNamespaceName, vmName, "PoweredOn")
		cryptoStatus := waitForCryptoCondition(ctx, config, svClusterClient, tmpNamespaceName, vmName, "")

		By("Verify VM is encrypted with the encryption class provider")
		Expect(cryptoStatus.ProviderID).To(Equal(class.KeyProvider))
		Expect(cryptoStatus.KeyID).NotTo(BeEmpty())

		By("Verify crypto status of volumes reflects volume is encrypted using encryption class from annotation")
		Eventually(func(g Gomega) {
			vm, err := utils.GetVirtualMachineA3(ctx, svClusterClient, tmpNamespaceName, vmName)
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(vm.Status.Volumes).To(HaveLen(2))

			for _, volume := range vm.Status.Volumes {
				volumeStatusCrypto := volume.Crypto
				g.Expect(volumeStatusCrypto).NotTo(BeNil())
				g.Expect(volumeStatusCrypto.ProviderID).To(Equal(class.KeyProvider))
				g.Expect(volumeStatusCrypto.KeyID).NotTo(BeEmpty())
			}
		}, config.GetIntervals("default", "wait-virtual-machine-creation")...).Should(Succeed(), "Timed out waiting for PVC volume encryption: %s", vmName)
	})

	It("Encrypt PVC using default key provider when no encryption class annotation", func() {
		if !byokFSSEnabled {
			Skip("BYOK FSS is not enabled")
		}

		adminClusterProxy, err := clusterProxy.NewAdminClusterProxy(ctx)
		Expect(err).ToNot(HaveOccurred())

		defer adminClusterProxy.Dispose(ctx)

		useKeyProvider(ctx, cryptoManager, standardKeyProviderID)

		By("Create Encryption Class for the VM")

		class := manifestbuilders.EncryptionClass{
			Namespace:   tmpNamespaceName,
			Name:        standardKeyProviderID,
			KeyProvider: standardKeyProviderID,
		}
		ecYaml := manifestbuilders.GetEncryptionClassYaml(class)
		Expect(adminClusterProxy.CreateWithArgs(ctx, ecYaml)).Should(Succeed(), "failed to create EncryptionClass:\n %s", string(ecYaml))

		By("Create PVC without encryption class annotation")

		clientSet := clusterProxy.GetClientSet()
		pvcName := vmName + "-pvc"
		testutils.AssertCreatePVC(clientSet, pvcName, tmpNamespaceName, utils.E2EEncryptionStorageClassName)

		By("Create VM with attached PVC and encryption class")

		vmParameters := manifestbuilders.VirtualMachineYaml{
			Namespace:        tmpNamespaceName,
			Name:             vmName,
			ImageName:        vmiName,
			VMClassName:      "best-effort-small",
			StorageClassName: utils.E2EEncryptionStorageClassName,
			ResourcePolicy:   clusterResources.VMResourcePolicyName,
			PowerState:       string(vmopv1a3.VirtualMachinePowerStateOn),
			PVCNames:         []string{pvcName},
			Crypto: &manifestbuilders.Crypto{
				EncryptionClassName: class.Name,
			},
		}
		vmYaml = manifestbuilders.GetVirtualMachineYamlA3(vmParameters)
		Expect(clusterProxy.CreateWithArgs(ctx, vmYaml)).Should(Succeed(), "failed to create virtualmachine:\n %s", string(vmYaml))

		vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, tmpNamespaceName, vmName, "PoweredOn")
		cryptoStatus := waitForCryptoCondition(ctx, config, svClusterClient, tmpNamespaceName, vmName, "")

		By("Verify VM is encrypted")
		Expect(cryptoStatus.ProviderID).To(Equal(class.KeyProvider))
		Expect(cryptoStatus.KeyID).NotTo(BeEmpty())

		By("Verify crypto status of volumes reflects volume is encrypted using the default key provider")
		Eventually(func(g Gomega) {
			vm, err := utils.GetVirtualMachineA3(ctx, svClusterClient, tmpNamespaceName, vmName)
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(vm.Status.Volumes).To(HaveLen(2))

			for _, volume := range vm.Status.Volumes {
				volumeStatusCrypto := volume.Crypto
				g.Expect(volumeStatusCrypto).NotTo(BeNil())
				g.Expect(volumeStatusCrypto.ProviderID).NotTo(BeEmpty())
			}
		}, config.GetIntervals("default", "wait-virtual-machine-creation")...).Should(Succeed(), "Timed out waiting for PVC volume encryption with default provider: %s", vmName)
	})

	It("Re-encrypt VM and PVC when encryption class is updated on both", func() {
		if !byokFSSEnabled {
			Skip("BYOK FSS is not enabled")
		}

		adminClusterProxy, err := clusterProxy.NewAdminClusterProxy(ctx)
		Expect(err).ToNot(HaveOccurred())

		defer adminClusterProxy.Dispose(ctx)

		By("Create initial Encryption Class")

		class := manifestbuilders.EncryptionClass{
			Namespace:   tmpNamespaceName,
			Name:        standardKeyProviderID,
			KeyProvider: standardKeyProviderID,
		}
		ecYaml := manifestbuilders.GetEncryptionClassYaml(class)
		Expect(adminClusterProxy.CreateWithArgs(ctx, ecYaml)).Should(Succeed(), "failed to create EncryptionClass:\n %s", string(ecYaml))
		useKeyProvider(ctx, cryptoManager, class.KeyProvider)

		By("Create PVC with encryption class annotation")

		clientSet := clusterProxy.GetClientSet()
		pvcName := vmName + "-pvc"
		testutils.AssertCreatePVC(clientSet, pvcName, tmpNamespaceName, utils.E2EEncryptionStorageClassName)
		volumeClaims := clientSet.CoreV1().PersistentVolumeClaims(tmpNamespaceName)
		pvc, err := volumeClaims.Get(ctx, pvcName, metav1.GetOptions{})
		Expect(err).ToNot(HaveOccurred())

		pvc.Annotations["csi.vsphere.encryption-class"] = class.Name
		_, err = volumeClaims.Update(ctx, pvc, metav1.UpdateOptions{})
		Expect(err).ToNot(HaveOccurred())

		By("Create VM with attached PVC")

		vmParameters := manifestbuilders.VirtualMachineYaml{
			Namespace:        tmpNamespaceName,
			Name:             vmName,
			ImageName:        vmiName,
			VMClassName:      "best-effort-small",
			StorageClassName: utils.E2EEncryptionStorageClassName,
			ResourcePolicy:   clusterResources.VMResourcePolicyName,
			PowerState:       string(vmopv1a3.VirtualMachinePowerStateOn),
			PVCNames:         []string{pvcName},
			Crypto: &manifestbuilders.Crypto{
				EncryptionClassName: class.Name,
			},
		}
		vmYaml = manifestbuilders.GetVirtualMachineYamlA3(vmParameters)
		Expect(clusterProxy.CreateWithArgs(ctx, vmYaml)).Should(Succeed(), "failed to create virtualmachine:\n %s", string(vmYaml))

		vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, tmpNamespaceName, vmName, "PoweredOn")
		cryptoStatus := waitForCryptoCondition(ctx, config, svClusterClient, tmpNamespaceName, vmName, "")
		Expect(cryptoStatus.ProviderID).To(Equal(class.KeyProvider))
		Expect(cryptoStatus.KeyID).NotTo(BeEmpty())
		initialKeyID := cryptoStatus.KeyID

		By("Update encryption class with a new key")

		newKeyID, err := cryptoManager.GenerateKey(ctx, standardKeyProviderID)
		Expect(err).ToNot(HaveOccurred())

		class.KeyID = newKeyID
		ecYaml = manifestbuilders.GetEncryptionClassYaml(class)
		Expect(adminClusterProxy.ApplyWithArgs(ctx, ecYaml)).Should(Succeed(), "failed to update EncryptionClass:\n %s", string(ecYaml))

		By("Verify VM is re-encrypted with new key")
		Eventually(func(g Gomega) {
			vm, err := utils.GetVirtualMachineA3(ctx, svClusterClient, tmpNamespaceName, vmName)
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(vm.Status.Crypto).NotTo(BeNil())
			g.Expect(vm.Status.Crypto.ProviderID).To(Equal(class.Name))
			g.Expect(vm.Status.Crypto.KeyID).To(Equal(newKeyID))
			g.Expect(vm.Status.Crypto.KeyID).NotTo(Equal(initialKeyID))
		}, config.GetIntervals("default", "wait-virtual-machine-creation")...).Should(Succeed(), "Timed out waiting for VM re-encryption: %s", vmName)

		By("Verify crypto status of volumes reflects the encryption class")
		Eventually(func(g Gomega) {
			vm, err := utils.GetVirtualMachineA3(ctx, svClusterClient, tmpNamespaceName, vmName)
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(vm.Status.Volumes).To(HaveLen(2))

			for _, volume := range vm.Status.Volumes {
				volumeStatusCrypto := volume.Crypto
				g.Expect(volumeStatusCrypto).NotTo(BeNil())
				g.Expect(volumeStatusCrypto.ProviderID).To(Equal(class.Name))
			}
		}, config.GetIntervals("default", "wait-virtual-machine-creation")...).Should(Succeed(), "Timed out waiting for PVC volume re-encryption: %s", vmName)
	})

	It("Error when PVC encryption class uses different provider type than VM", func() {
		if !byokFSSEnabled {
			Skip("BYOK FSS is not enabled")
		}

		adminClusterProxy, err := clusterProxy.NewAdminClusterProxy(ctx)
		Expect(err).ToNot(HaveOccurred())

		defer adminClusterProxy.Dispose(ctx)

		By("Create Encryption Class using native key provider for the VM")

		vmClass := manifestbuilders.EncryptionClass{
			Namespace:   tmpNamespaceName,
			Name:        nativeKeyProviderID,
			KeyProvider: nativeKeyProviderID,
		}
		vmECYaml := manifestbuilders.GetEncryptionClassYaml(vmClass)
		Expect(adminClusterProxy.CreateWithArgs(ctx, vmECYaml)).Should(Succeed(), "failed to create EncryptionClass:\n %s", string(vmECYaml))
		useKeyProvider(ctx, cryptoManager, nativeKeyProviderID)

		By("Create Encryption Class using standard key provider for the PVC")

		pvcClass := manifestbuilders.EncryptionClass{
			Namespace:   tmpNamespaceName,
			Name:        standardKeyProviderID,
			KeyProvider: standardKeyProviderID,
		}
		pvcECYaml := manifestbuilders.GetEncryptionClassYaml(pvcClass)
		Expect(adminClusterProxy.CreateWithArgs(ctx, pvcECYaml)).Should(Succeed(), "failed to create EncryptionClass:\n %s", string(pvcECYaml))

		By("Create PVC annotated with the standard key provider encryption class")

		clientSet := clusterProxy.GetClientSet()
		pvcName := vmName + "-pvc"
		testutils.AssertCreatePVC(clientSet, pvcName, tmpNamespaceName, utils.E2EEncryptionStorageClassName)
		volumeClaims := clientSet.CoreV1().PersistentVolumeClaims(tmpNamespaceName)
		pvc, err := volumeClaims.Get(ctx, pvcName, metav1.GetOptions{})
		Expect(err).ToNot(HaveOccurred())

		pvc.Annotations["csi.vsphere.encryption-class"] = pvcClass.Name
		_, err = volumeClaims.Update(ctx, pvc, metav1.UpdateOptions{})
		Expect(err).ToNot(HaveOccurred())

		By("Create VM with native provider encryption class and PVC using standard provider")

		vmParameters := manifestbuilders.VirtualMachineYaml{
			Namespace:        tmpNamespaceName,
			Name:             vmName,
			ImageName:        vmiName,
			VMClassName:      "best-effort-small",
			StorageClassName: utils.E2EEncryptionStorageClassName,
			ResourcePolicy:   clusterResources.VMResourcePolicyName,
			PowerState:       string(vmopv1a3.VirtualMachinePowerStateOn),
			PVCNames:         []string{pvcName},
			Crypto: &manifestbuilders.Crypto{
				EncryptionClassName: vmClass.Name,
			},
		}
		vmYaml = manifestbuilders.GetVirtualMachineYamlA3(vmParameters)
		Expect(clusterProxy.CreateWithArgs(ctx, vmYaml)).Should(Succeed(), "failed to create virtualmachine:\n %s", string(vmYaml))

		By("Verify the encryption synced condition reports an error due to mixed provider types")
		Eventually(func(g Gomega) {
			vm, err := utils.GetVirtualMachineA3(ctx, svClusterClient, tmpNamespaceName, vmName)
			g.Expect(err).ToNot(HaveOccurred())

			condition := vmoperator.GetVirtualMachineConditionA3(vm, vmopv1a3.VirtualMachineEncryptionSynced)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(condition.Status).To(Equal(metav1.ConditionFalse),
				"expected EncryptionSynced to be False due to mixed provider types, got %s with reason %s: %s",
				condition.Status, condition.Reason, condition.Message)
		}, config.GetIntervals("default", "wait-virtual-machine-creation")...).Should(Succeed(),
			"Timed out waiting for encryption error with mixed provider types: %s", vmName)
	})
}

func useKeyProvider(ctx context.Context, cryptoManager *crypto.ManagerKmip, keyProviderID string) {
	By(fmt.Sprintf("Use %s key provider and mark it as default", keyProviderID))
	status, err := cryptoManager.GetClusterStatus(ctx, keyProviderID)
	Expect(err).NotTo(HaveOccurred(), "error fetching status of key provider %s", keyProviderID)

	Expect(status.OverallStatus).To(Equal(types.ManagedEntityStatusGreen))

	Expect(cryptoManager.SetDefaultKmsClusterId(ctx, keyProviderID, nil)).To(Succeed())
}

func waitForCryptoCondition(ctx context.Context, _ *e2eConfig.E2EConfig, client ctrlclient.Client, ns string, vmName string, reason string) *vmopv1a3.VirtualMachineCryptoStatus {
	expectedCondition := metav1.Condition{
		Type:   vmopv1a3.VirtualMachineEncryptionSynced,
		Status: metav1.ConditionTrue,
	}
	if reason != "" {
		expectedCondition.Status = metav1.ConditionFalse
		expectedCondition.Reason = reason
	}

	By(fmt.Sprintf("Waiting for VirtualMachine.Status.Conditions[%s]", expectedCondition.Type))
	// Note: this is vmoperator.WaitOnVirtualMachineCondition with diff:
	// - Using A3
	// - Check Reason before Status, gives more context on failure
	// - Much shorter timeout
	Eventually(func(g Gomega) {
		vm, err := utils.GetVirtualMachineA3(ctx, client, ns, vmName)
		g.Expect(err).ToNot(HaveOccurred())

		actualCondition := vmoperator.GetVirtualMachineConditionA3(vm, expectedCondition.Type)
		g.Expect(actualCondition).ToNot(BeNil())

		if actualCondition.Status == metav1.ConditionFalse {
			g.Expect(actualCondition.Reason).Should(Equal(expectedCondition.Reason))
		}

		g.Expect(actualCondition.Status).Should(Equal(expectedCondition.Status))
	}, time.Minute*2, time.Second*5).Should(Succeed(), fmt.Sprintf("%s condition not %s", expectedCondition.Type, expectedCondition.Status))

	By("Checking VirtualMachine.Status.Crypto")

	vm, err := utils.GetVirtualMachineA3(ctx, client, ns, vmName)
	Expect(err).To(BeNil())

	if expectedCondition.Status == metav1.ConditionTrue {
		Expect(vm.Status.Crypto.KeyID).NotTo(BeEmpty())
		Expect(vm.Status.Crypto.Encrypted).NotTo(BeEmpty())
		Expect(vm.Status.Crypto.ProviderID).NotTo(BeEmpty())
	} else {
		Expect(vm.Status.Crypto).To(BeNil())
	}

	return vm.Status.Crypto
}
