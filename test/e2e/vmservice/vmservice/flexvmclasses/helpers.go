// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package flexvmclasses

import (
	"context"

	ginkgo "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	"k8s.io/apimachinery/pkg/api/resource"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
)

const mib = 1024 * 1024

// computeSnapshot is the CPU and memory configuration a VM realized in
// vSphere. Unset reservations are recorded as zero and unset limits as -1,
// which is how vSphere reports an unlimited allocation.
type computeSnapshot struct {
	NumCPU            int32 `json:"numCPU"`
	MemoryMB          int32 `json:"memoryMB"`
	CPUReservation    int64 `json:"cpuReservation"`
	CPULimit          int64 `json:"cpuLimit"`
	MemoryReservation int64 `json:"memoryReservation"`
	MemoryLimit       int64 `json:"memoryLimit"`
}

// getComputeSnapshot reads the VM's hardware and allocations through the
// property collector.
func getComputeSnapshot(ctx context.Context, vimClient *vim25.Client, moID string) computeSnapshot {
	ginkgo.GinkgoHelper()

	var vmMO mo.VirtualMachine
	ref := types.ManagedObjectReference{Type: "VirtualMachine", Value: moID}
	Expect(property.DefaultCollector(vimClient).RetrieveOne(ctx, ref, []string{"config"}, &vmMO)).To(Succeed(), "failed to retrieve config of VM %s", moID)
	Expect(vmMO.Config).NotTo(BeNil(), "VM %s has no config", moID)

	alloc := func(a *types.ResourceAllocationInfo) (reservation, limit int64) {
		limit = -1
		if a == nil {
			return
		}
		if a.Reservation != nil {
			reservation = *a.Reservation
		}
		if a.Limit != nil {
			limit = *a.Limit
		}
		return
	}

	s := computeSnapshot{NumCPU: vmMO.Config.Hardware.NumCPU, MemoryMB: vmMO.Config.Hardware.MemoryMB}
	s.CPUReservation, s.CPULimit = alloc(vmMO.Config.CpuAllocation)
	s.MemoryReservation, s.MemoryLimit = alloc(vmMO.Config.MemoryAllocation)
	return s
}

// expectFixedRange asserts that the range is fixed at the given value, which
// is how a legacy scalar is represented in v1alpha6.
func expectFixedRange(r *vmopv1.ResourceQuantityRange, want resource.Quantity, what string) {
	ginkgo.GinkgoHelper()

	Expect(r).NotTo(BeNil(), "%s range is unset", what)
	Expect(r.Min).NotTo(BeNil(), "%s min is unset", what)
	Expect(r.Max).NotTo(BeNil(), "%s max is unset", what)
	Expect(r.Default).NotTo(BeNil(), "%s default is unset", what)
	Expect(r.Min.Cmp(want)).To(BeZero(), "%s min is %s, want %s", what, r.Min, &want)
	Expect(r.Max.Cmp(want)).To(BeZero(), "%s max is %s, want %s", what, r.Max, &want)
	Expect(r.Default.Cmp(want)).To(BeZero(), "%s default is %s, want %s", what, r.Default, &want)
}

// expectRange asserts the exact min, default and max of a range.
func expectRange(r *vmopv1.ResourceQuantityRange, minimum, def, maximum resource.Quantity, what string) {
	ginkgo.GinkgoHelper()

	Expect(r).NotTo(BeNil(), "%s range is unset", what)
	Expect(r.Min).NotTo(BeNil(), "%s min is unset", what)
	Expect(r.Max).NotTo(BeNil(), "%s max is unset", what)
	Expect(r.Default).NotTo(BeNil(), "%s default is unset", what)
	Expect(r.Min.Cmp(minimum)).To(BeZero(), "%s min is %s, want %s", what, r.Min, &minimum)
	Expect(r.Default.Cmp(def)).To(BeZero(), "%s default is %s, want %s", what, r.Default, &def)
	Expect(r.Max.Cmp(maximum)).To(BeZero(), "%s max is %s, want %s", what, r.Max, &maximum)
}

// expectLegacyAllocation asserts how a legacy request or limit appears in
// v1alpha6: a zero legacy value is an unset range and anything else a fixed
// range.
func expectLegacyAllocation(r *vmopv1.ResourceQuantityRange, legacy resource.Quantity, what string) {
	ginkgo.GinkgoHelper()

	if legacy.IsZero() {
		Expect(r).To(BeNil(), "%s should be unset for a zero legacy value", what)
		return
	}
	expectFixedRange(r, legacy, what)
}
