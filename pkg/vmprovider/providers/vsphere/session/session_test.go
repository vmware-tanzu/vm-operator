// +build !integration

// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package session_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/session"
)

var _ = Describe("Test Session", func() {

	Context("Convert CPU units from milli-cores to MHz", func() {
		Specify("return whole number for non-integer CPU quantity", func() {
			q, err := resource.ParseQuantity("500m")
			Expect(err).NotTo(HaveOccurred())
			freq := session.CPUQuantityToMhz(q, 3225)
			expectVal := int64(1613)
			Expect(freq).Should(BeNumerically("==", expectVal))
		})

		Specify("return whole number for integer CPU quantity", func() {
			q, err := resource.ParseQuantity("1000m")
			Expect(err).NotTo(HaveOccurred())
			freq := session.CPUQuantityToMhz(q, 3225)
			expectVal := int64(3225)
			Expect(freq).Should(BeNumerically("==", expectVal))
		})
	})

	Context("Compute CPU Min Frequency in the Cluster", func() {
		Specify("return cpu min frequency when natural number of hosts attached the cluster", func() {
			// The default model used by simulator has one host and one cluster configured.
			res := simulator.VPX().Run(func(ctx context.Context, c *vim25.Client) error {
				find := find.NewFinder(c)
				cr, err := find.DefaultClusterComputeResource(ctx)
				Expect(err).ShouldNot(HaveOccurred())

				cpuMinFreq, err := session.ComputeCPUInfo(ctx, cr)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(cpuMinFreq).Should(BeNumerically(">", 0))

				return nil
			})
			Expect(res).To(BeNil())
		})

		Specify("return cpu min frequency when the cluster contains no hosts", func() {
			// The model being used is configured to have 0 hosts. (Defined in vsphere_suite_test.go/BeforeSuite)
			c, _ := govmomi.NewClient(ctx, server.URL, true)
			si := object.NewSearchIndex(c.Client)
			ref, err := si.FindByInventoryPath(ctx, "/DC0/host/DC0_C0")
			Expect(err).NotTo(HaveOccurred())
			cr := object.NewClusterComputeResource(c.Client, ref.Reference())

			cpuMinFreq, err := session.ComputeCPUInfo(ctx, cr)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(cpuMinFreq).Should(BeNumerically(">", 0))
		})
	})
})
