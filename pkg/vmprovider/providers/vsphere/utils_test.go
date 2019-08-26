// +build integration

/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/
package vsphere_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
	Vs "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
)

type fakeRoundTripper struct {
	err error
}

func (f *fakeRoundTripper) RoundTrip(ctx context.Context, req, res soap.HasFault) error {
	return f.err
}

type testSetupData struct {
	ctx    context.Context
	client *govmomi.Client
	cls    *object.ClusterComputeResource
	rpBad  *object.ResourcePool
	rpGood *object.ResourcePool
}

func testSetup(ctx context.Context, c *govmomi.Client) (*testSetupData, error) {
	tsd := testSetupData{ctx: ctx, client: c}
	finder := find.NewFinder(c.Client, false)
	dc, err := finder.Datacenter(ctx, "DC0")
	if err != nil {
		fmt.Printf("Failed to find datacenter: %v\n", err)
		return nil, err
	}
	finder.SetDatacenter(dc)
	//Add resource pool to standalone host
	host, err := finder.HostSystem(ctx, "DC0_H0")
	if err != nil {
		fmt.Printf("Failed to find standalone host: %v\n", err)
		return nil, err
	}
	drp, err := host.ResourcePool(ctx)
	if err != nil {
		fmt.Printf("Failed to get host's resource pool: %v\n", err)
		return nil, err
	}
	rp, err := drp.Create(ctx, "DC0_H0_RP0", types.DefaultResourceConfigSpec())
	if err != nil {
		fmt.Printf("Failed to create resource pool: %v\n", err)
		return nil, err
	}
	tsd.rpBad = rp
	//Add child resource pool to cluster's resource pool
	cls, err := finder.ClusterComputeResource(ctx, "DC0_C0")
	if err != nil {
		fmt.Printf("Failed to find ClusterComputeResource: %v\n", err)
		return nil, err
	}
	tsd.cls = cls
	drp, err = cls.ResourcePool(ctx)
	if err != nil {
		fmt.Printf("Failed to get cluster's resource pool: %v\n", err)
		return nil, err
	}
	rp, err = drp.Create(ctx, "DC0_C0_RP0_1", types.DefaultResourceConfigSpec())
	if err != nil {
		fmt.Printf("Failed to create child resource pool: %v\n", err)
		return nil, err
	}
	tsd.rpGood = rp
	return &tsd, nil
}

var _ = Describe("GetResourcePoolOwner", func() {
	Context("when vim client is valid", func() {
		var tsd *testSetupData

		BeforeEach(func() {
			ctx := context.TODO()
			gClient, err := vcSim.NewClient(ctx)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(gClient).NotTo(BeNil())
			tsd, err = testSetup(ctx, gClient)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(tsd).NotTo(BeNil())
		})
		AfterEach(func() {
			task, err := tsd.rpBad.Destroy(tsd.ctx)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(task).NotTo(BeNil())
			err = task.Wait(tsd.ctx)
			Expect(err).To(BeNil())
			task, err = tsd.rpGood.Destroy(tsd.ctx)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(task).NotTo(BeNil())
			err = task.Wait(tsd.ctx)
			Expect(err).ShouldNot(HaveOccurred())
			err = tsd.client.Logout(tsd.ctx)
			Expect(err).ShouldNot(HaveOccurred())
		})
		Context("when vim client is valid", func() {
			Specify("Resource pool has cluster parent", func() {
				cls, err := Vs.GetResourcePoolOwner(tsd.ctx, tsd.rpGood)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(cls).NotTo(BeNil())
				Expect(cls.Reference().Type).To(Equal("ClusterComputeResource"))
				Expect(cls.Reference().Value).To(Equal(tsd.cls.Reference().Value))
			})
			Specify("Resource pool does not have cluster parent", func() {
				cls, err := Vs.GetResourcePoolOwner(tsd.ctx, tsd.rpBad)
				Expect(err).Should(HaveOccurred())
				Expect(cls).To(BeNil())
			})
		})
	})
	Context("when vim client is not valid", func() {
		Specify("Using fake vim client", func() {
			var rtp soap.RoundTripper = &fakeRoundTripper{err: fmt.Errorf("Fake error\n")}
			ref := types.ManagedObjectReference{}
			rp := object.NewResourcePool(&vim25.Client{RoundTripper: rtp}, ref)
			cls, err := Vs.GetResourcePoolOwner(context.TODO(), rp)
			Expect(err).Should(HaveOccurred())
			Expect(cls).To(BeNil())
		})
	})
})
