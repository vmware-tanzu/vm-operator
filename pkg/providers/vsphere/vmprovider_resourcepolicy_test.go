// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/vapi/cluster"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
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

func resourcePolicyTests() {
	Describe("VirtualMachineSetResourcePolicy Tests", func() {

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
		})
	})
}
