// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"fmt"
	"path"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/object"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
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
			Folder: vmopv1.FolderSpec{
				Name: fmt.Sprintf("%s-folder", name),
			},
			ClusterModules: []vmopv1.ClusterModuleSpec{
				{
					GroupName: "ControlPlane",
				},
				{
					GroupName: "NodeGroup1",
				},
			},
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
			vmProvider  vmprovider.VirtualMachineProviderInterface
		)

		BeforeEach(func() {
			testConfig = builder.VCSimTestConfig{}
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

		Context("VirtualMachineSetResourcePolicy", func() {
			var (
				resourcePolicy *vmopv1.VirtualMachineSetResourcePolicy
			)

			JustBeforeEach(func() {
				testPolicyName := "test-policy"

				resourcePolicy = getVirtualMachineSetResourcePolicy(testPolicyName, nsInfo.Namespace)
				Expect(vmProvider.CreateOrUpdateVirtualMachineSetResourcePolicy(ctx, resourcePolicy)).To(Succeed())
			})

			JustAfterEach(func() {
				Expect(vmProvider.DeleteVirtualMachineSetResourcePolicy(ctx, resourcePolicy)).To(Succeed())
				Expect(resourcePolicy.Status.ClusterModules).Should(BeEmpty())
			})

			It("creates expected cluster modules", func() {
				modules := resourcePolicy.Status.ClusterModules
				Expect(modules).Should(HaveLen(2))
				module := modules[0]
				Expect(module.GroupName).To(Equal(resourcePolicy.Spec.ClusterModules[0].GroupName))
				Expect(module.ModuleUuid).ToNot(BeEmpty())
				module = modules[1]
				Expect(module.GroupName).To(Equal(resourcePolicy.Spec.ClusterModules[1].GroupName))
				Expect(module.ModuleUuid).ToNot(BeEmpty())
			})

			Context("for an existing resource policy", func() {
				It("should keep existing cluster modules", func() {
					status := resourcePolicy.Status.DeepCopy()

					Expect(vmProvider.CreateOrUpdateVirtualMachineSetResourcePolicy(ctx, resourcePolicy)).To(Succeed())
					Expect(resourcePolicy.Status.ClusterModules).To(ContainElements(status.ClusterModules))
				})

				It("successfully able to find the resource policy", func() {
					exists, err := vmProvider.IsVirtualMachineSetResourcePolicyReady(ctx, "", resourcePolicy)
					Expect(err).NotTo(HaveOccurred())
					Expect(exists).To(BeTrue())
				})
			})

			Context("for an absent resource policy", func() {
				It("should fail to find the resource policy without any errors", func() {
					failResPolicy := getVirtualMachineSetResourcePolicy("test-policy", nsInfo.Namespace)
					exists, err := vmProvider.IsVirtualMachineSetResourcePolicyReady(ctx, "", failResPolicy)
					Expect(err).NotTo(HaveOccurred())
					Expect(exists).To(BeFalse())
				})
			})

			Context("for a resource policy with invalid cluster module", func() {
				It("successfully able to delete the resource policy", func() {
					resourcePolicy.Status.ClusterModules = append([]vmopv1.ClusterModuleStatus{
						{
							GroupName:  "invalid-group",
							ModuleUuid: "invalid-uuid",
						},
					}, resourcePolicy.Status.ClusterModules...)
				})
			})

			It("creates expected resource pool", func() {
				rp, err := ctx.GetSingleClusterCompute().ResourcePool(ctx)
				Expect(err).ToNot(HaveOccurred())

				// Make trip through the Finder to populate InventoryPath.
				objRef, err := ctx.Finder.ObjectReference(ctx, rp.Reference())
				Expect(err).ToNot(HaveOccurred())
				rp, ok := objRef.(*object.ResourcePool)
				Expect(ok).To(BeTrue())

				inventoryPath := path.Join(rp.InventoryPath, nsInfo.Namespace, resourcePolicy.Spec.ResourcePool.Name)
				_, err = ctx.Finder.ResourcePool(ctx, inventoryPath)
				Expect(err).ToNot(HaveOccurred())
			})

			It("creates expected child folder", func() {
				_, err := ctx.Finder.Folder(ctx, path.Join(nsInfo.Folder.InventoryPath, resourcePolicy.Spec.Folder.Name))
				Expect(err).ToNot(HaveOccurred())
			})

			Context("when HA is enabled", func() {
				BeforeEach(func() {
					testConfig.WithFaultDomains = true
				})

				It("creates expected cluster modules for each cluster", func() {
					moduleCount := len(resourcePolicy.Spec.ClusterModules)
					Expect(moduleCount).To(Equal(2))

					modules := resourcePolicy.Status.ClusterModules
					zoneNames := ctx.ZoneNames
					Expect(modules).To(HaveLen(len(zoneNames) * ctx.ClustersPerZone * moduleCount))

					for zoneIdx, zoneName := range zoneNames {
						// NOTE: This assumes some ordering but is the easiest way to test.
						moduleIdx := zoneIdx * ctx.ClustersPerZone * moduleCount
						modules := modules[moduleIdx : moduleIdx+ctx.ClustersPerZone*moduleCount]

						ccrs := ctx.GetAZClusterComputes(zoneName)
						Expect(ccrs).To(HaveLen(ctx.ClustersPerZone))

						for _, cluster := range ccrs {
							clusterMoID := cluster.Reference().Value

							module := modules[0]
							Expect(module.GroupName).To(Equal(resourcePolicy.Spec.ClusterModules[0].GroupName))
							Expect(module.ModuleUuid).ToNot(BeEmpty())
							Expect(module.ClusterMoID).To(Equal(clusterMoID))

							module = modules[1]
							Expect(module.GroupName).To(Equal(resourcePolicy.Spec.ClusterModules[1].GroupName))
							Expect(module.ModuleUuid).ToNot(BeEmpty())
							Expect(module.ClusterMoID).To(Equal(clusterMoID))
						}
					}
				})

				It("should claim cluster module without ClusterMoID set", func() {
					Expect(resourcePolicy.Spec.ClusterModules).ToNot(BeEmpty())
					groupName := resourcePolicy.Spec.ClusterModules[0].GroupName

					moduleStatus := resourcePolicy.Status.DeepCopy()
					Expect(moduleStatus.ClusterModules).ToNot(BeEmpty())

					for i := range resourcePolicy.Status.ClusterModules {
						if resourcePolicy.Status.ClusterModules[i].GroupName == groupName {
							resourcePolicy.Status.ClusterModules[i].ClusterMoID = ""
						}
					}
					Expect(vmProvider.CreateOrUpdateVirtualMachineSetResourcePolicy(ctx, resourcePolicy)).To(Succeed())
					Expect(resourcePolicy.Status.ClusterModules).To(Equal(moduleStatus.ClusterModules))
				})

				It("successfully able to find the resource policy in each zone", func() {
					for _, zoneName := range ctx.ZoneNames {
						exists, err := vmProvider.IsVirtualMachineSetResourcePolicyReady(ctx, zoneName, resourcePolicy)
						Expect(err).NotTo(HaveOccurred())
						Expect(exists).To(BeTrue())
					}
				})

				It("creates expected resource pool for each cluster", func() {
					for _, zoneName := range ctx.ZoneNames {
						for _, cluster := range ctx.GetAZClusterComputes(zoneName) {
							rp, err := cluster.ResourcePool(ctx)
							Expect(err).ToNot(HaveOccurred())

							// Make trip through the Finder to populate InventoryPath.
							objRef, err := ctx.Finder.ObjectReference(ctx, rp.Reference())
							Expect(err).ToNot(HaveOccurred())
							rp, ok := objRef.(*object.ResourcePool)
							Expect(ok).To(BeTrue())

							inventoryPath := path.Join(rp.InventoryPath, nsInfo.Namespace, resourcePolicy.Spec.ResourcePool.Name)
							_, err = ctx.Finder.ResourcePool(ctx, inventoryPath)
							Expect(err).ToNot(HaveOccurred())
						}
					}
				})
			})
		})
	})
}
