// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package placement_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/placement"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func createRelocateSpec() *vimtypes.VirtualMachineRelocateSpec {
	spec := &vimtypes.VirtualMachineRelocateSpec{}
	spec.Host = &vimtypes.ManagedObjectReference{}
	spec.Pool = &vimtypes.ManagedObjectReference{}
	spec.Datastore = &vimtypes.ManagedObjectReference{}
	return spec
}

func createValidPlacementAction() (vimtypes.BaseClusterAction, *vimtypes.VirtualMachineRelocateSpec) {
	action := vimtypes.PlacementAction{}
	action.RelocateSpec = createRelocateSpec()
	return vimtypes.BaseClusterAction(&action), action.RelocateSpec
}

func createInvalidPlacementAction() vimtypes.BaseClusterAction {
	action := vimtypes.PlacementAction{}
	action.RelocateSpec = createRelocateSpec()
	action.RelocateSpec.Host = nil
	return vimtypes.BaseClusterAction(&action)
}

func createStoragePlacementAction() vimtypes.BaseClusterAction {
	action := vimtypes.StoragePlacementAction{}
	action.RelocateSpec = *createRelocateSpec()
	return vimtypes.BaseClusterAction(&action)
}

func createInvalidRecommendation() vimtypes.ClusterRecommendation {
	r := vimtypes.ClusterRecommendation{}
	r.Reason = string(vimtypes.RecommendationReasonCodeXvmotionPlacement)
	r.Action = append(r.Action, createStoragePlacementAction())
	r.Action = append(r.Action, createInvalidPlacementAction())
	return r
}

func createValidRecommendation() (vimtypes.ClusterRecommendation, *vimtypes.VirtualMachineRelocateSpec) {
	r := createInvalidRecommendation()
	a, s := createValidPlacementAction()
	r.Action = append(r.Action, a)
	return r, s
}

var _ = Describe("ParsePlaceVMResponse", func() {
	var vmCtx pkgctx.VirtualMachineContext

	BeforeEach(func() {
		vmCtx = pkgctx.VirtualMachineContext{
			Context: context.TODO(),
			VM:      builder.DummyVirtualMachine(),
			Logger:  suite.GetLogger(),
		}
	})

	Context("when response is valid", func() {
		Specify("PlaceVm Response is valid", func() {
			res := vimtypes.PlacementResult{}
			res.Recommendations = append(res.Recommendations, createInvalidRecommendation(), createInvalidRecommendation())
			rec, _ := createValidRecommendation()
			rec.Reason = string(vimtypes.RecommendationReasonCodePowerOnVm)
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
			res := vimtypes.PlacementResult{}
			rSpec := placement.ParseRelocateVMResponse(vmCtx, &res)
			Expect(rSpec).To(BeNil())
		})
	})

	Context("when response is not valid", func() {
		Specify("PlaceVm Response with invalid recommendations only", func() {
			res := vimtypes.PlacementResult{}
			res.Recommendations = append(res.Recommendations, createInvalidRecommendation(), createInvalidRecommendation())
			rec, _ := createValidRecommendation()
			rec.Reason = string(vimtypes.RecommendationReasonCodePowerOnVm)
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
