// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	e2eframework "k8s.io/kubernetes/test/e2e/framework"
	capiutil "sigs.k8s.io/cluster-api/util"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/testbed"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/wcp"
	"github.com/vmware-tanzu/vm-operator/test/e2e/manifestbuilders"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/common"
	e2eConfig "github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/consts"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/lib/vmoperator"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/skipper"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/vmservice"
	"github.com/vmware-tanzu/vm-operator/test/e2e/wcpframework"
)

// VMLocationSpecInput is the input to VMLocationSpec.
type VMLocationSpecInput struct {
	ClusterProxy     wcpframework.WCPClusterProxyInterface
	Config           *e2eConfig.E2EConfig
	WCPClient        wcp.WorkloadManagementAPI
	ArtifactFolder   string
	WCPNamespaceName string
}

// VMLocationSpec validates that the VirtualMachineInValidLocation condition is set correctly
// when a VM is created in, moved out of, or returned to the expected vCenter inventory location.
func VMLocationSpec(ctx context.Context, inputGetter func() VMLocationSpecInput) {
	const (
		specName = "vm-location"
		vmKind   = "VirtualMachine"
	)

	var (
		input              VMLocationSpecInput
		config             *e2eConfig.E2EConfig
		clusterProxy       *common.VMServiceClusterProxy
		svClusterClient    ctrlclient.Client
		vCenterAdminClient *vim25.Client
		clusterResources   *e2eConfig.Resources

		vmName       string
		linuxVMIName string
	)

	BeforeEach(func() {
		input = inputGetter()
		Expect(input.Config).ToNot(BeNil(),
			"Invalid argument. input.E2EConfig can't be nil when calling %s spec", specName)
		Expect(input.Config.InfraConfig).ToNot(BeNil(),
			"Invalid argument. input.E2EConfig.InfraConfig can't be nil when calling %s spec", specName)
		skipper.SkipUnlessInfraIs(input.Config.InfraConfig.InfraName, consts.WCP)

		Expect(input.ClusterProxy).ToNot(BeNil(),
			"Invalid argument. input.ClusterProxy can't be nil when calling %s spec", specName)
		Expect(input.WCPNamespaceName).ToNot(BeEmpty(),
			"Invalid argument. input.WCPNamespaceName can't be empty when calling %s spec", specName)
		Expect(os.MkdirAll(input.ArtifactFolder, 0755)).To(Succeed(),
			"Invalid argument. input.ArtifactFolder can't be created for %s spec", specName)

		config = input.Config
		clusterResources = config.InfraConfig.ManagementClusterConfig.Resources
		clusterProxy = input.ClusterProxy.(*common.VMServiceClusterProxy)

		cancelPodWatches := framework.WatchPodLogsAndEventsInNamespaces(
			ctx,
			[]string{config.GetVariable("VMOPNamespace")},
			clusterProxy.GetClientSet(),
			filepath.Join(input.ArtifactFolder, specName),
		)
		DeferCleanup(cancelPodWatches)

		svClusterClient = clusterProxy.GetClient()

		kubeconfigPath := clusterProxy.GetKubeconfigPath()
		vCenterHostname := vcenter.GetVCPNIDFromKubeconfigFile(ctx, kubeconfigPath)

		var err error
		vCenterAdminClient, err = vcenter.NewVimClient(vCenterHostname, testbed.AdminUsername, testbed.AdminPassword)
		Expect(err).ToNot(HaveOccurred(), "Failed to create vCenter admin client")

		linuxImageDisplayName := vmservice.GetDefaultImageDisplayName(clusterResources)
		linuxVMIName, err = vmoperator.WaitForVirtualMachineImageName(
			ctx, &config.Config, svClusterClient,
			input.WCPNamespaceName, linuxImageDisplayName)
		Expect(err).NotTo(HaveOccurred(),
			"failed to get VMI name for display name %q in namespace %q",
			linuxImageDisplayName, input.WCPNamespaceName)

		vmName = fmt.Sprintf("%s-%s", specName, capiutil.RandomString(4))
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			vmoperator.DescribeResourceIfExists(
				ctx, svClusterClient,
				clusterProxy.GetKubeconfigPath(),
				input.WCPNamespaceName, vmName, vmKind)
		}

		vmoperator.VerifyVMDeleted(ctx, svClusterClient, config, input.WCPNamespaceName, vmName)
		vcenter.LogoutVimClient(vCenterAdminClient)
	})

	// getNsRPAndFolder returns the namespace RP MoID and folder MoID for the
	// WCP namespace.  It mirrors the lookup order in topology.GetNamespaceFolderAndRPMoID:
	// Zone.Spec.ManagedVMs first, then AvailabilityZone.Spec.Namespaces as fallback.
	getNsRPAndFolder := func(namespace string) (rpMoID, folderMoID string) {
		zoneList := &topologyv1.ZoneList{}
		Expect(svClusterClient.List(ctx, zoneList, ctrlclient.InNamespace(namespace))).
			To(Succeed(), "failed to list Zones for namespace %s", namespace)

		for _, z := range zoneList.Items {
			if len(z.Spec.ManagedVMs.PoolMoIDs) > 0 {
				e2eframework.Logf("resolved namespace RP from Zone %s: %s / %s",
					z.Name, z.Spec.ManagedVMs.PoolMoIDs[0], z.Spec.ManagedVMs.FolderMoID)
				return z.Spec.ManagedVMs.PoolMoIDs[0], z.Spec.ManagedVMs.FolderMoID
			}
		}

		// Fallback: AvailabilityZone.Spec.Namespaces (older cluster configurations).
		azList := &topologyv1.AvailabilityZoneList{}
		Expect(svClusterClient.List(ctx, azList)).
			To(Succeed(), "failed to list AvailabilityZones")
		Expect(azList.Items).ToNot(BeEmpty(), "expected at least one AvailabilityZone")

		for _, az := range azList.Items {
			if nsInfo, ok := az.Spec.Namespaces[namespace]; ok && nsInfo.PoolMoId != "" {
				e2eframework.Logf("resolved namespace RP from AvailabilityZone %s: %s / %s",
					az.Name, nsInfo.PoolMoId, nsInfo.FolderMoId)
				return nsInfo.PoolMoId, nsInfo.FolderMoId
			}
		}

		Fail("could not determine namespace RP and folder MoIDs for namespace " + namespace)
		return "", ""
	}

	// createVM deploys a VM and waits for it to reach Running state.
	createVM := func() {
		vmParameters := manifestbuilders.VirtualMachineYaml{
			Namespace:        input.WCPNamespaceName,
			Name:             vmName,
			ImageName:        linuxVMIName,
			VMClassName:      clusterResources.VMClassName,
			StorageClassName: clusterResources.StorageClassName,
			PowerState:       "PoweredOn",
		}
		vmYaml := manifestbuilders.GetVirtualMachineYamlA6(vmParameters)
		e2eframework.Logf("Creating VirtualMachine %s", vmName)
		Expect(clusterProxy.CreateWithArgs(ctx, vmYaml)).To(Succeed(),
			"failed to create VM %s", vmName)
		vmoperator.WaitForVirtualMachineCreation(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
	}

	// relocateVM moves the VM to the given resource pool and/or folder MoID.
	// Pass an empty string to leave that field unchanged.
	relocateVM := func(vmMoID, poolMoID, folderMoID string) {
		vmObj := object.NewVirtualMachine(vCenterAdminClient, vimtypes.ManagedObjectReference{
			Type:  "VirtualMachine",
			Value: vmMoID,
		})
		spec := vimtypes.VirtualMachineRelocateSpec{}
		if poolMoID != "" {
			ref := vimtypes.ManagedObjectReference{Type: "ResourcePool", Value: poolMoID}
			spec.Pool = &ref
		}
		if folderMoID != "" {
			ref := vimtypes.ManagedObjectReference{Type: "Folder", Value: folderMoID}
			spec.Folder = &ref
		}
		task, err := vmObj.Relocate(ctx, spec, vimtypes.VirtualMachineMovePriorityDefaultPriority)
		Expect(err).ToNot(HaveOccurred(), "failed to start Relocate task for VM %s", vmMoID)
		Expect(task.Wait(ctx)).To(Succeed(), "Relocate task failed for VM %s", vmMoID)
	}

	When("VM is created in the correct namespace RP and folder", Label("vmrelocation"), func() {
		It("sets VirtualMachineInValidLocation condition to True", func() {
			createVM()

			vmoperator.WaitOnVirtualMachineCondition(ctx, config, svClusterClient,
				input.WCPNamespaceName, vmName, metav1.Condition{
					Type:   vmopv1.VirtualMachineInValidLocation,
					Status: metav1.ConditionTrue,
				})
		})
	})

	When("VM is moved outside the namespace RP hierarchy", Label("vmrelocation"), func() {
		It("sets condition False, then recovers to True when VM is returned to the correct location", func() {
			By("Creating VM and waiting for it to reach Running state")
			createVM()

			By("Waiting for VirtualMachineInValidLocation=True after initial creation")
			vmoperator.WaitOnVirtualMachineCondition(ctx, config, svClusterClient,
				input.WCPNamespaceName, vmName, metav1.Condition{
					Type:   vmopv1.VirtualMachineInValidLocation,
					Status: metav1.ConditionTrue,
				})

			vmMoID := vmoperator.GetVirtualMachineMOID(ctx, svClusterClient, input.WCPNamespaceName, vmName)
			Expect(vmMoID).ToNot(BeEmpty(), "VM must have a UniqueID before relocation")

			By("Retrieving the correct namespace RP and folder MoIDs")
			nsRPMoID, nsFolderMoID := getNsRPAndFolder(input.WCPNamespaceName)

			By("Retrieving the cluster root RP to use as an invalid location")
			// 1. Resolve the specific Cluster MoID for the active Supervisor context
			kubeconfigPath := clusterProxy.GetKubeconfigPath()
			clusterMoID := vcenter.GetClusterMoIDFromKubeconfigFile(ctx, kubeconfigPath)

			// 2. Create an explicit ManagedObjectReference using the real Cluster ID
			clusterMoRef := vimtypes.ManagedObjectReference{
				Type:  "ClusterComputeResource",
				Value: clusterMoID,
			}
			clusterRef := object.NewClusterComputeResource(vCenterAdminClient, clusterMoRef)

			// 3. Extract the root Resource Pool from the verified cluster
			clusterRP, err := clusterRef.ResourcePool(ctx)
			Expect(err).ToNot(HaveOccurred(), "Failed to get the root Resource Pool for cluster %s", clusterMoID)
			clusterRPRef := clusterRP.Reference()
			e2eframework.Logf("cluster root RP MoID: %s", clusterRPRef.Value)

			By("Relocating VM to the cluster root RP (outside the namespace RP hierarchy)")
			relocateVM(vmMoID, clusterRPRef.Value, "")

			By("Waiting for VirtualMachineInValidLocation condition to become False")
			vmoperator.WaitOnVirtualMachineCondition(ctx, config, svClusterClient,
				input.WCPNamespaceName, vmName, metav1.Condition{
					Type:   vmopv1.VirtualMachineInValidLocation,
					Status: metav1.ConditionFalse,
					Reason: "LocationMismatch",
				})

			By("Relocating VM back to the correct namespace RP and folder")
			relocateVM(vmMoID, nsRPMoID, nsFolderMoID)

			By("Waiting for VirtualMachineInValidLocation condition to return to True")
			vmoperator.WaitOnVirtualMachineCondition(ctx, config, svClusterClient,
				input.WCPNamespaceName, vmName, metav1.Condition{
					Type:   vmopv1.VirtualMachineInValidLocation,
					Status: metav1.ConditionTrue,
				})
		})
	})

	When("VM is moved outside the namespace Folder hierarchy", Label("vmrelocation"), func() {
		It("sets condition False, then recovers to True when VM is returned to the correct location", func() {
			By("Creating VM and waiting for it to reach Running state")
			createVM()

			By("Waiting for VirtualMachineInValidLocation=True after initial creation")
			vmoperator.WaitOnVirtualMachineCondition(ctx, config, svClusterClient,
				input.WCPNamespaceName, vmName, metav1.Condition{
					Type:   vmopv1.VirtualMachineInValidLocation,
					Status: metav1.ConditionTrue,
				})

			vmMoID := vmoperator.GetVirtualMachineMOID(ctx, svClusterClient, input.WCPNamespaceName, vmName)
			Expect(vmMoID).ToNot(BeEmpty(), "VM must have a UniqueID before relocation")

			By("Retrieving the correct namespace folder MoID")
			_, nsFolderMoID := getNsRPAndFolder(input.WCPNamespaceName)

			By("Retrieving the parent of the namespace folder as the invalid folder location")
			// The namespace folder's parent (the DC's root VM folder) is always a
			// Folder-typed MoRef and always lies outside the 2-level hierarchy that
			// validateVMFolder checks, so it reliably triggers the LocationMismatch.
			pc := property.DefaultCollector(vCenterAdminClient)
			var nsFolderMo mo.Folder
			Expect(pc.RetrieveOne(ctx,
				vimtypes.ManagedObjectReference{Type: "Folder", Value: nsFolderMoID},
				[]string{"parent"},
				&nsFolderMo,
			)).To(Succeed(), "failed to fetch namespace folder's parent")
			Expect(nsFolderMo.Parent).ToNot(BeNil(), "namespace folder has no parent")
			Expect(nsFolderMo.Parent.Type).To(Equal("Folder"),
				"namespace folder's parent must be a Folder (got %s %s)",
				nsFolderMo.Parent.Type, nsFolderMo.Parent.Value)
			invalidFolderMoID := nsFolderMo.Parent.Value
			Expect(invalidFolderMoID).ToNot(Equal(nsFolderMoID),
				"namespace folder's parent unexpectedly equals the namespace folder itself")
			e2eframework.Logf("invalid folder MoID (parent of namespace folder): %s", invalidFolderMoID)

			By("Moving VM into the DC root VM folder via MoveIntoFolder (direct inventory move)")
			// Use Folder.MoveInto rather than Relocate.Folder: in WCP, the
			// Relocate API honors Pool changes but silently ignores the Folder
			// field because WCP controls namespace folder placement.
			// MoveIntoFolder_Task is a pure vCenter inventory move that bypasses
			// this restriction and actually changes the VM's parent in vCenter.
			invalidFolderObj := object.NewFolder(vCenterAdminClient,
				vimtypes.ManagedObjectReference{Type: "Folder", Value: invalidFolderMoID})
			moveTask, err := invalidFolderObj.MoveInto(ctx, []vimtypes.ManagedObjectReference{
				{Type: "VirtualMachine", Value: vmMoID},
			})
			Expect(err).ToNot(HaveOccurred(), "failed to start MoveIntoFolder task")
			Expect(moveTask.Wait(ctx)).To(Succeed(), "MoveIntoFolder task failed for VM %s", vmMoID)

			By("Verifying the VM actually moved to the invalid folder")
			var vmMoAfterMove mo.VirtualMachine
			Expect(pc.RetrieveOne(ctx,
				vimtypes.ManagedObjectReference{Type: "VirtualMachine", Value: vmMoID},
				[]string{"parent"},
				&vmMoAfterMove,
			)).To(Succeed(), "failed to fetch VM parent after move")
			e2eframework.Logf("VM parent after MoveIntoFolder: type=%s value=%s (expected=%s)",
				vmMoAfterMove.Parent.Type, vmMoAfterMove.Parent.Value, invalidFolderMoID)
			Expect(vmMoAfterMove.Parent.Value).To(Equal(invalidFolderMoID),
				"VM did not move to the DC root VM folder; actual parent: %s", vmMoAfterMove.Parent.Value)

			By("Waiting for VirtualMachineInValidLocation condition to become False")
			vmoperator.WaitOnVirtualMachineCondition(ctx, config, svClusterClient,
				input.WCPNamespaceName, vmName, metav1.Condition{
					Type:   vmopv1.VirtualMachineInValidLocation,
					Status: metav1.ConditionFalse,
					Reason: "LocationMismatch",
				})

			By("Moving VM back into the namespace folder")
			nsFolderObj := object.NewFolder(vCenterAdminClient,
				vimtypes.ManagedObjectReference{Type: "Folder", Value: nsFolderMoID})
			recoverTask, recoverErr := nsFolderObj.MoveInto(ctx, []vimtypes.ManagedObjectReference{
				{Type: "VirtualMachine", Value: vmMoID},
			})
			Expect(recoverErr).ToNot(HaveOccurred(), "failed to start MoveIntoFolder recovery task")
			Expect(recoverTask.Wait(ctx)).To(Succeed(), "MoveIntoFolder recovery task failed for VM %s", vmMoID)

			By("Waiting for VirtualMachineInValidLocation condition to return to True")
			vmoperator.WaitOnVirtualMachineCondition(ctx, config, svClusterClient,
				input.WCPNamespaceName, vmName, metav1.Condition{
					Type:   vmopv1.VirtualMachineInValidLocation,
					Status: metav1.ConditionTrue,
				})
		})
	})
}
