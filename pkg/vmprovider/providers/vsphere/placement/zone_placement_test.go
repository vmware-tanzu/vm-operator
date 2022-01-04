// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package placement_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	vimtypes "github.com/vmware/govmomi/vim25/types"
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
			recommendations := map[string][]vimtypes.ManagedObjectReference{
				"zone1": {
					{Type: "whatever", Value: "host123"},
				},
			}

			zoneName, hostMoID := placement.MakePlacementDecision(recommendations)
			Expect(zoneName).To(Equal("zone1"))
			Expect(hostMoID).To(Equal("host123"))
		})
	})

	Context("multiple placement candidates exist", func() {
		zones := map[string][]string{
			"zone1": {"z1-host1", "z1-host2", "z1-host3"},
			"zone2": {"z2-host1", "z2-host2"},
		}

		recommendations := map[string][]vimtypes.ManagedObjectReference{}
		for zoneName, hosts := range zones {
			for _, host := range hosts {
				recommendations[zoneName] = append(recommendations[zoneName],
					vimtypes.ManagedObjectReference{Type: "whatever", Value: host})
			}
		}

		zoneName, hostMoID := placement.MakePlacementDecision(recommendations)
		Expect(zones).To(HaveKey(zoneName))
		Expect(hostMoID).To(BeElementOf(zones[zoneName]))
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
		configSpec *vimtypes.VirtualMachineConfigSpec
	)

	BeforeEach(func() {
		testConfig = builder.VCSimTestConfig{}

		vm = builder.DummyVirtualMachine()
		vm.Name = "placement-test"

		// ConfigSpec contents don't matter for vcsim.
		configSpec = &vimtypes.VirtualMachineConfigSpec{}
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
				err := placement.Placement(vmCtx, ctx.Client, ctx.VCClient.Client, configSpec)
				Expect(err).ToNot(HaveOccurred())

				Expect(vm.Labels).To(HaveKeyWithValue(topology.KubernetesTopologyZoneLabelKey, zoneName))
			})
		})

		Context("no placement candidates", func() {
			JustBeforeEach(func() {
				vm.Namespace = "does-not-exist"
			})

			It("returns an error", func() {
				err := placement.Placement(vmCtx, ctx.Client, ctx.VCClient.Client, configSpec)
				Expect(err).To(MatchError("no placement candidates available"))
				Expect(vm.Labels).ToNot(HaveKey(topology.KubernetesTopologyZoneLabelKey))
			})
		})

		It("returns success", func() {
			err := placement.Placement(vmCtx, ctx.Client, ctx.VCClient.Client, configSpec)
			Expect(err).ToNot(HaveOccurred())

			zone, ok := vm.Labels[topology.KubernetesTopologyZoneLabelKey]
			Expect(ok).To(BeTrue())
			Expect(zone).To(BeElementOf(ctx.ZoneNames))
		})
	})

	Context("Instance Storage Placement", func() {

		BeforeEach(func() {
			testConfig.WithInstanceStorage = true
			builder.AddDummyInstanceStorageVolume(vm)
		})

		It("returns success", func() {
			err := placement.Placement(vmCtx, ctx.Client, ctx.VCClient.Client, configSpec)
			Expect(err).ToNot(HaveOccurred())

			hostMoID, ok := vm.Annotations[constants.InstanceStorageSelectedNodeMOIDAnnotationKey]
			Expect(ok).To(BeTrue())
			hostname, ok := vm.Annotations[constants.InstanceStorageSelectedNodeAnnotationKey]
			Expect(ok).To(BeTrue())
			Expect(hostname).To(HavePrefix(hostMoID))
		})
	})
}
