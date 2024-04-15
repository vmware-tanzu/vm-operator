// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package placement_test

import (
	goctx "context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/placement"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

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

var _ = Describe("ParsePlaceVMResponse", func() {
	var vmCtx context.VirtualMachineContext

	BeforeEach(func() {
		vmCtx = context.VirtualMachineContext{
			Context: goctx.TODO(),
			VM:      builder.DummyVirtualMachineA2(),
			Logger:  suite.GetLogger(),
		}
	})

	Context("when response is valid", func() {
		Specify("PlaceVm Response is valid", func() {
			res := types.PlacementResult{}
			res.Recommendations = append(res.Recommendations, createInvalidRecommendation(), createInvalidRecommendation())
			rec, _ := createValidRecommendation()
			rec.Reason = string(types.RecommendationReasonCodePowerOnVm)
			res.Recommendations = append(res.Recommendations, rec)
			rec, spec := createValidRecommendation()
			res.Recommendations = append(res.Recommendations, rec)

			rSpec := placement.ParseRelocateVMResponse(vmCtx, &res)
			Expect(rSpec).NotTo(BeNil())
			Expect(rSpec.Host).To(BeEquivalentTo(spec.Host))
			Expect(rSpec.Pool).To(BeEquivalentTo(spec.Pool))
			Expect(rSpec.Datastore).To(BeEquivalentTo(spec.Datastore))
		})
	})

	Context("when response is not valid", func() {
		Specify("PlaceVm Response without recommendations", func() {
			res := types.PlacementResult{}
			rSpec := placement.ParseRelocateVMResponse(vmCtx, &res)
			Expect(rSpec).To(BeNil())
		})
	})

	Context("when response is not valid", func() {
		Specify("PlaceVm Response with invalid recommendations only", func() {
			res := types.PlacementResult{}
			res.Recommendations = append(res.Recommendations, createInvalidRecommendation(), createInvalidRecommendation())
			rec, _ := createValidRecommendation()
			rec.Reason = string(types.RecommendationReasonCodePowerOnVm)
			res.Recommendations = append(res.Recommendations, rec)

			rSpec := placement.ParseRelocateVMResponse(vmCtx, &res)
			Expect(rSpec).To(BeNil())
		})
	})
})

var _ = Describe("CheckPlacementRelocateSpec", func() {

	Context("when relocation spec is valid", func() {
		Specify("Relocation spec is valid", func() {
			spec := createRelocateSpec()
			err := placement.CheckPlacementRelocateSpec(spec)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("when relocation spec is not valid", func() {
		Specify("Relocation spec is nil", func() {
			err := placement.CheckPlacementRelocateSpec(nil)
			Expect(err).To(HaveOccurred())
		})

		Specify("Host is nil", func() {
			spec := createRelocateSpec()
			spec.Host = nil
			err := placement.CheckPlacementRelocateSpec(spec)
			Expect(err).To(HaveOccurred())
		})

		Specify("Pool is nil", func() {
			spec := createRelocateSpec()
			spec.Pool = nil
			err := placement.CheckPlacementRelocateSpec(spec)
			Expect(err).To(HaveOccurred())
		})

		Specify("Datastore is nil", func() {
			spec := createRelocateSpec()
			spec.Datastore = nil
			err := placement.CheckPlacementRelocateSpec(spec)
			Expect(err).To(HaveOccurred())
		})
	})
})
