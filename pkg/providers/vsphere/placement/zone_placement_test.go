// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package placement_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/vim25/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/placement"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("MakePlacementDecision", func() {

	Context("only one placement decision is possible", func() {
		It("makes expected decision", func() {
			recommendations := map[string][]placement.Recommendation{
				"zone1": {
					placement.Recommendation{
						PoolMoRef: types.ManagedObjectReference{Type: "a", Value: "abc"},
						HostMoRef: &types.ManagedObjectReference{Type: "b", Value: "xyz"},
					},
				},
			}

			zoneName, rec := placement.MakePlacementDecision(recommendations)
			Expect(zoneName).To(Equal("zone1"))
			Expect(rec).To(BeElementOf(recommendations[zoneName]))
		})
	})

	Context("multiple placement candidates exist", func() {
		It("makes an decision", func() {
			zones := map[string][]string{
				"zone1": {"z1-host1", "z1-host2", "z1-host3"},
				"zone2": {"z2-host1", "z2-host2"},
			}

			recommendations := map[string][]placement.Recommendation{}
			for zoneName, hosts := range zones {
				for _, host := range hosts {
					recommendations[zoneName] = append(recommendations[zoneName],
						placement.Recommendation{
							PoolMoRef: types.ManagedObjectReference{Type: "a", Value: "abc"},
							HostMoRef: &types.ManagedObjectReference{Type: "b", Value: host},
						})
				}
			}

			zoneName, rec := placement.MakePlacementDecision(recommendations)
			Expect(zones).To(HaveKey(zoneName))
			Expect(rec).To(BeElementOf(recommendations[zoneName]))
		})
	})
})

