// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"fmt"
	"math/rand"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/vapi/cluster"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func getVirtualMachineSetResourcePolicy(name, namespace string) *vmopv1.VirtualMachineSetResourcePolicy {
	return &vmopv1.VirtualMachineSetResourcePolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-resourcepolicy", name),
			Namespace: namespace,
		},
		Spec: vmopv1.VirtualMachineSetResourcePolicySpec{
			ResourcePool: vmopv1.ResourcePoolSpec{
				Name:         fmt.Sprintf("%s-resourcepool", name),
				Reservations: vmopv1.VirtualMachineResourceSpec{},
				Limits:       vmopv1.VirtualMachineResourceSpec{},
			},
			Folder:              fmt.Sprintf("%s-folder", name),
			ClusterModuleGroups: []string{"ControlPlane", "NodeGroup1"},
		},
	}
}

var _ = Describe("VirtualMachineSetResourcePolicy Tests", func() {

	var (
		initObjects []client.Object
		ctx         *builder.TestContextForVCSim
		nsInfo      builder.WorkloadNamespaceInfo
		testConfig  builder.VCSimTestConfig
		vmProvider  providers.VirtualMachineProviderInterface
	)

	BeforeEach(func() {
		testConfig = builder.VCSimTestConfig{
			NumFaultDomains: 3,
		}
	})

	JustBeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(testConfig, initObjects...)
		vmProvider = vsphere.NewVSphereVMProviderFromClient(ctx, ctx.Client, ctx.Recorder)
		nsInfo = ctx.CreateWorkloadNamespace()
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		initObjects = nil
	})

	assertSetResourcePolicy := func(rp *vmopv1.VirtualMachineSetResourcePolicy, expectedExists bool) {
		if folderName := rp.Spec.Folder; folderName != "" {
			exists, err := vcenter.DoesChildFolderExist(ctx, ctx.VCClient.Client, nsInfo.Folder.Reference().Value, folderName)
			Expect(err).ToNot(HaveOccurred())
			Expect(exists).To(Equal(expectedExists))
		}

		if rpName := rp.Spec.ResourcePool.Name; rpName != "" {
			if expectedExists {
				expectedCnt := ctx.ClustersPerZone * ctx.ZoneCount
				Expect(rp.Status.ResourcePools).To(HaveLen(expectedCnt))
			}

			for _, zoneName := range ctx.ZoneNames {
				nsRP := ctx.GetResourcePoolForNamespace(rp.Namespace, zoneName, "")

				childRP, err := vcenter.GetChildResourcePool(ctx, nsRP, rpName)
				if expectedExists {
					Expect(err).ToNot(HaveOccurred())
					Expect(childRP).ToNot(BeNil())

					ccr, err := nsRP.Owner(ctx)
					Expect(err).ToNot(HaveOccurred())

					Expect(rp.Status.ResourcePools).To(ContainElement(vmopv1.ResourcePoolStatus{
						ClusterMoID:           ccr.Reference().Value,
						ChildResourcePoolMoID: childRP.Reference().Value,
					}))
				} else {
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("not found under parent ResourcePool"))
				}
			}
		} else {
			Expect(rp.Status.ResourcePools).To(BeEmpty())
		}

		clusterModules, err := cluster.NewManager(ctx.RestClient).ListModules(ctx)
		Expect(err).ToNot(HaveOccurred())

		if expectedExists {
			expectedCnt := len(rp.Spec.ClusterModuleGroups) * ctx.ClustersPerZone * ctx.ZoneCount
			Expect(rp.Status.ClusterModules).To(HaveLen(expectedCnt))
			Expect(clusterModules).To(HaveLen(expectedCnt))

			cmMap := map[string]struct{}{}
			cmUUID := map[string]struct{}{}

			for _, cmStatus := range rp.Status.ClusterModules {
				k := cmStatus.GroupName + "::" + cmStatus.ClusterMoID
				Expect(cmMap).ToNot(HaveKey(k))
				cmMap[k] = struct{}{}

				Expect(cmUUID).ToNot(HaveKey(cmStatus.ModuleUuid))
				cmUUID[cmStatus.ModuleUuid] = struct{}{}

				expectedSummary := cluster.ModuleSummary{
					Cluster: cmStatus.ClusterMoID,
					Module:  cmStatus.ModuleUuid,
				}
				Expect(clusterModules).To(ContainElement(expectedSummary))
			}

			// Check that each module was created for each CCR.
			for _, zoneName := range ctx.ZoneNames {
				ccrs := ctx.GetAZClusterComputes(zoneName)
				Expect(ccrs).ToNot(BeEmpty())
				for _, ccr := range ccrs {
					for _, cmName := range rp.Spec.ClusterModuleGroups {
						k := cmName + "::" + ccr.Reference().Value
						Expect(cmMap).To(HaveKey(k))
					}
				}
			}

		} else {
			Expect(rp.Status.ClusterModules).To(BeEmpty())
			Expect(clusterModules).To(BeEmpty())
		}
	}

	Context("Empty VirtualMachineSetResourcePolicy", func() {
		It("Creates and Deletes successfully", func() {
			resourcePolicy := &vmopv1.VirtualMachineSetResourcePolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-policy",
					Namespace: nsInfo.Namespace,
				},
			}

			By("Create", func() {
				Expect(vmProvider.CreateOrUpdateVirtualMachineSetResourcePolicy(ctx, resourcePolicy)).To(Succeed())
				assertSetResourcePolicy(resourcePolicy, true)
			})

			By("Delete", func() {
				Expect(vmProvider.DeleteVirtualMachineSetResourcePolicy(ctx, resourcePolicy)).To(Succeed())
				assertSetResourcePolicy(resourcePolicy, false)
			})
		})
	})

	Context("VirtualMachineSetResourcePolicy", func() {
		var (
			resourcePolicy *vmopv1.VirtualMachineSetResourcePolicy
		)

		JustBeforeEach(func() {
			resourcePolicy = getVirtualMachineSetResourcePolicy("test-policy", nsInfo.Namespace)
			Expect(vmProvider.CreateOrUpdateVirtualMachineSetResourcePolicy(ctx, resourcePolicy)).To(Succeed())
		})

		JustAfterEach(func() {
			Expect(vmProvider.DeleteVirtualMachineSetResourcePolicy(ctx, resourcePolicy)).To(Succeed())
			assertSetResourcePolicy(resourcePolicy, false)
		})

		It("creates expected resource policy", func() {
			assertSetResourcePolicy(resourcePolicy, true)
		})

		Context("for an existing resource policy", func() {
			It("should keep existing cluster modules", func() {
				assertSetResourcePolicy(resourcePolicy, true)
				status := resourcePolicy.Status.DeepCopy()

				Expect(vmProvider.CreateOrUpdateVirtualMachineSetResourcePolicy(ctx, resourcePolicy)).To(Succeed())
				Expect(resourcePolicy.Status.ClusterModules).To(HaveExactElements(status.ClusterModules))
				assertSetResourcePolicy(resourcePolicy, true)
			})
		})

		Context("for a resource policy with invalid cluster module", func() {
			It("successfully able to delete the resource policy", func() {
				assertSetResourcePolicy(resourcePolicy, true)

				resourcePolicy.Status.ClusterModules = append([]vmopv1.VSphereClusterModuleStatus{
					{
						GroupName:  "invalid-group",
						ModuleUuid: "invalid-uuid",
					},
				}, resourcePolicy.Status.ClusterModules...)
			})
		})

		It("should claim cluster module without ClusterMoID set", func() {
			Expect(resourcePolicy.Spec.ClusterModuleGroups).ToNot(BeEmpty())
			groupName := resourcePolicy.Spec.ClusterModuleGroups[0]

			moduleStatus := resourcePolicy.Status.DeepCopy()
			Expect(moduleStatus.ClusterModules).ToNot(BeEmpty())

			for i := range resourcePolicy.Status.ClusterModules {
				if resourcePolicy.Status.ClusterModules[i].GroupName == groupName {
					resourcePolicy.Status.ClusterModules[i].ClusterMoID = ""
				}
			}
			Expect(vmProvider.CreateOrUpdateVirtualMachineSetResourcePolicy(ctx, resourcePolicy)).To(Succeed())
			Expect(resourcePolicy.Status.ClusterModules).To(Equal(moduleStatus.ClusterModules))
			assertSetResourcePolicy(resourcePolicy, true)
		})

		It("prunes stale cluster module entry and leaves the still-valid ones untouched", func() {
			assertSetResourcePolicy(resourcePolicy, true)

			status := resourcePolicy.Status.DeepCopy()
			Expect(status.ClusterModules).ToNot(BeEmpty())

			resourcePolicy.Status.ClusterModules = append(resourcePolicy.Status.ClusterModules,
				vmopv1.VSphereClusterModuleStatus{
					GroupName:   "stale-group",
					ModuleUuid:  "bogus-module-uuid",
					ClusterMoID: "bogus-cluster-moid",
				})
			Expect(vmProvider.CreateOrUpdateVirtualMachineSetResourcePolicy(ctx, resourcePolicy)).To(Succeed())

			// Only the stale entry should be gone.
			Expect(resourcePolicy.Status.ClusterModules).To(HaveExactElements(status.ClusterModules))
		})
	})

	// This is intentionally not under the "VirtualMachineSetResourcePolicy" Context above
	// since its JustAfterEach() deletes the policy and then asserts that every zone's child
	// ResourcePool lookup fails. Once we remove a zone below, that zone's ResourcePool MoID
	// is no longer resolvable for this namespace, so DeleteVirtualMachineSetResourcePolicy()
	// can't (and doesn't need to) touch it, breaking that unrelated assertion. We don't delete
	// it since it will go away when the Namespace RP is deleted.
	Context("VirtualMachineSetResourcePolicy when a zone is removed", func() {
		It("entries for removed zone are not present in status", func() {
			resourcePolicy := getVirtualMachineSetResourcePolicy("test-policy-zone-removed", nsInfo.Namespace)
			Expect(vmProvider.CreateOrUpdateVirtualMachineSetResourcePolicy(ctx, resourcePolicy)).To(Succeed())
			assertSetResourcePolicy(resourcePolicy, true)

			Expect(ctx.ZoneNames).ToNot(BeEmpty())
			removedZoneName := ctx.ZoneNames[rand.Intn(len(ctx.ZoneNames))]
			removedCCRs := ctx.GetAZClusterComputes(removedZoneName)
			Expect(removedCCRs).ToNot(BeEmpty())
			removedClusterMoID := removedCCRs[0].Reference().Value

			var removedModuleUUIDs []string
			var survivingBefore []vmopv1.VSphereClusterModuleStatus
			for _, cm := range resourcePolicy.Status.ClusterModules {
				if cm.ClusterMoID == removedClusterMoID {
					removedModuleUUIDs = append(removedModuleUUIDs, cm.ModuleUuid)
				} else {
					survivingBefore = append(survivingBefore, cm)
				}
			}
			Expect(removedModuleUUIDs).To(HaveLen(len(resourcePolicy.Spec.ClusterModuleGroups)))

			var survivingResourcePoolsBefore []vmopv1.ResourcePoolStatus
			for _, rp := range resourcePolicy.Status.ResourcePools {
				if rp.ClusterMoID != removedClusterMoID {
					survivingResourcePoolsBefore = append(survivingResourcePoolsBefore, rp)
				}
			}
			Expect(survivingResourcePoolsBefore).To(HaveLen(len(resourcePolicy.Status.ResourcePools) - 1))

			// Simulate the zone being decommissioned by removing the namespace's Zone CR
			// for it, so it no longer contributes a  ResourcePool MoID for this namespace.
			zone := &topologyv1.Zone{
				ObjectMeta: metav1.ObjectMeta{
					Name:      removedZoneName,
					Namespace: nsInfo.Namespace,
				},
			}
			Expect(ctx.Client.Delete(ctx, zone)).To(Succeed())

			Expect(vmProvider.CreateOrUpdateVirtualMachineSetResourcePolicy(ctx, resourcePolicy)).To(Succeed())

			expectedRemainingCnt :=
				len(resourcePolicy.Spec.ClusterModuleGroups) * (ctx.ZoneCount - 1) * ctx.ClustersPerZone
			Expect(resourcePolicy.Status.ClusterModules).To(HaveLen(expectedRemainingCnt))
			Expect(resourcePolicy.Status.ClusterModules).To(HaveExactElements(survivingBefore))

			// The removed cluster's child ResourcePool is not actually deleted on VC -
			// we just stop tracking it - so its Status.ResourcePools entry must also
			// be gone, even though nothing changed on the VC side for it.
			Expect(resourcePolicy.Status.ResourcePools).To(HaveLen(ctx.ClustersPerZone * (ctx.ZoneCount - 1)))
			Expect(resourcePolicy.Status.ResourcePools).To(ConsistOf(survivingResourcePoolsBefore))

			clusterModules, err := cluster.NewManager(ctx.RestClient).ListModules(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(clusterModules).To(HaveLen(expectedRemainingCnt))
			for _, uuid := range removedModuleUUIDs {
				Expect(clusterModules).ToNot(ContainElement(HaveField("Module", uuid)))
			}
		})
	})
})
