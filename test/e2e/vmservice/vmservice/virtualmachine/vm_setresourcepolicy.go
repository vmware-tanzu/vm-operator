// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vapi/cluster"
	"github.com/vmware/govmomi/vim25"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	capiutil "sigs.k8s.io/cluster-api/util"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/testbed"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/wcp"
	"github.com/vmware-tanzu/vm-operator/test/e2e/utils"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/common"
	e2eConfig "github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/consts"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/skipper"
	"github.com/vmware-tanzu/vm-operator/test/e2e/wcpframework"
)

// VMSetResourcePolicySpecInput is the input to VMSetResourcePolicySpec.
type VMSetResourcePolicySpecInput struct {
	ClusterProxy   wcpframework.WCPClusterProxyInterface
	Config         *e2eConfig.E2EConfig
	ArtifactFolder string
}

// VMSetResourcePolicySpec verifies the VirtualMachineSetResourcePolicy
// controller:
//
//   - Basic reconciles the vSphere child ResourcePool, Folder, and cluster
//     modules described by the policy, reports them in status, and removes
//     them when the policy is deleted.
//   - Zone decommission drops the cluster modules of a vSphere Zone (workload
//     domain) that is removed from the namespace. The VMSetRP spec is
//     immutable, but the set of Zones backing a namespace is not, so the
//     controller must best-effort delete the modules it previously created on
//     the removed Zone's ClusterComputeResource and stop reporting them.
func VMSetResourcePolicySpec(ctx context.Context, inputGetter func() VMSetResourcePolicySpecInput) {
	const (
		specName = "vm-set-resource-policy"

		// zone-1 is the default Zone bound to every namespace and cannot be
		// removed, so a different Zone is decommissioned in the Zone
		// decommission spec.
		defaultZoneName = "zone-1"
	)

	var (
		input   VMSetResourcePolicySpecInput
		setup   *vmsetRPSpecSetup
		vmsetRP *vmopv1.VirtualMachineSetResourcePolicy
	)

	BeforeEach(func() {
		input = inputGetter()
		setup = setupVMSetRPSpec(ctx, input, specName)

		vmsetRP = &vmopv1.VirtualMachineSetResourcePolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-%s", specName, capiutil.RandomString(6)),
				Namespace: setup.nsName,
			},
		}
	})

	Context("Basic", func() {
		It("Should reconcile the child ResourcePool, Folder, and cluster modules", Label("core-functional", "experimental"), func() {
			const (
				resourcePoolName = "e2e-vmsetrp-resource-pool"
				folderName       = "e2e-vmsetrp-folder"
			)
			groupNames := []string{"e2e-vmsetrp-group-1", "e2e-vmsetrp-group-2"}

			By("Resolving the namespace's vSphere Zone entities")
			nsFolderMoID, nsPoolMoIDs, nsClusterMoIDs := namespaceVSphereEntities(ctx, setup.svClusterClient, setup.nsName)
			Expect(nsFolderMoID).ToNot(BeEmpty(), "namespace folder MoID should not be empty")
			Expect(nsPoolMoIDs).ToNot(BeEmpty(), "namespace resource pool MoIDs should not be empty")
			Expect(nsClusterMoIDs).ToNot(BeEmpty(), "namespace cluster MoIDs should not be empty")

			vmsetRP.Spec = vmopv1.VirtualMachineSetResourcePolicySpec{
				ResourcePool: vmopv1.ResourcePoolSpec{
					Name: resourcePoolName,
				},
				Folder:              folderName,
				ClusterModuleGroups: groupNames,
			}

			By(fmt.Sprintf("Creating VirtualMachineSetResourcePolicy %s", ctrlclient.ObjectKeyFromObject(vmsetRP)))
			Expect(setup.adminClient.Create(ctx, vmsetRP)).To(Succeed(), "failed to create VirtualMachineSetResourcePolicy")

			key := ctrlclient.ObjectKeyFromObject(vmsetRP)
			interval := setup.config.GetIntervals("default", "wait-vmsetrp-clustermodules")

			By("Waiting for the VirtualMachineSetResourcePolicy status to reflect the vSphere resources")
			Eventually(func(g Gomega) {
				rp := &vmopv1.VirtualMachineSetResourcePolicy{}
				g.Expect(setup.adminClient.Get(ctx, key, rp)).To(Succeed())

				g.Expect(rp.Spec.ResourcePool.Name).To(Equal(resourcePoolName))
				g.Expect(rp.Spec.Folder).To(Equal(folderName))
				g.Expect(rp.Spec.ClusterModuleGroups).To(Equal(groupNames))

				g.Expect(rp.Status.ResourcePools).To(HaveLen(len(nsPoolMoIDs)))
				for _, rpStatus := range rp.Status.ResourcePools {
					g.Expect(rpStatus.ClusterMoID).ToNot(BeEmpty())
					g.Expect(nsClusterMoIDs).To(ContainElement(rpStatus.ClusterMoID))
					g.Expect(rpStatus.ChildResourcePoolMoID).ToNot(BeEmpty())
				}

				clusterMoIDs := distinctClusterMoIDs(rp)
				g.Expect(clusterMoIDs).ToNot(BeEmpty())
				g.Expect(nsClusterMoIDs).To(ContainElements(clusterMoIDs))
				g.Expect(rp.Status.ClusterModules).To(HaveLen(len(groupNames) * len(clusterMoIDs)))
			}, interval...).Should(Succeed(), "VirtualMachineSetResourcePolicy status did not converge")

			rp := &vmopv1.VirtualMachineSetResourcePolicy{}
			Expect(setup.adminClient.Get(ctx, key, rp)).To(Succeed())

			By("Verifying the child ResourcePool exists under each namespace ResourcePool")
			var statusChildRPMoIDs []string
			for _, rpStatus := range rp.Status.ResourcePools {
				statusChildRPMoIDs = append(statusChildRPMoIDs, rpStatus.ChildResourcePoolMoID)
			}
			var foundChildRPMoIDs []string
			for _, poolMoID := range nsPoolMoIDs {
				parentRP := object.NewResourcePool(setup.vCenterClient,
					vimtypes.ManagedObjectReference{Type: "ResourcePool", Value: poolMoID})
				child, err := findChild(ctx, setup.vCenterClient, parentRP, resourcePoolName)
				Expect(err).ToNot(HaveOccurred(), "failed to find child ResourcePool %q", resourcePoolName)
				Expect(child).ToNot(BeNil(), "expected child ResourcePool %q under %s", resourcePoolName, poolMoID)
				foundChildRPMoIDs = append(foundChildRPMoIDs, child.Reference().Value)
			}
			Expect(statusChildRPMoIDs).To(ConsistOf(foundChildRPMoIDs))

			By("Verifying the child Folder exists under the namespace Folder")
			nsFolder := object.NewFolder(setup.vCenterClient,
				vimtypes.ManagedObjectReference{Type: "Folder", Value: nsFolderMoID})
			childFolder, err := findChild(ctx, setup.vCenterClient, nsFolder, folderName)
			Expect(err).ToNot(HaveOccurred(), "failed to find child Folder %q", folderName)
			Expect(childFolder).ToNot(BeNil(), "expected child Folder %q under %s", folderName, nsFolderMoID)
			Expect(childFolder.Reference().Type).To(Equal("Folder"))

			By("Verifying the cluster modules exist on vCenter")
			moduleUUIDs, err := clusterModuleUUIDs(ctx, setup.clusterModuleManager)
			Expect(err).ToNot(HaveOccurred(), "failed to list cluster modules")
			for _, cm := range rp.Status.ClusterModules {
				Expect(moduleUUIDs).To(ContainElement(cm.ModuleUuid), "cluster module %s is missing on vCenter", cm.ModuleUuid)
			}

			By("Deleting the VirtualMachineSetResourcePolicy")
			Expect(setup.adminClient.Get(ctx, key, vmsetRP)).To(Succeed())
			Expect(setup.adminClient.Delete(ctx, vmsetRP)).To(Succeed())
			Eventually(func(g Gomega) {
				err := setup.adminClient.Get(ctx, key, &vmopv1.VirtualMachineSetResourcePolicy{})
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "VirtualMachineSetResourcePolicy %s should be deleted", key)
			}, interval...).Should(Succeed())

			By("Verifying the child ResourcePool and Folder were deleted from vCenter")
			for _, poolMoID := range nsPoolMoIDs {
				parentRP := object.NewResourcePool(setup.vCenterClient,
					vimtypes.ManagedObjectReference{Type: "ResourcePool", Value: poolMoID})
				child, err := findChild(ctx, setup.vCenterClient, parentRP, resourcePoolName)
				Expect(err).ToNot(HaveOccurred(), "failed to look up child ResourcePool %q", resourcePoolName)
				Expect(child).To(BeNil(), "child ResourcePool %q under %s should be deleted", resourcePoolName, poolMoID)
			}
			childFolder, err = findChild(ctx, setup.vCenterClient, nsFolder, folderName)
			Expect(err).ToNot(HaveOccurred(), "failed to look up child Folder %q", folderName)
			Expect(childFolder).To(BeNil(), "child Folder %q under %s should be deleted", folderName, nsFolderMoID)

			By("Verifying the cluster modules were deleted from vCenter")
			deletedModuleUUIDs := make([]string, 0, len(rp.Status.ClusterModules))
			for _, cm := range rp.Status.ClusterModules {
				deletedModuleUUIDs = append(deletedModuleUUIDs, cm.ModuleUuid)
			}
			Eventually(func(g Gomega) {
				moduleUUIDs, err := clusterModuleUUIDs(ctx, setup.clusterModuleManager)
				g.Expect(err).ToNot(HaveOccurred(), "failed to list cluster modules")
				for _, uuid := range deletedModuleUUIDs {
					g.Expect(moduleUUIDs).ToNot(ContainElement(uuid), "cluster module %s should be deleted from vCenter", uuid)
				}
			}, interval...).Should(Succeed())
		})
	})

	Context("Zone decommission", func() {
		BeforeEach(func() {
			// A Zone has to be removable, which needs a stretched Supervisor
			// with more than one workload domain.
			skipper.SkipUnlessStretchSupervisorIsEnabled()

			By("Binding all Zones bound with the Supervisor to the temporary namespace")

			supervisorID := vcenter.GetSupervisorIDFromKubeconfig(ctx, setup.config.InfraConfig.KubeconfigPath)
			Expect(supervisorID).ToNot(BeEmpty(), "Supervisor ID should not be empty")
			supervisorZones, err := setup.clusterProxy.GetZonesBoundWithSupervisor(supervisorID)
			Expect(err).ToNot(HaveOccurred(), "failed to get Zones bound with Supervisor")

			namespaceZones, err := utils.ListZonesByNamespace(ctx, setup.svClusterClient, setup.nsName)
			Expect(err).ToNot(HaveOccurred(), "failed to list Zones for namespace %s", setup.nsName)

			boundZones := make(map[string]struct{}, len(namespaceZones.Items))
			for _, zone := range namespaceZones.Items {
				boundZones[zone.Name] = struct{}{}
			}

			var unboundZones []string
			for _, zone := range supervisorZones.Zones {
				if _, ok := boundZones[zone.Zone]; !ok {
					unboundZones = append(unboundZones, zone.Zone)
				}
			}

			if len(unboundZones) > 0 {
				_, err = setup.clusterProxy.UpdateNamespaceWithZones(ctx, setup.nsName, unboundZones, setup.svClusterClient)
				Expect(err).ToNot(HaveOccurred(), "failed to update namespace with Zones")
			}

			// A Zone other than the default must be bound to the namespace so
			// it can be decommissioned in the spec below.
			namespaceZones, err = utils.ListZonesByNamespace(ctx, setup.svClusterClient, setup.nsName)
			Expect(err).ToNot(HaveOccurred(), "failed to list Zones for namespace %s", setup.nsName)

			var hasRemovableZone bool
			for _, zone := range namespaceZones.Items {
				if zone.Name != defaultZoneName {
					hasRemovableZone = true
					break
				}
			}
			if !hasRemovableZone {
				Skip("no non-default Zone is bound to the temporary namespace")
			}
		})

		It("Should remove the cluster modules of a decommissioned Zone", Label("core-functional", "experimental"), func() {
			namespaceZones, err := utils.ListZonesByNamespace(ctx, setup.svClusterClient, setup.nsName)
			Expect(err).ToNot(HaveOccurred(), "failed to list Zones for namespace %s", setup.nsName)

			var removedZone topologyv1.Zone
			for _, zone := range namespaceZones.Items {
				if zone.Name != defaultZoneName {
					removedZone = zone
					break
				}
			}
			Expect(removedZone.Name).ToNot(BeEmpty(), "expected a removable Zone in namespace %s", setup.nsName)

			removedClusterMoIDs := removedZone.Spec.ManagedVMs.ClusterMoIDs
			Expect(removedClusterMoIDs).ToNot(BeEmpty(), "Zone %s has no ClusterMoIDs", removedZone.Name)

			groupNames := []string{"e2e-cluster-module-group-1", "e2e-cluster-module-group-2"}
			vmsetRP.Spec = vmopv1.VirtualMachineSetResourcePolicySpec{
				ClusterModuleGroups: groupNames,
			}

			By(fmt.Sprintf("Creating VirtualMachineSetResourcePolicy %s/%s with cluster module groups", setup.nsName, vmsetRP.Name))
			Expect(setup.adminClient.Create(ctx, vmsetRP)).To(Succeed(), "failed to create VirtualMachineSetResourcePolicy")

			key := ctrlclient.ObjectKeyFromObject(vmsetRP)
			interval := setup.config.GetIntervals("default", "wait-vmsetrp-clustermodules")

			By("Waiting for the VirtualMachineSetResourcePolicy to have cluster modules for every Zone")

			Eventually(func(g Gomega) {
				rp := &vmopv1.VirtualMachineSetResourcePolicy{}
				g.Expect(setup.adminClient.Get(ctx, key, rp)).To(Succeed())

				clusterMoIDs := distinctClusterMoIDs(rp)
				g.Expect(clusterMoIDs).To(ContainElements(removedClusterMoIDs),
					"expected cluster modules for the removable Zone's clusters")
				g.Expect(rp.Status.ClusterModules).To(HaveLen(len(groupNames) * len(clusterMoIDs)))
			}, interval...).Should(Succeed(), "VirtualMachineSetResourcePolicy did not get cluster modules for all Zones")

			rp := &vmopv1.VirtualMachineSetResourcePolicy{}
			Expect(setup.adminClient.Get(ctx, key, rp)).To(Succeed())

			allClusterMoIDs := distinctClusterMoIDs(rp)
			survivingClusterMoIDs := slices.DeleteFunc(slices.Clone(allClusterMoIDs), func(moID string) bool {
				return slices.Contains(removedClusterMoIDs, moID)
			})
			Expect(survivingClusterMoIDs).ToNot(BeEmpty(), "expected cluster modules for at least one surviving Zone")

			var removedModuleUUIDs, survivingModuleUUIDs []string
			for _, cm := range rp.Status.ClusterModules {
				if slices.Contains(removedClusterMoIDs, cm.ClusterMoID) {
					removedModuleUUIDs = append(removedModuleUUIDs, cm.ModuleUuid)
				} else {
					survivingModuleUUIDs = append(survivingModuleUUIDs, cm.ModuleUuid)
				}
			}
			Expect(removedModuleUUIDs).To(HaveLen(len(groupNames) * len(removedClusterMoIDs)))

			By(fmt.Sprintf("Decommissioning Zone %s from namespace %s", removedZone.Name, setup.nsName))
			Expect(setup.clusterProxy.DeleteZonesFromNamespace(ctx, setup.nsName, []string{removedZone.Name}, setup.svClusterClient)).To(Succeed())

			By("Waiting for the removed Zone's cluster modules to be pruned from the VirtualMachineSetResourcePolicy status")

			Eventually(func(g Gomega) {
				rp := &vmopv1.VirtualMachineSetResourcePolicy{}
				g.Expect(setup.adminClient.Get(ctx, key, rp)).To(Succeed())

				for _, cm := range rp.Status.ClusterModules {
					g.Expect(removedClusterMoIDs).ToNot(ContainElement(cm.ClusterMoID),
						"status still has cluster module %s for decommissioned cluster %s", cm.ModuleUuid, cm.ClusterMoID)
				}

				g.Expect(rp.Status.ClusterModules).To(HaveLen(len(groupNames) * len(survivingClusterMoIDs)))
				for _, moID := range survivingClusterMoIDs {
					var cnt int
					for _, cm := range rp.Status.ClusterModules {
						if cm.ClusterMoID == moID {
							cnt++
						}
					}
					g.Expect(cnt).To(Equal(len(groupNames)),
						"expected cluster modules for surviving cluster %s", moID)
				}
			}, interval...).Should(Succeed(), "VirtualMachineSetResourcePolicy status still references the decommissioned Zone")

			By("Verifying the removed Zone's cluster modules were deleted from vCenter")
			waitForClusterModuleUUIDs(ctx, setup.clusterModuleManager, removedModuleUUIDs, survivingModuleUUIDs, interval...)
		})
	})
}

