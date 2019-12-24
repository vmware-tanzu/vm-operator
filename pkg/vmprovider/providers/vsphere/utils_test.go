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
	"github.com/vmware/govmomi/simulator"
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
	dc, err := finder.Datacenter(ctx, simulator.Map.Any("Datacenter").Reference().Value)
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

func createRelocateSpec() *types.VirtualMachineRelocateSpec {
	spec := &types.VirtualMachineRelocateSpec{}
	spec.Host = &types.ManagedObjectReference{}
	spec.Pool = &types.ManagedObjectReference{}
	spec.Datastore = &types.ManagedObjectReference{}
	return spec
}

func createValidPlacementAction() (types.BaseClusterAction, *types.VirtualMachineRelocateSpec) {
	action := types.PlacementAction{}
	action.RelocateSpec = createRelocateSpec()
	return types.BaseClusterAction(&action), action.RelocateSpec
}

func createInvalidPlacementAction() types.BaseClusterAction {
	action := types.PlacementAction{}
	action.RelocateSpec = createRelocateSpec()
	action.RelocateSpec.Host = nil
	return types.BaseClusterAction(&action)
}

func createStoragePlacementAction() types.BaseClusterAction {
	action := types.StoragePlacementAction{}
	action.RelocateSpec = *createRelocateSpec()
	return types.BaseClusterAction(&action)
}

func createInvalidRecommendation() types.ClusterRecommendation {
	r := types.ClusterRecommendation{}
	r.Reason = string(types.RecommendationReasonCodeXvmotionPlacement)
	r.Action = append(r.Action, createStoragePlacementAction())
	r.Action = append(r.Action, createInvalidPlacementAction())
	return r
}

func createValidRecommendation() (types.ClusterRecommendation, *types.VirtualMachineRelocateSpec) {
	r := createInvalidRecommendation()
	a, s := createValidPlacementAction()
	r.Action = append(r.Action, a)
	return r, s
}

var _ = Describe("CheckPlacementRelocateSpec", func() {
	Context("when relocation spec is valid", func() {
		Specify("Relocation spec is valid", func() {
			spec := createRelocateSpec()
			isValid := Vs.CheckPlacementRelocateSpec(spec)
			Expect(isValid).To(BeTrue())
		})
	})
	Context("when relocation spec is not valid", func() {
		Specify("Relocation spec is nil", func() {
			isValid := Vs.CheckPlacementRelocateSpec(nil)
			Expect(isValid).To(BeFalse())
		})
		Specify("Host is nil", func() {
			spec := createRelocateSpec()
			spec.Host = nil
			isValid := Vs.CheckPlacementRelocateSpec(spec)
			Expect(isValid).To(BeFalse())
		})
		Specify("Pool is nil", func() {
			spec := createRelocateSpec()
			spec.Pool = nil
			isValid := Vs.CheckPlacementRelocateSpec(spec)
			Expect(isValid).To(BeFalse())
		})
		Specify("Datastore is nil", func() {
			spec := createRelocateSpec()
			spec.Datastore = nil
			isValid := Vs.CheckPlacementRelocateSpec(spec)
			Expect(isValid).To(BeFalse())
		})
	})
})

var _ = Describe("ParsePlaceVmResponse", func() {
	Context("when response is valid", func() {
		Specify("PlaceVm Response is valid", func() {
			res := types.PlacementResult{}
			res.Recommendations = append(res.Recommendations, createInvalidRecommendation())
			res.Recommendations = append(res.Recommendations, createInvalidRecommendation())
			rec, _ := createValidRecommendation()
			rec.Reason = string(types.RecommendationReasonCodePowerOnVm)
			res.Recommendations = append(res.Recommendations, rec)
			rec, spec := createValidRecommendation()
			res.Recommendations = append(res.Recommendations, rec)
			rSpec := Vs.ParsePlaceVmResponse(&res)
			Expect(rSpec).NotTo(BeNil())
			Expect(rSpec.Host).To(BeEquivalentTo(spec.Host))
			Expect(rSpec.Pool).To(BeEquivalentTo(spec.Pool))
			Expect(rSpec.Datastore).To(BeEquivalentTo(spec.Datastore))
		})
	})
	Context("when response is not valid", func() {
		Specify("PlaceVm Response without recommendations", func() {
			res := types.PlacementResult{}
			rSpec := Vs.ParsePlaceVmResponse(&res)
			Expect(rSpec).To(BeNil())
		})
	})
	Context("when response is not valid", func() {
		Specify("PlaceVm Response with invalid recommendations only", func() {
			res := types.PlacementResult{}
			res.Recommendations = append(res.Recommendations, createInvalidRecommendation())
			res.Recommendations = append(res.Recommendations, createInvalidRecommendation())
			rec, _ := createValidRecommendation()
			rec.Reason = string(types.RecommendationReasonCodePowerOnVm)
			res.Recommendations = append(res.Recommendations, rec)
			rSpec := Vs.ParsePlaceVmResponse(&res)
			Expect(rSpec).To(BeNil())
		})
	})
})
