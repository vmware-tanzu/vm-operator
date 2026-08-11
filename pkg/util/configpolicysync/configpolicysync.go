// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// Package configpolicysync maps one or more vim.vmware.com ConfigTarget
// statuses onto the fields of a VirtualMachineConfigPolicy spec that
// syncMode=ConfigTarget derives from vSphere cluster capabilities.
//
// A zone with more than one relevant cluster (multi-cluster zone) merges by
// intersection: the result is only what every target's cluster supports --
// the minimum of numeric maxima, the logical AND of boolean support flags,
// and, for ConfigTargetDevices, only the device descriptors that appear,
// value-for-value, on every target.
//
// Fields the policy spec owns independently of any ConfigTarget --
// createMode, updateMode, powerOnMode, vmClassMode, extraConfig,
// latencySensitivityLevels, txRxThreadModels, and the *Supported flags with
// no ConfigTargetStatus counterpart (cpuLockedToMaxSupported,
// memoryLockedToMaxSupported, hugePagesSupported, iommuSupported,
// rssSupported, udpRSSSupported, lroSupported) -- are left untouched by
// Merge.
package configpolicysync

import (
	"errors"
	"fmt"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"

	vimv1 "github.com/vmware-tanzu/vm-operator/external/vim/api/v1alpha1"
)

// Merge returns a copy of spec with its ConfigTarget-derived fields
// replaced by the intersection of targets. Fields not derived from a
// ConfigTarget are copied through from spec unmodified. If targets is
// empty, spec is returned unmodified.
//
// A cluster's reported maximum can narrow as well as widen -- e.g. a
// cluster loses hosts or is downgraded after an admin set a range field's
// Min. If a range field's derived Max would drop below an existing,
// tenant-managed Min, that field is left unchanged rather than publishing a
// Min > Max range, and Merge returns a non-nil error naming it -- every
// other field still converges normally, so one field in conflict does not
// freeze the rest of the policy's sync. Callers should surface the error
// (e.g. as a Ready=False condition) but must still apply the returned spec.
func Merge(
	spec vimv1.VirtualMachineConfigPolicySpec,
	targets ...vimv1.ConfigTargetStatus) (vimv1.VirtualMachineConfigPolicySpec, error) {
	if len(targets) == 0 {
		return spec, nil
	}

	agg := aggregate{
		numCPUCores:            targets[0].NumCPUCores,
		numNUMANodes:           targets[0].NumNumaNodes,
		numSimultaneousThreads: targets[0].MaxSimultaneousThreads,
		memory:                 quantityPtrCopy(targets[0].SupportedMaxMem),
		smcPresent:             targets[0].SMCPresent,
		sevSupported:           targets[0].SEVSupported,
		sevSNPSupported:        targets[0].SEVSNPSupported,
		tdxSupported:           targets[0].TDXSupported,
		devices:                *targets[0].ConfigTargetDevices.DeepCopy(),
	}

	for _, t := range targets[1:] {
		agg = agg.intersect(t)
	}

	out := spec

	var errs []error

	if r := mergeIntRangeMax(out.NumCPUCores, agg.numCPUCores); r.Min <= r.Max {
		out.NumCPUCores = r
	} else {
		errs = append(errs, fmt.Errorf("numCPUCores: min (%d) exceeds cluster-reported max (%d); field left unchanged", r.Min, r.Max))
	}

	if r := mergeIntRangeMax(out.NumNUMANodes, agg.numNUMANodes); r.Min <= r.Max {
		out.NumNUMANodes = r
	} else {
		errs = append(errs, fmt.Errorf("numNUMANodes: min (%d) exceeds cluster-reported max (%d); field left unchanged", r.Min, r.Max))
	}

	if r := mergeIntRangeMax(out.NumSimultaneousThreads, agg.numSimultaneousThreads); r.Min <= r.Max {
		out.NumSimultaneousThreads = r
	} else {
		errs = append(errs, fmt.Errorf("numSimultaneousThreads: min (%d) exceeds cluster-reported max (%d); field left unchanged", r.Min, r.Max))
	}

	// A nil agg.memory means no target reported SupportedMaxMem -- leave
	// Memory untouched rather than treating "no data" as a real zero.
	if agg.memory != nil {
		if r := mergeResourceQuantityRangeMax(out.Memory, *agg.memory); r.Min.Cmp(r.Max) <= 0 {
			out.Memory = r
		} else {
			errs = append(errs, fmt.Errorf(
				"memory: min (%s) exceeds cluster-reported max (%s); field left unchanged", r.Min.String(), r.Max.String()))
		}
	}

	out.SMCPresent = agg.smcPresent
	out.SEVSupported = agg.sevSupported
	out.SEVSNPSupported = agg.sevSNPSupported
	out.TDXSupported = agg.tdxSupported
	out.ConfigTargetDevices = agg.devices

	return out, errors.Join(errs...)
}

