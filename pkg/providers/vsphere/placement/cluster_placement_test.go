// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package placement_test

import (
	"context"
	"slices"

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

var _ = Describe("PlaceVMsXClusterErrors", func() {
	It("keeps full and per-member messages stable when DRS reorders resource pool faults", func() {
		faults := []vimtypes.PlaceVmsXClusterResultPlacementFaults{
			{VmName: "vm-1", ResourcePool: vimtypes.ManagedObjectReference{Value: "rp-2"},
				Faults: []vimtypes.LocalizedMethodFault{{LocalizedMessage: "second pool fault"}}},
			{VmName: "vm-2", ResourcePool: vimtypes.ManagedObjectReference{Value: "rp-3"},
				Faults: []vimtypes.LocalizedMethodFault{{LocalizedMessage: "other member fault"}}},
			{VmName: "vm-1", ResourcePool: vimtypes.ManagedObjectReference{Value: "rp-1"},
				Faults: []vimtypes.LocalizedMethodFault{{LocalizedMessage: "first pool fault"}}},
		}
		first := placement.NewPlaceVMsXClusterErrors(faults)
		reversed := slices.Clone(faults)
		slices.Reverse(reversed)
		second := placement.NewPlaceVMsXClusterErrors(reversed)
		Expect(first.Error()).To(Equal(
			"[VM vm-1: [resource pool (rp-1) has faults: first pool fault; " +
				"resource pool (rp-2) has faults: second pool fault] " +
				"VM vm-2: [resource pool (rp-3) has faults: other member fault]]"))
		Expect(first.ErrorForVM("vm-1")).To(MatchError(
			"resource pool (rp-1) has faults: first pool fault; " +
				"resource pool (rp-2) has faults: second pool fault"))
		Expect(first.Error()).To(Equal(second.Error()))
		Expect(first.ErrorForVM("vm-1").Error()).To(Equal(second.ErrorForVM("vm-1").Error()))
		Expect(first.ErrorForVM("vm-1").Error()).ToNot(ContainSubstring("other member fault"))
		Expect(first.FaultsByVM["vm-1"][0].ResourcePool.Value).To(Equal("rp-2"), "formatting must not mutate structured faults")
	})

	It("keeps faults for different VMs separate and retains all resource pool details", func() {
		err := placement.NewPlaceVMsXClusterErrors([]vimtypes.PlaceVmsXClusterResultPlacementFaults{
			{
				VmName:       "vm-1",
				ResourcePool: vimtypes.ManagedObjectReference{Value: "rp-1"},
				Faults:       []vimtypes.LocalizedMethodFault{{LocalizedMessage: "first VM fault"}},
			},
			{
				VmName:       "vm-2",
				ResourcePool: vimtypes.ManagedObjectReference{Value: "rp-2"},
				Faults:       []vimtypes.LocalizedMethodFault{{LocalizedMessage: "second VM fault"}},
			},
			{
				VmName:       "vm-1",
				ResourcePool: vimtypes.ManagedObjectReference{Value: "rp-3"},
				Faults: []vimtypes.LocalizedMethodFault{{
					Fault: &vimtypes.NoCompatibleHost{Error: []vimtypes.LocalizedMethodFault{{LocalizedMessage: "nested fault"}}},
				}},
			},
		})
		vmErr := err.ErrorForVM("vm-1")
		Expect(vmErr).To(HaveOccurred())
		Expect(vmErr.Error()).To(ContainSubstring("first VM fault"))
		Expect(vmErr.Error()).To(ContainSubstring("nested fault"))
		Expect(vmErr.Error()).To(ContainSubstring("rp-1"))
		Expect(vmErr.Error()).To(ContainSubstring("rp-3"))
		Expect(vmErr.Error()).ToNot(ContainSubstring("second VM fault"))
		Expect(err.ErrorForVM("vm-2")).To(MatchError(ContainSubstring("second VM fault")))
		Expect(err.ErrorForVM("vm-3")).ToNot(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("first VM fault"))
		Expect(err.Error()).To(ContainSubstring("second VM fault"))
		Expect(err.Error()).To(ContainSubstring("nested fault"))
	})

	It("retains the VM association when faults have no localized messages", func() {
		err := placement.NewPlaceVMsXClusterErrors([]vimtypes.PlaceVmsXClusterResultPlacementFaults{{
			VmName:       "vm-1",
			ResourcePool: vimtypes.ManagedObjectReference{Value: "rp-1"},
			Faults:       []vimtypes.LocalizedMethodFault{{Fault: &vimtypes.NoCompatibleHost{}}},
		}})
		Expect(err.ErrorForVM("vm-1")).To(MatchError(ContainSubstring("placement failed without fault details")))
		Expect(err.Error()).To(ContainSubstring("VM vm-1: [resource pool (rp-1)"))
		Expect(err.ErrorForVM("vm-2")).ToNot(HaveOccurred())
	})
})

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