// vmsetRPSpecSetup bundles the objects shared by the VMSetResourcePolicy specs.
type vmsetRPSpecSetup struct {
	config               *e2eConfig.E2EConfig
	clusterResources     *e2eConfig.Resources
	clusterProxy         *common.VMServiceClusterProxy
	svClusterClient      ctrlclient.Client
	adminClient          ctrlclient.Client
	vCenterClient        *vim25.Client
	clusterModuleManager *cluster.Manager
	nsContext            wcpframework.NamespaceContext
	nsName               string
}

// setupVMSetRPSpec creates a temporary Supervisor Namespace plus the admin and
// vCenter clients the VMSetResourcePolicy specs need. It must be called from a
// BeforeEach so the cleanup it registers runs at spec teardown.
func setupVMSetRPSpec(ctx context.Context, input VMSetResourcePolicySpecInput, specName string) *vmsetRPSpecSetup {
	Expect(input.Config).ToNot(BeNil(), "Invalid argument. input.E2EConfig can't be nil when calling %s spec", specName)
	Expect(input.Config.InfraConfig).ToNot(BeNil(), "Invalid argument. input.E2EConfig.InfraConfig can't be nil when calling %s spec", specName)
	Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling %s spec", specName)
	Expect(input.ArtifactFolder).ToNot(BeEmpty(), "Invalid argument. input.ArtifactFolder can't be empty when calling %s spec", specName)
	Expect(os.MkdirAll(input.ArtifactFolder, 0755)).To(Succeed(), "Invalid argument. input.ArtifactFolder can't be created for %s spec", specName)

	skipper.SkipUnlessInfraIs(input.Config.InfraConfig.InfraName, consts.WCP)

	setup := &vmsetRPSpecSetup{
		config:           input.Config,
		clusterResources: input.Config.InfraConfig.ManagementClusterConfig.Resources,
		clusterProxy:     input.ClusterProxy.(*common.VMServiceClusterProxy),
	}
	setup.svClusterClient = setup.clusterProxy.GetClient()

	cancelPodWatches := framework.WatchPodLogsAndEventsInNamespaces(
		ctx,
		[]string{setup.config.GetVariable("VMOPNamespace")},
		setup.clusterProxy.GetRESTConfig(),
		filepath.Join(input.ArtifactFolder, specName),
	)
	DeferCleanup(cancelPodWatches)

	// The Supervisor service account used by the regular cluster proxy has no
	// RBAC for VirtualMachineSetResourcePolicies, so all VMSetRP operations go
	// through the admin client.
	adminProxy, err := setup.clusterProxy.NewAdminClusterProxy(ctx)
	Expect(err).ToNot(HaveOccurred(), "failed to get admin cluster proxy")
	DeferCleanup(func() { adminProxy.Dispose(ctx) })

	setup.adminClient, err = adminProxy.GetAdminClient()
	Expect(err).ToNot(HaveOccurred(), "failed to get admin client")

	setup.vCenterClient = vcenter.NewVimClientFromKubeconfig(ctx, setup.clusterProxy.GetKubeconfigPath())
	DeferCleanup(func() { vcenter.LogoutVimClient(setup.vCenterClient) })
	setup.clusterModuleManager = newClusterModuleManager(ctx, setup.vCenterClient)

	By("Creating a temporary namespace")

	vmsvcSpecs := wcp.NewVMServiceSpecDetails([]string{}, []string{})
	setup.nsContext, err = setup.clusterProxy.CreateWCPNamespace(ctx, setup.config, vmsvcSpecs,
		setup.clusterResources.StorageClassName,
		fmt.Sprintf("%s-%s", specName, capiutil.RandomString(6)),
		input.ArtifactFolder)
	Expect(err).ToNot(HaveOccurred(), "failed to create WCP namespace")
	Expect(setup.nsContext.GetNamespace()).ToNot(BeNil(), "namespace should not be nil")
	setup.nsName = setup.nsContext.GetNamespace().Name

	DeferCleanup(func() {
		if setup.nsName != "" {
			setup.clusterProxy.DeleteWCPNamespace(setup.nsContext)
			setup.nsName = ""
		}
	})

	return setup
}