// aggregate accumulates the fold-over-targets state used by Merge.
type aggregate struct {
	numCPUCores            int32
	numNUMANodes           int32
	numSimultaneousThreads int32

	// memory is nil until some target reports SupportedMaxMem -- see
	// minQuantityPtr.
	memory *resource.Quantity

	smcPresent      bool
	sevSupported    bool
	sevSNPSupported bool
	tdxSupported    bool

	devices vimv1.ConfigTargetDevices
}

// intersect folds t into agg, narrowing every field to what both agg and t
// support.
func (agg aggregate) intersect(t vimv1.ConfigTargetStatus) aggregate {
	agg.numCPUCores = minInt32(agg.numCPUCores, t.NumCPUCores)
	agg.numNUMANodes = minInt32(agg.numNUMANodes, t.NumNumaNodes)
	agg.numSimultaneousThreads = minInt32(agg.numSimultaneousThreads, t.MaxSimultaneousThreads)
	agg.memory = minQuantityPtr(agg.memory, t.SupportedMaxMem)
	agg.smcPresent = agg.smcPresent && t.SMCPresent
	agg.sevSupported = agg.sevSupported && t.SEVSupported
	agg.sevSNPSupported = agg.sevSNPSupported && t.SEVSNPSupported
	agg.tdxSupported = agg.tdxSupported && t.TDXSupported
	agg.devices = intersectConfigTargetDevices(agg.devices, t.ConfigTargetDevices)

	return agg
}

// minInt32 returns the smaller of a and b. Callers are expected to only pass
// status from a Ready ConfigTarget (see the Reconciler's getConfigTargets),
// so a zero value here is real reported data, not "not populated yet" --
// e.g. a zero MaxSimultaneousThreads means the cluster does not support
// HT/SMT, per VirtualMachineConfigPolicySpec's field docs.
func minInt32(a, b int32) int32 {
	if a < b {
		return a
	}

	return b
}

// minQuantityPtr returns the smaller of a and b, treating either as "not
// reported by this ConfigTarget field" (SupportedMaxMem is +optional) when
// nil, rather than a real zero -- so seeding or folding in a target that
// omitted the field never forces the result to zero and discards every
// other target's real reported value. A zero, non-nil Quantity is real
// data, see minInt32.
func minQuantityPtr(a, b *resource.Quantity) *resource.Quantity {
	if a == nil {
		return quantityPtrCopy(b)
	}

	if b == nil {
		return a
	}

	if b.Cmp(*a) < 0 {
		return quantityPtrCopy(b)
	}

	return a
}

// quantityPtrCopy returns a deep copy of q, or nil if q is nil.
func quantityPtrCopy(q *resource.Quantity) *resource.Quantity {
	if q == nil {
		return nil
	}

	c := q.DeepCopy()

	return &c
}

// mergeIntRangeMax returns a copy of existing with Max set to maxVal, or a
// new IntRange{Max: maxVal} if existing is nil. Min is left untouched: it is
// not derived from any ConfigTarget, and is a tenant-managed floor.
func mergeIntRangeMax(existing *vimv1.IntRange, maxVal int32) *vimv1.IntRange {
	out := vimv1.IntRange{Max: maxVal}
	if existing != nil {
		out.Min = existing.Min
	}

	return &out
}

