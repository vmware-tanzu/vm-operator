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
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"

	vimv1 "github.com/vmware-tanzu/vm-operator/external/vim/api/v1alpha1"
)

// Merge returns a copy of spec with its ConfigTarget-derived fields
// replaced by the intersection of targets. Fields not derived from a
// ConfigTarget are copied through from spec unmodified. If targets is
// empty, spec is returned unmodified.
func Merge(
	spec vimv1.VirtualMachineConfigPolicySpec,
	targets ...vimv1.ConfigTargetStatus) vimv1.VirtualMachineConfigPolicySpec {
	if len(targets) == 0 {
		return spec
	}

	agg := aggregate{
		numCPUCores:            targets[0].NumCPUCores,
		numNUMANodes:           targets[0].NumNumaNodes,
		numSimultaneousThreads: targets[0].MaxSimultaneousThreads,
		memory:                 quantityOrZero(targets[0].SupportedMaxMem),
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
	out.NumCPUCores = mergeIntRangeMax(out.NumCPUCores, agg.numCPUCores)
	out.NumNUMANodes = mergeIntRangeMax(out.NumNUMANodes, agg.numNUMANodes)
	out.NumSimultaneousThreads = mergeIntRangeMax(out.NumSimultaneousThreads, agg.numSimultaneousThreads)
	out.Memory = mergeResourceQuantityRangeMax(out.Memory, agg.memory)
	out.SMCPresent = agg.smcPresent
	out.SEVSupported = agg.sevSupported
	out.SEVSNPSupported = agg.sevSNPSupported
	out.TDXSupported = agg.tdxSupported
	out.ConfigTargetDevices = agg.devices

	return out
}

// aggregate accumulates the fold-over-targets state used by Merge.
type aggregate struct {
	numCPUCores            int32
	numNUMANodes           int32
	numSimultaneousThreads int32
	memory                 resource.Quantity

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
	agg.memory = minQuantity(agg.memory, t.SupportedMaxMem)
	agg.smcPresent = agg.smcPresent && t.SMCPresent
	agg.sevSupported = agg.sevSupported && t.SEVSupported
	agg.sevSNPSupported = agg.sevSNPSupported && t.SEVSNPSupported
	agg.tdxSupported = agg.tdxSupported && t.TDXSupported
	agg.devices = intersectConfigTargetDevices(agg.devices, t.ConfigTargetDevices)

	return agg
}

// minInt32 returns the smaller of a and b. A zero value is treated as "no
// data reported" and does not restrict the result, since ConfigTargetStatus
// fields are all +optional and a target that has not finished populating
// its status should not silently zero out an otherwise-healthy merge.
func minInt32(a, b int32) int32 {
	switch {
	case a == 0:
		return b
	case b == 0:
		return a
	case a < b:
		return a
	default:
		return b
	}
}

// minQuantity returns the smaller of a and b. A nil/zero b is treated as
// "no data reported" and does not restrict the result; see minInt32.
func minQuantity(a resource.Quantity, b *resource.Quantity) resource.Quantity {
	if b == nil || b.IsZero() {
		return a
	}

	if a.IsZero() || b.Cmp(a) < 0 {
		return b.DeepCopy()
	}

	return a
}

// quantityOrZero returns *q, or the zero Quantity if q is nil.
func quantityOrZero(q *resource.Quantity) resource.Quantity {
	if q == nil {
		return resource.Quantity{}
	}

	return q.DeepCopy()
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
