// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package placement_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/vim25/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/placement"
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

		vm         *vmopv1alpha1.VirtualMachine
		vmCtx      context.VirtualMachineContext
		configSpec *types.VirtualMachineConfigSpec
	)

	BeforeEach(func() {
		testConfig = builder.VCSimTestConfig{}

		vm = builder.DummyVirtualMachine()
		vm.Name = "placement-test"

		// Other than the name ConfigSpec contents don't matter for vcsim.
		configSpec = &types.VirtualMachineConfigSpec{
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
		BeforeEach(func() {
			testConfig.WithFaultDomains = true
		})

		Context("zone already assigned", func() {
			zoneName := "in the zone"

			JustBeforeEach(func() {
				vm.Labels[topology.KubernetesTopologyZoneLabelKey] = zoneName
			})

			It("returns success without changing zone", func() {
				err := placement.Placement(vmCtx, ctx.Client, ctx.VCClient.Client, configSpec, "")
				Expect(err).ToNot(HaveOccurred())

				Expect(vm.Labels).To(HaveKeyWithValue(topology.KubernetesTopologyZoneLabelKey, zoneName))
			})
		})

		Context("no placement candidates", func() {
			JustBeforeEach(func() {
				vm.Namespace = "does-not-exist"
			})

			It("returns an error", func() {
				err := placement.Placement(vmCtx, ctx.Client, ctx.VCClient.Client, configSpec, "")
				Expect(err).To(MatchError("no placement candidates available"))
				Expect(vm.Labels).ToNot(HaveKey(topology.KubernetesTopologyZoneLabelKey))
			})
		})

		It("returns success", func() {
			err := placement.Placement(vmCtx, ctx.Client, ctx.VCClient.Client, configSpec, "")
			Expect(err).ToNot(HaveOccurred())

			zone, ok := vm.Labels[topology.KubernetesTopologyZoneLabelKey]
			Expect(ok).To(BeTrue())
			Expect(zone).To(BeElementOf(ctx.ZoneNames))
		})

		Context("Only one zone exists", func() {
			BeforeEach(func() {
				testConfig.NumFaultDomains = 1
			})

			It("returns success", func() {
				err := placement.Placement(vmCtx, ctx.Client, ctx.VCClient.Client, configSpec, "")
				Expect(err).ToNot(HaveOccurred())

				zone, ok := vm.Labels[topology.KubernetesTopologyZoneLabelKey]
				Expect(ok).To(BeTrue())
				Expect(zone).To(BeElementOf(ctx.ZoneNames))
			})

		})

		Context("VM is in child RP via ResourcePolicy", func() {
			It("returns success", func() {
				resourcePolicy, _ := ctx.CreateVirtualMachineSetResourcePolicy("my-child-rp", nsInfo)
				Expect(resourcePolicy).ToNot(BeNil())
				childRPName := resourcePolicy.Spec.ResourcePool.Name
				Expect(childRPName).ToNot(BeEmpty())
				vmCtx.VM.Spec.ResourcePolicyName = resourcePolicy.Name

				err := placement.Placement(vmCtx, ctx.Client, ctx.VCClient.Client, configSpec, childRPName)
				Expect(err).ToNot(HaveOccurred())

				zone, ok := vm.Labels[topology.KubernetesTopologyZoneLabelKey]
				Expect(ok).To(BeTrue())
				Expect(zone).To(BeElementOf(ctx.ZoneNames))
			})
		})
	})

	Context("Instance Storage Placement", func() {

		BeforeEach(func() {
			testConfig.WithInstanceStorage = true
			builder.AddDummyInstanceStorageVolume(vm)
		})

		It("returns success", func() {
			err := placement.Placement(vmCtx, ctx.Client, ctx.VCClient.Client, configSpec, "")
			Expect(err).ToNot(HaveOccurred())

			hostMoID, ok := vm.Annotations[constants.InstanceStorageSelectedNodeMOIDAnnotationKey]
			Expect(ok).To(BeTrue())
			hostname, ok := vm.Annotations[constants.InstanceStorageSelectedNodeAnnotationKey]
			Expect(ok).To(BeTrue())
			Expect(hostname).To(HavePrefix(hostMoID))
		})
	})
}