// mergeResourceQuantityRangeMax returns a copy of existing with Max set to
// maxVal, or a new ResourceQuantityRange{Max: maxVal} if existing is nil.
// Min is left untouched; see mergeIntRangeMax.
func mergeResourceQuantityRangeMax(
	existing *vimv1.ResourceQuantityRange, maxVal resource.Quantity) *vimv1.ResourceQuantityRange {
	out := vimv1.ResourceQuantityRange{Max: maxVal}
	if existing != nil {
		out.Min = existing.Min
	}

	return &out
}

// intersectConfigTargetDevices returns the ConfigTargetDevices categories
// common to both a and b: for list fields, only entries that are, value for
// value, present in both lists; for the single SGXTargetInfo pointer, the
// shared value if both are equal, otherwise nil.
func intersectConfigTargetDevices(a, b vimv1.ConfigTargetDevices) vimv1.ConfigTargetDevices {
	return vimv1.ConfigTargetDevices{
		CDROM:                     intersectDeepEqual(a.CDROM, b.CDROM),
		Floppy:                    intersectDeepEqual(a.Floppy, b.Floppy),
		Serial:                    intersectDeepEqual(a.Serial, b.Serial),
		Parallel:                  intersectDeepEqual(a.Parallel, b.Parallel),
		Sound:                     intersectDeepEqual(a.Sound, b.Sound),
		USB:                       intersectDeepEqual(a.USB, b.USB),
		PCIPassthrough:            intersectDeepEqual(a.PCIPassthrough, b.PCIPassthrough),
		DynamicPassthroughDevices: intersectDeepEqual(a.DynamicPassthroughDevices, b.DynamicPassthroughDevices),
		SRIOV:                     intersectDeepEqual(a.SRIOV, b.SRIOV),
		VGPUDevice:                intersectDeepEqual(a.VGPUDevice, b.VGPUDevice),
		VGPUProfile:               intersectDeepEqual(a.VGPUProfile, b.VGPUProfile),
		SharedGPUPassthroughTypes: intersectDeepEqual(a.SharedGPUPassthroughTypes, b.SharedGPUPassthroughTypes),
		SGXTargetInfo:             intersectPtr(a.SGXTargetInfo, b.SGXTargetInfo),
		PrecisionClockInfo:        intersectDeepEqual(a.PrecisionClockInfo, b.PrecisionClockInfo),
		VendorDeviceGroupInfo:     intersectDeepEqual(a.VendorDeviceGroupInfo, b.VendorDeviceGroupInfo),
		DVXClassInfo:              intersectDeepEqual(a.DVXClassInfo, b.DVXClassInfo),
		IDEDisks:                  intersectDeepEqual(a.IDEDisks, b.IDEDisks),
		SCSIDisks:                 intersectDeepEqual(a.SCSIDisks, b.SCSIDisks),
		SCSIPassthrough:           intersectDeepEqual(a.SCSIPassthrough, b.SCSIPassthrough),
		VFlashModule:              intersectDeepEqual(a.VFlashModule, b.VFlashModule),
	}
}

// intersectDeepEqual returns the elements of a that have an equal (per
// apiequality.Semantic.DeepEqual) counterpart somewhere in b.
func intersectDeepEqual[T any](a, b []T) []T {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}

	out := make([]T, 0, len(a))

	for _, av := range a {
		for _, bv := range b {
			if apiequality.Semantic.DeepEqual(av, bv) {
				out = append(out, av)
				break
			}
		}
	}

	if len(out) == 0 {
		return nil
	}

	return out
}

// intersectPtr returns a if a and b are both non-nil and equal, else nil.
func intersectPtr[T any](a, b *T) *T {
	if a == nil || b == nil || !apiequality.Semantic.DeepEqual(a, b) {
		return nil
	}

	return a
}
