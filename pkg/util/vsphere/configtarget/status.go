// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// Package configtarget maps a vSphere EnvironmentBrowser's
// QueryConfigTarget and QueryConfigOptionDescriptor results onto
// vimv1.ConfigTargetStatus.
package configtarget

import (
	vimtypes "github.com/vmware/govmomi/vim25/types"
	"k8s.io/apimachinery/pkg/api/resource"

	vimv1 "github.com/vmware-tanzu/vm-operator/external/vim/api/v1alpha1"
)

// bytesPerMB is used to convert vSphere's MB-denominated memory fields into
// the byte-denominated resource.Quantity fields used by ConfigTargetStatus.
const bytesPerMB = 1024 * 1024

// PopulateStatus maps the vSphere QueryConfigTarget result and
// QueryConfigOptionDescriptor descriptors onto obj's status: numeric
// limits, memory quantities, security flags, MaxHardwareVersion, and every
// ConfigTargetDevices category except SR-IOV. SR-IOV requires per-host
// attribution (host MoID, DVX capabilities) that this cluster-scope result
// does not carry; that per-host enrichment is not supported yet -- see
// vmop-3926 (T121).
func PopulateStatus(
	obj *vimv1.ConfigTarget,
	ct *vimtypes.ConfigTarget,
	descriptors []vimtypes.VirtualMachineConfigOptionDescriptor) {
	obj.Status.MaxHardwareVersion = computeMaxHardwareVersion(descriptors)

	if ct == nil {
		return
	}

	populateConfigTargetDevices(&obj.Status.ConfigTargetDevices, ct)

	obj.Status.NumCPUs = ct.NumCpus
	obj.Status.NumCPUCores = ct.NumCpuCores
	obj.Status.NumNumaNodes = ct.NumNumaNodes
	obj.Status.MaxCPUsPerVM = ct.MaxCpusPerHost
	obj.Status.MaxSimultaneousThreads = ct.MaxSimultaneousThreads
	obj.Status.SMCPresent = ct.SmcPresent

	if ct.MaxMemMBOptimalPerf > 0 {
		obj.Status.MaxMemOptimalPerf = resource.NewQuantity(int64(ct.MaxMemMBOptimalPerf)*bytesPerMB, resource.BinarySI)
	} else {
		obj.Status.MaxMemOptimalPerf = nil
	}

	if ct.SupportedMaxMemMB > 0 {
		obj.Status.SupportedMaxMem = resource.NewQuantity(int64(ct.SupportedMaxMemMB)*bytesPerMB, resource.BinarySI)
	} else {
		obj.Status.SupportedMaxMem = nil
	}

	if ct.AvailablePersistentMemoryReservationMB > 0 {
		obj.Status.AvailablePersistentMemoryReservation = resource.NewQuantity(
			ct.AvailablePersistentMemoryReservationMB*bytesPerMB, resource.BinarySI)
	} else {
		obj.Status.AvailablePersistentMemoryReservation = nil
	}

	obj.Status.SEVSupported = ct.SevSupported != nil && *ct.SevSupported
	obj.Status.SEVSNPSupported = ct.SevSnpSupported != nil && *ct.SevSnpSupported
	obj.Status.TDXSupported = ct.TdxSupported != nil && *ct.TdxSupported
}

// computeMaxHardwareVersion returns ConfigTarget.status.maxHardwareVersion:
// the highest virtual hardware version creatable on at least one host in
// the cluster.
//
// Each VirtualMachineConfigOptionDescriptor returned by
// QueryConfigOptionDescriptor represents one candidate hardware version
// (Key, e.g. "vmx-21") for this EnvironmentBrowser's scope, with
// CreateSupported reporting whether a VM can actually be created at that
// hardware version on at least one host in scope -- some keys are
// deprecated or not yet supported by any host and come back with
// CreateSupported false. MaxHardwareVersion's contract is "a VM at this
// version fits somewhere in the cluster," which is exactly what the
// highest Key among CreateSupported descriptors guarantees; there is no
// per-host vSphere property to aggregate instead (HostConfigInfo has no
// "default hardware version" field).
//
// Descriptors with an empty or unparsable Key are skipped rather than
// treated as an error, since a malformed descriptor should not prevent
// computing a value from the rest.
func computeMaxHardwareVersion(descriptors []vimtypes.VirtualMachineConfigOptionDescriptor) string {
	var maxVer vimtypes.HardwareVersion

	for _, d := range descriptors {
		if d.Key == "" || !d.CreateSupported {
			continue
		}

		hv, err := vimtypes.ParseHardwareVersion(d.Key)
		if err != nil || !hv.IsValid() {
			continue
		}

		if hv > maxVer {
			maxVer = hv
		}
	}

	return maxVer.String()
}