func vcSimPlacement() {

	var (
		initObjects []client.Object
		ctx         *builder.TestContextForVCSim
		nsInfo      builder.WorkloadNamespaceInfo
		testConfig  builder.VCSimTestConfig

		vm         *vmopv1.VirtualMachine
		vmCtx      context.VirtualMachineContext
		configSpec types.VirtualMachineConfigSpec
	)

	BeforeEach(func() {
		testConfig = builder.VCSimTestConfig{}

		vm = builder.DummyVirtualMachineA2()
		vm.Name = "placement-test"

		// Other than the name ConfigSpec contents don't matter for vcsim.
		configSpec = types.VirtualMachineConfigSpec{
			Name: vm.Name,
		}
	})

	JustBeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(testConfig, initObjects...)
		nsInfo = ctx.CreateWorkloadNamespace()

		vm.Namespace = nsInfo.Namespace

		vmCtx = context.VirtualMachineContext{
			Context: ctx,
			Logger:  suite.GetLogger().WithValues("vmName", vm.Name),
			VM:      vm,
		}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		initObjects = nil
	})

	Context("Zone placement", func() {

		Context("zone already assigned", func() {
			zoneName := "in the zone"

			JustBeforeEach(func() {
				vm.Labels[topology.KubernetesTopologyZoneLabelKey] = zoneName
			})

			It("returns success with same zone", func() {
				result, err := placement.Placement(vmCtx, ctx.Client, ctx.VCClient.Client, configSpec, "")
				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
				Expect(result.ZonePlacement).To(BeTrue())
				Expect(result.ZoneName).To(Equal(zoneName))
				Expect(result.HostMoRef).To(BeNil())
				Expect(vm.Labels).To(HaveKeyWithValue(topology.KubernetesTopologyZoneLabelKey, zoneName))

				// Current contract is the caller must look this up based on the pre-assigned zone but
				// we might want to change that later.
				Expect(result.PoolMoRef.Value).To(BeEmpty())
			})
		})

		Context("no placement candidates", func() {
			JustBeforeEach(func() {
				vm.Namespace = "does-not-exist"
			})

			It("returns an error", func() {
				result, err := placement.Placement(vmCtx, ctx.Client, ctx.VCClient.Client, configSpec, "")
				Expect(err).To(MatchError("no placement candidates available"))
				Expect(result).To(BeNil())
			})
		})

		It("returns success", func() {
			result, err := placement.Placement(vmCtx, ctx.Client, ctx.VCClient.Client, configSpec, "")
			Expect(err).ToNot(HaveOccurred())

			Expect(result.ZonePlacement).To(BeTrue())
			Expect(result.ZoneName).To(BeElementOf(ctx.ZoneNames))
			Expect(result.PoolMoRef.Value).ToNot(BeEmpty())
			Expect(result.HostMoRef).To(BeNil())

			nsRP := ctx.GetResourcePoolForNamespace(vm.Namespace, result.ZoneName, "")
			Expect(nsRP).ToNot(BeNil())
			Expect(result.PoolMoRef.Value).To(Equal(nsRP.Reference().Value))
		})

		Context("Only one zone exists", func() {
			BeforeEach(func() {
				testConfig.NumFaultDomains = 1
			})

			It("returns success", func() {
				result, err := placement.Placement(vmCtx, ctx.Client, ctx.VCClient.Client, configSpec, "")
				Expect(err).ToNot(HaveOccurred())

				Expect(result.ZonePlacement).To(BeTrue())
				Expect(result.ZoneName).To(BeElementOf(ctx.ZoneNames))
				Expect(result.PoolMoRef.Value).ToNot(BeEmpty())
				Expect(result.HostMoRef).To(BeNil())

				nsRP := ctx.GetResourcePoolForNamespace(vm.Namespace, result.ZoneName, "")
				Expect(nsRP).ToNot(BeNil())
				Expect(result.PoolMoRef.Value).To(Equal(nsRP.Reference().Value))
			})
		})

		Context("VM is in child RP via ResourcePolicy", func() {
			It("returns success", func() {
				resourcePolicy, _ := ctx.CreateVirtualMachineSetResourcePolicyA2("my-child-rp", nsInfo)
				Expect(resourcePolicy).ToNot(BeNil())
				childRPName := resourcePolicy.Spec.ResourcePool.Name
				Expect(childRPName).ToNot(BeEmpty())
				vmCtx.VM.Spec.Reserved = &vmopv1.VirtualMachineReservedSpec{
					ResourcePolicyName: resourcePolicy.Name,
				}

				result, err := placement.Placement(vmCtx, ctx.Client, ctx.VCClient.Client, configSpec, childRPName)
				Expect(err).ToNot(HaveOccurred())

				Expect(result.ZonePlacement).To(BeTrue())
				Expect(result.ZoneName).To(BeElementOf(ctx.ZoneNames))

				childRP := ctx.GetResourcePoolForNamespace(vm.Namespace, result.ZoneName, childRPName)
				Expect(childRP).ToNot(BeNil())
				Expect(result.PoolMoRef.Value).To(Equal(childRP.Reference().Value))
			})
		})
	})

	Context("Instance Storage Placement", func() {

		BeforeEach(func() {
			testConfig.WithInstanceStorage = true
			builder.AddDummyInstanceStorageVolumeA2(vm)
			testConfig.NumFaultDomains = 1 // Only support for non-HA "HA"
		})

		When("host already assigned", func() {
			const hostMoID = "foobar-host-42"

			BeforeEach(func() {
				vm.Labels[topology.KubernetesTopologyZoneLabelKey] = "my-zone"
				vm.Annotations[constants.InstanceStorageSelectedNodeMOIDAnnotationKey] = hostMoID
			})

			It("returns success with same host", func() {
				result, err := placement.Placement(vmCtx, ctx.Client, ctx.VCClient.Client, configSpec, "")
				Expect(err).ToNot(HaveOccurred())

				Expect(result.InstanceStoragePlacement).To(BeTrue())
				Expect(result.HostMoRef).ToNot(BeNil())
				Expect(result.HostMoRef.Value).To(Equal(hostMoID))
			})
		})

		It("returns success", func() {
			result, err := placement.Placement(vmCtx, ctx.Client, ctx.VCClient.Client, configSpec, "")
			Expect(err).ToNot(HaveOccurred())

			Expect(result.ZonePlacement).To(BeTrue())
			Expect(result.ZoneName).ToNot(BeEmpty())

			Expect(result.InstanceStoragePlacement).To(BeTrue())
			Expect(result.HostMoRef).ToNot(BeNil())
			Expect(result.HostMoRef.Value).ToNot(BeEmpty())

			nsRP := ctx.GetResourcePoolForNamespace(vm.Namespace, result.ZoneName, "")
			Expect(nsRP).ToNot(BeNil())
			Expect(result.PoolMoRef.Value).To(Equal(nsRP.Reference().Value))
		})

		Context("VM is in child RP via ResourcePolicy", func() {
			It("returns success", func() {
				resourcePolicy, _ := ctx.CreateVirtualMachineSetResourcePolicyA2("my-child-rp", nsInfo)
				Expect(resourcePolicy).ToNot(BeNil())
				childRPName := resourcePolicy.Spec.ResourcePool.Name
				Expect(childRPName).ToNot(BeEmpty())
				vmCtx.VM.Spec.Reserved = &vmopv1.VirtualMachineReservedSpec{
					ResourcePolicyName: resourcePolicy.Name,
				}

				result, err := placement.Placement(vmCtx, ctx.Client, ctx.VCClient.Client, configSpec, childRPName)
				Expect(err).ToNot(HaveOccurred())

				Expect(result.ZonePlacement).To(BeTrue())
				Expect(result.ZoneName).To(BeElementOf(ctx.ZoneNames))

				Expect(result.InstanceStoragePlacement).To(BeTrue())
				Expect(result.HostMoRef).ToNot(BeNil())
				Expect(result.HostMoRef.Value).ToNot(BeEmpty())

				childRP := ctx.GetResourcePoolForNamespace(vm.Namespace, result.ZoneName, childRPName)
				Expect(childRP).ToNot(BeNil())
				Expect(result.PoolMoRef.Value).To(Equal(childRP.Reference().Value))
			})
		})
	})
}