// namespaceVSphereEntities returns the namespace's managed-VM Folder, and the
// ResourcePool and ClusterComputeResource MoIDs backing it, read from the
// namespace's Zones. The Zone's ManagedVMs entity info is the same source
// topology.GetNamespaceFolderAndRPMoIDs uses when WorkloadDomainIsolation is
// enabled. Callers gate on that FSS, so a namespace without any namespaced
// Zones is a test failure rather than an empty result.
func namespaceVSphereEntities(
	ctx context.Context,
	client ctrlclient.Client,
	namespace string,
) (folderMoID string, poolMoIDs, clusterMoIDs []string) {
	zones, err := utils.ListZonesByNamespace(ctx, client, namespace)
	Expect(err).ToNot(HaveOccurred(), "failed to list Zones for namespace %s", namespace)

	for _, zone := range zones.Items {
		if folderMoID == "" {
			folderMoID = zone.Spec.ManagedVMs.FolderMoID
		}
		poolMoIDs = append(poolMoIDs, zone.Spec.ManagedVMs.PoolMoIDs...)
		clusterMoIDs = append(clusterMoIDs, zone.Spec.ManagedVMs.ClusterMoIDs...)
	}

	return folderMoID, poolMoIDs, clusterMoIDs
}

// distinctClusterMoIDs returns the distinct ClusterMoIDs referenced by the
// VirtualMachineSetResourcePolicy status.
func distinctClusterMoIDs(rp *vmopv1.VirtualMachineSetResourcePolicy) []string {
	var moIDs []string
	for _, cm := range rp.Status.ClusterModules {
		if !slices.Contains(moIDs, cm.ClusterMoID) {
			moIDs = append(moIDs, cm.ClusterMoID)
		}
	}
	return moIDs
}

