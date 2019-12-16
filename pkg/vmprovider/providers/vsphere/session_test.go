// +build !integration

// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"context"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/simulator/vpx"
	"github.com/vmware/govmomi/vim25/types"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25"

	"github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
)

var _ = Describe("Test Session", func() {

	Context("ExtraConfig priority", func() {
		Specify("ExtraConfig map is correct with no global map", func() {
			vmConfig := map[string]string{"oneK": "oneV", "twoK": "twoV"}

			vmMeta := vsphere.GetExtraConfig(vmConfig)

			// Check that the VM extra config is returned in the correct format
			for _, option := range vmMeta {
				Expect(option.GetOptionValue().Value).Should(Equal(vmConfig[option.GetOptionValue().Key]))
			}
		})

		Specify("ExtraConfig map is correct with global map", func() {
			vmSpec := &v1alpha1.VirtualMachineSpec{ImageName: "photon-3-something"}

			vmConfig := map[string]string{"oneK": "oneV", "twoK": "twoV"}
			renderedVmConfig := vsphere.ApplyVmSpec(vmConfig, vmSpec)

			globalConfig := map[string]string{"twoK": "glob-twoV", "threeK": "glob-three-{{.ImageName}}-V"}
			renderedGlobalConfig := vsphere.ApplyVmSpec(globalConfig, vmSpec)

			mergedMeta := vsphere.MergeMeta(vmConfig, globalConfig)
			vmMeta := vsphere.GetExtraConfig(vsphere.ApplyVmSpec(mergedMeta, vmSpec))

			Expect(renderedGlobalConfig["threeK"]).To(ContainSubstring(vmSpec.ImageName))

			// Check that the VM extra config overrides the global config
			for _, option := range vmMeta {
				if _, ok := vmConfig[option.GetOptionValue().Key]; ok {
					Expect(option.GetOptionValue().Value).Should(Equal(renderedVmConfig[option.GetOptionValue().Key]))
				} else if _, ok := globalConfig[option.GetOptionValue().Key]; ok {
					Expect(option.GetOptionValue().Value).Should(Equal(renderedGlobalConfig[option.GetOptionValue().Key]))
				} else {
					Fail("Unrecognized extraConfig option")
				}
			}
		})
	})

	Context("RP as inventory path", func() {
		Specify("returns RP object without error", func() {
			res := simulator.VPX().Run(func(ctx context.Context, c *vim25.Client) error {
				finder := find.NewFinder(c)
				pools, err := finder.ResourcePoolList(ctx, "*")
				Expect(err).ShouldNot(HaveOccurred())
				Expect(len(pools)).ToNot(BeZero())

				paths := []string{
					pools[0].InventoryPath,
					pools[0].Reference().Value,
				}

				for _, path := range paths {
					pool, err := vsphere.GetResourcePool(ctx, finder, path)
					Expect(err).ShouldNot(HaveOccurred())
					Expect(pool.InventoryPath).To(Equal(pools[0].InventoryPath))
				}
				return nil
			})
			Expect(res).To(BeNil())
		})
	})

	Context("Folder as inventory path", func() {
		Specify("returns Folder object without error", func() {
			res := simulator.VPX().Run(func(ctx context.Context, c *vim25.Client) error {
				finder := find.NewFinder(c)
				folders, err := finder.FolderList(ctx, "*")
				Expect(err).ShouldNot(HaveOccurred())
				Expect(len(folders)).ToNot(BeZero())

				paths := []string{
					folders[0].InventoryPath,
					folders[0].Reference().Value,
				}

				for _, path := range paths {
					folder, err := vsphere.GetVMFolder(ctx, finder, path)
					Expect(err).ShouldNot(HaveOccurred())
					Expect(folder.InventoryPath).To(Equal(folders[0].InventoryPath))
				}

				return nil
			})
			Expect(res).To(BeNil())
		})
	})

	Context("Convert CPU units from milli-cores to MHz", func() {
		Specify("return whole number for non-integer CPU quantity", func() {
			q, err := resource.ParseQuantity("500m")
			Expect(err).NotTo(HaveOccurred())
			freq := vsphere.CpuQuantityToMhz(q, 3225)
			expectVal := int64(1613)
			Expect(freq).Should(BeNumerically("==", expectVal))
		})

		Specify("return whole number for integer CPU quantity", func() {
			q, err := resource.ParseQuantity("1000m")
			Expect(err).NotTo(HaveOccurred())
			freq := vsphere.CpuQuantityToMhz(q, 3225)
			expectVal := int64(3225)
			Expect(freq).Should(BeNumerically("==", expectVal))
		})
	})

	Context("Compute CPU Min Frequency in the Cluster", func() {
		Specify("return cpu min frequency when natural number of hosts attached the cluster", func() {
			res := simulator.VPX().Run(func(ctx context.Context, c *vim25.Client) error {
				find := find.NewFinder(c)
				cr, err := find.DefaultClusterComputeResource(ctx)
				Expect(err).ShouldNot(HaveOccurred())

				cpuMinFreq, err := vsphere.ComputeCPUInfo(ctx, cr)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(cpuMinFreq).Should(BeNumerically(">", 0))

				return nil
			})
			Expect(res).To(BeNil())
		})

		Specify("return cpu min frequency when the cluster contains no hosts", func() {
			content := vpx.ServiceContent
			s := simulator.New(simulator.NewServiceInstance(content, vpx.RootFolder))

			ts := s.NewServer()
			defer ts.Close()

			ctx := context.Background()
			c, err := govmomi.NewClient(ctx, ts.URL, true)
			Expect(err).NotTo(HaveOccurred())

			f := object.NewRootFolder(c.Client)

			dc, err := f.CreateDatacenter(ctx, "foo")
			Expect(err).NotTo(HaveOccurred())

			folders, err := dc.Folders(ctx)
			Expect(err).NotTo(HaveOccurred())

			cluster, err := folders.HostFolder.CreateCluster(ctx, "cluster1", types.ClusterConfigSpecEx{})
			Expect(err).NotTo(HaveOccurred())

			cpuMinFreq, err := vsphere.ComputeCPUInfo(ctx, cluster)
			Expect(err.Error()).Should(Equal("No hosts found in the cluster"))
			Expect(cpuMinFreq).Should(BeNumerically("==", 0))
		})
	})
})