// newClusterModuleManager returns a vCenter REST-backed cluster module manager.
func newClusterModuleManager(ctx context.Context, vCenterClient *vim25.Client) *cluster.Manager {
	restClient, err := vcenter.NewRestClient(ctx, vCenterClient, testbed.AdminUsername, testbed.AdminPassword)
	Expect(err).ToNot(HaveOccurred(), "failed to create vCenter REST client")
	return cluster.NewManager(restClient)
}

// clusterModuleUUIDs returns the UUIDs of every cluster module on vCenter.
func clusterModuleUUIDs(ctx context.Context, m *cluster.Manager) ([]string, error) {
	modules, err := m.ListModules(ctx)
	if err != nil {
		return nil, err
	}

	uuids := make([]string, 0, len(modules))
	for _, module := range modules {
		uuids = append(uuids, module.Module)
	}
	return uuids, nil
}

// findChild returns the named child of parent, or nil when no such child exists.
func findChild(ctx context.Context, vimClient *vim25.Client, parent object.Reference, name string) (object.Reference, error) {
	return object.NewSearchIndex(vimClient).FindChild(ctx, parent, name)
}

// waitForClusterModuleUUIDs waits until the given cluster module UUIDs are
// absent from vCenter while the surviving ones are still present.
func waitForClusterModuleUUIDs(
	ctx context.Context,
	clusterModuleManager *cluster.Manager,
	deletedUUIDs, presentUUIDs []string,
	intervals ...any,
) {
	Eventually(func(g Gomega) {
		moduleUUIDs, err := clusterModuleUUIDs(ctx, clusterModuleManager)
		g.Expect(err).ToNot(HaveOccurred(), "failed to list vCenter cluster modules")

		for _, uuid := range deletedUUIDs {
			g.Expect(moduleUUIDs).ToNot(ContainElement(uuid),
				"cluster module %s for the decommissioned Zone still exists on vCenter", uuid)
		}
		for _, uuid := range presentUUIDs {
			g.Expect(moduleUUIDs).To(ContainElement(uuid),
				"cluster module %s for a surviving Zone is missing on vCenter", uuid)
		}
	}, intervals...).Should(Succeed())
}
