// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// Package configpolicy provides the matching logic shared by every
// enforcement point for a vim.vmware.com VirtualMachineConfigPolicy: the
// VirtualMachine admission webhook (which validates a VM's desired spec)
// and the vSphere provider's power-on reconcile (which validates a VM's
// actual, live vSphere configuration). Validate is the single entry point
// for that logic; InputFromVM and InputFromConfigInfo build its Input from
// each side's respective data source, so the comparison rules themselves
// can never silently drift apart between the two enforcement points.
//
// Everything downstream of "did this violate the policy" -- resolving the
// policy that governs a VM's zone, deciding whether a given Create/Update/
// PowerOn mode should actually block the request, and translating a
// violation into a field.Error or a pkg/errors type -- is left to the
// caller, since those decisions depend on caller-specific context (a
// webhook's field paths, a reconcile's requeue semantics) that this
// package intentionally knows nothing about.
package configpolicy

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"

	vimtypes "github.com/vmware/govmomi/vim25/types"
	"k8s.io/apimachinery/pkg/api/resource"

	vimv1 "github.com/vmware-tanzu/vm-operator/external/vim/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
)

// Input describes the VM configuration data to validate against a
// VirtualMachineConfigPolicy. A nil pointer field means that data is not
// available from whatever source built the Input (e.g. a live
// VirtualMachineConfigInfo has no vNUMA *node count* field) and is treated
// as compliant, since there is nothing yet to compare.
//
// Every field here is one where (a) a directly corresponding vmopv1
// VirtualMachine spec field exists and (b) that field maps unambiguously to
// a single VirtualMachineConfigPolicySpec field with no cross-API enum
// translation required. The following VirtualMachineConfigPolicySpec
// fields are deliberately not represented in Input, and so are never
// checked by Validate:
//
//   - SMCPresent, SEVSupported, SEVSNPSupported, TDXSupported,
//     CPULockedToMaxSupported: no corresponding spec.* field exists to
//     compare against -- these describe host/cluster capabilities, not a
//     setting a VM's config can violate.
//   - NumSimultaneousThreads: no direct, general spec.* field was found.
//   - LatencySensitivityLevels: vmopv1's enum
//     (Normal/High/HighWithHyperthreading) and vimv1's enum
//     (Low/Normal/High) do not line up one-to-one; mapping
//     "HighWithHyperthreading" (and, on the provider side, govmomi's
//     "medium"/"custom") requires a deliberate decision this package does
//     not make.
//   - RSSSupported, UDPRSSSupported, LROSupported, TxRxThreadModels: these
//     are per-network-interface (vmxnet3) properties, not VM-level ones;
//     checking them means iterating spec.network.interfaces[], a
//     materially different check.
//   - The embedded ConfigTargetDevices fields (CD-ROM, floppy, serial,
//     parallel, sound, USB, PCI passthrough availability): these describe
//     available *devices*, matched by device type/backing, not a simple
//     range or boolean; a separate feature.
type Input struct {
	// ExtraConfigKeys are the keys set in the VM's advanced.extraConfig.
	ExtraConfigKeys []string

	// HardwareVersion is the VM's hardware version. The zero value means
	// unset.
	HardwareVersion vimv1.HardwareVersion

	// NumCPUCores is the number of vCPUs assigned to the VM.
	NumCPUCores *int32

	// Memory is the amount of memory assigned to the VM.
	Memory *resource.Quantity

	// NumNUMANodes is the number of vNUMA nodes assigned to the VM.
	NumNUMANodes *int32

	// IOMMUEnabled indicates whether the VM has IOMMU (Intel VT-d) enabled.
	IOMMUEnabled *bool

	// MemoryLockedToMax indicates whether the VM's memory reservation is
	// locked to its memory limit.
	MemoryLockedToMax *bool

	// HugePagesEnabled indicates whether the VM has 1 GB huge page backing
	// enabled.
	HugePagesEnabled *bool
}

// InputFromVM builds an Input from a VM's desired spec, for validating a
// VM's desired configuration -- e.g. from the VirtualMachine admission
// webhook.
func InputFromVM(vm *vmopv1.VirtualMachine) Input {
	var in Input

	if adv := vm.Spec.Advanced; adv != nil {
		for _, kv := range adv.ExtraConfig {
			in.ExtraConfigKeys = append(in.ExtraConfigKeys, kv.Key)
		}
		in.HugePagesEnabled = adv.HugePages1GEnabled
	}

	if vm.Spec.MinHardwareVersion != 0 {
		//nolint:gosec // already validated to be within the type's range.
		in.HardwareVersion = vimv1.HardwareVersion(vm.Spec.MinHardwareVersion)
	}

	if res := vm.Spec.Resources; res != nil && res.Size != nil {
		if cpu := res.Size.CPU; cpu != nil {
			//nolint:gosec // spec.resources.size.cpu is a small, whole vCPU count.
			numCPUCores := int32(cpu.Value())
			in.NumCPUCores = &numCPUCores
		}
		in.Memory = res.Size.Memory
	}

	if cpu := vm.Spec.CPUAdvanced; cpu != nil {
		if topo := cpu.Topology; topo != nil {
			in.NumNUMANodes = topo.VNUMANodeCount
		}
		in.IOMMUEnabled = cpu.IOMMUEnabled
	}

	if mem := vm.Spec.MemoryAdvanced; mem != nil {
		in.MemoryLockedToMax = mem.ReservationLockedToMax
	}

	return in
}

// InputFromConfigInfo builds an Input from a VM's live vSphere
// configuration, for validating a VM's actual configuration -- e.g. from
// the vSphere provider's power-on reconcile.
//
// NumNUMANodes and HugePagesEnabled are always left unset: ConfigInfo has
// no direct vNUMA *node count* field (only cores-per-node, which would
// need dividing out), and huge-page backing is only reachable live via a
// specific ExtraConfig key string that isn't exposed as a named constant
// anywhere in this codebase.
func InputFromConfigInfo(cfg vimtypes.VirtualMachineConfigInfo) Input {
	numCPUCores := cfg.Hardware.NumCPU
	memory := resource.NewQuantity(
		int64(cfg.Hardware.MemoryMB)*1024*1024, resource.BinarySI)

	in := Input{
		NumCPUCores:       &numCPUCores,
		Memory:            memory,
		IOMMUEnabled:      cfg.Flags.VvtdEnabled,
		MemoryLockedToMax: cfg.MemoryReservationLockedToMax,
	}

	for _, bov := range cfg.ExtraConfig {
		if ov := bov.GetOptionValue(); ov != nil {
			in.ExtraConfigKeys = append(in.ExtraConfigKeys, ov.Key)
		}
	}

	if hv, err := vimv1.ParseHardwareVersion(cfg.Version); err == nil {
		in.HardwareVersion = hv
	}

	return in
}

// AppliesToVM reports whether spec, a VirtualMachineConfigPolicy's spec,
// applies to a VM with the given desired class name. VMClassMode defaults
// to AsPolicy, which preserves pre-9.1 behavior: VM Class-derived config
// bypasses the policy entirely, so the policy does not apply.
func AppliesToVM(
	spec vimv1.VirtualMachineConfigPolicySpec, className string) bool {

	return className == "" ||
		spec.VMClassMode == vimv1.VirtualMachineConfigPolicyVMClassModeAsConfig
}

// Validate reports every way in which in violates spec, joined via
// errors.Join, or nil if in is fully compliant. Each violation is one of
// the Err*Violation types in this package; callers can inspect a returned
// error with errors.As, or flatten it into its individual violations with
// Violations.
func Validate(
	ctx context.Context,
	spec vimv1.VirtualMachineConfigPolicySpec,
	in Input) error {

	var violations []error

	for _, key := range in.ExtraConfigKeys {
		if err := checkExtraConfigKey(spec.ExtraConfig, key); err != nil {
			violations = append(violations, err)
		}
	}

	hvErr := checkHardwareVersion(spec.HardwareVersions, in.HardwareVersion)
	if hvErr != nil {
		violations = append(violations, hvErr)
	}

	if in.NumCPUCores != nil {
		if err := checkCPUCores(spec.NumCPUCores, *in.NumCPUCores); err != nil {
			violations = append(violations, err)
		}
	}

	if in.Memory != nil {
		if err := checkMemory(spec.Memory, *in.Memory); err != nil {
			violations = append(violations, err)
		}
	}

	if in.NumNUMANodes != nil {
		if err := checkNUMANodes(spec.NumNUMANodes, *in.NumNUMANodes); err != nil {
			violations = append(violations, err)
		}
	}

	if in.IOMMUEnabled != nil && *in.IOMMUEnabled && !spec.IOMMUSupported {
		violations = append(violations, &ErrIOMMUViolation{})
	}

	if in.MemoryLockedToMax != nil && *in.MemoryLockedToMax &&
		!spec.MemoryLockedToMaxSupported {
		violations = append(violations, &ErrMemoryLockedToMaxViolation{})
	}

	if in.HugePagesEnabled != nil && *in.HugePagesEnabled &&
		!spec.HugePagesSupported {
		violations = append(violations, &ErrHugePagesViolation{})
	}

	return errors.Join(violations...)
}

// Violations flattens err -- nil, a single violation, or the result of
// errors.Join as returned by Validate -- into its individual violations.
// It returns nil if err is nil.
func Violations(err error) []error {
	if err == nil {
		return nil
	}

	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		var out []error
		for _, e := range joined.Unwrap() {
			out = append(out, Violations(e)...)
		}
		return out
	}

	return []error{err}
}

// MatchesExtraConfigKey reports whether key matches k per k.Type: Fixed is
// an exact match, Regex uses regexp.MatchString, and Glob uses
// filepath.Match semantics. A malformed Regex or Glob pattern is treated as
// a non-match rather than failing on the policy's data.
func MatchesExtraConfigKey(
	k vimv1.VirtualMachineConfigPolicyExtraConfigKey, key string) bool {
	switch k.Type {
	case vimv1.MatchTypeRegex:
		matched, err := regexp.MatchString(k.Key, key)
		return err == nil && matched
	case vimv1.MatchTypeGlob:
		matched, err := filepath.Match(k.Key, key)
		return err == nil && matched
	default: // vimv1.MatchTypeFixed
		return k.Key == key
	}
}

func matchesAny(
	keys []vimv1.VirtualMachineConfigPolicyExtraConfigKey, key string) bool {
	for _, k := range keys {
		if MatchesExtraConfigKey(k, key) {
			return true
		}
	}

	return false
}

// checkExtraConfigKey applies ec's denied/allowed lists to key, with
// Denied taking precedence over Allowed. It returns nil if ec is nil or
// key is compliant with ec.
func checkExtraConfigKey(
	ec *vimv1.VirtualMachineConfigPolicyExtraConfigSpec, key string) error {
	if ec == nil {
		return nil
	}

	if matchesAny(ec.Denied, key) {
		return &ErrExtraConfigViolation{Key: key, Denied: true}
	}

	if len(ec.Allowed) > 0 && !matchesAny(ec.Allowed, key) {
		return &ErrExtraConfigViolation{Key: key}
	}

	return nil
}

// checkHardwareVersion reports whether hv complies with r, a policy's
// spec.hardwareVersions range. Because the range is a min/max pair, a zero
// Min means there is no minimum and a zero Max means there is no maximum.
// It returns nil if r is nil, hv is the zero/invalid hardware version
// (nothing yet to compare), or hv is compliant with r.
func checkHardwareVersion(
	r *vimv1.HardwareVersionRange, hv vimv1.HardwareVersion) error {
	if r == nil || hv == 0 {
		return nil
	}

	if r.Min != 0 && hv < r.Min {
		return &ErrHardwareVersionViolation{Version: hv, Range: *r}
	}

	if r.Max != 0 && hv > r.Max {
		return &ErrHardwareVersionViolation{Version: hv, Range: *r, Above: true}
	}

	return nil
}

// checkCPUCores reports whether got complies with r. A zero Min means
// there is no minimum; a zero Max means there is no maximum. It returns
// nil if r is nil or got is compliant with r.
func checkCPUCores(r *vimv1.IntRange, got int32) error {
	if r == nil {
		return nil
	}

	if r.Min != 0 && got < r.Min {
		return &ErrCPUCoresViolation{Got: got, Range: *r}
	}

	if r.Max != 0 && got > r.Max {
		return &ErrCPUCoresViolation{Got: got, Range: *r, Above: true}
	}

	return nil
}

// checkNUMANodes reports whether got complies with r. A zero Min means
// there is no minimum; a zero Max means there is no maximum. It returns
// nil if r is nil or got is compliant with r.
func checkNUMANodes(r *vimv1.IntRange, got int32) error {
	if r == nil {
		return nil
	}

	if r.Min != 0 && got < r.Min {
		return &ErrNUMANodesViolation{Got: got, Range: *r}
	}

	if r.Max != 0 && got > r.Max {
		return &ErrNUMANodesViolation{Got: got, Range: *r, Above: true}
	}

	return nil
}

// checkMemory reports whether got complies with r. A zero-value Min means
// there is no minimum; a zero-value Max means there is no maximum. It
// returns nil if r is nil or got is compliant with r.
func checkMemory(r *vimv1.ResourceQuantityRange, got resource.Quantity) error {
	if r == nil {
		return nil
	}

	if !r.Min.IsZero() && got.Cmp(r.Min) < 0 {
		return &ErrMemoryViolation{Got: got, Range: *r}
	}

	if !r.Max.IsZero() && got.Cmp(r.Max) > 0 {
		return &ErrMemoryViolation{Got: got, Range: *r, Above: true}
	}

	return nil
}

// withCause appends err's message to msg, if err is non-nil.
func withCause(msg string, err error) string {
	if err == nil {
		return msg
	}
	return fmt.Sprintf("%s: %v", msg, err)
}

// ErrExtraConfigViolation indicates that an advanced.extraConfig key is
// denied by, or absent from the allowed list of, a
// VirtualMachineConfigPolicy.
type ErrExtraConfigViolation struct {
	// Key is the extraConfig key in violation.
	Key string

	// Denied is true if Key matched the policy's denied list; false if Key
	// is absent from a non-empty allowed list.
	Denied bool

	// Err, if non-nil, is included in Error and returned by Unwrap.
	Err error
}

func (e *ErrExtraConfigViolation) Error() string {
	if e.Denied {
		return withCause(fmt.Sprintf(
			"%s: denied by the namespace's VirtualMachineConfigPolicy",
			e.Key), e.Err)
	}
	return withCause(fmt.Sprintf(
		"%s: not in the namespace's VirtualMachineConfigPolicy allowed list",
		e.Key), e.Err)
}

func (e *ErrExtraConfigViolation) Unwrap() error { return e.Err }

// ErrHardwareVersionViolation indicates that a VM's hardware version falls
// outside a VirtualMachineConfigPolicy's supported hardwareVersions range.
type ErrHardwareVersionViolation struct {
	// Version is the hardware version in violation.
	Version vimv1.HardwareVersion

	// Range is the policy's supported hardwareVersions range.
	Range vimv1.HardwareVersionRange

	// Above is true if Version exceeds Range.Max; false if Version is
	// below Range.Min.
	Above bool

	// Err, if non-nil, is included in Error and returned by Unwrap.
	Err error
}

func (e *ErrHardwareVersionViolation) Error() string {
	if e.Above {
		return withCause(fmt.Sprintf(
			"%s exceeds the maximum hardware version %s supported by "+
				"the namespace's VirtualMachineConfigPolicy",
			e.Version, e.Range.Max), e.Err)
	}
	return withCause(fmt.Sprintf(
		"%s is below the minimum hardware version %s supported by "+
			"the namespace's VirtualMachineConfigPolicy",
		e.Version, e.Range.Min), e.Err)
}

func (e *ErrHardwareVersionViolation) Unwrap() error { return e.Err }

// ErrCPUCoresViolation indicates that a VM's number of CPU cores falls
// outside a VirtualMachineConfigPolicy's supported numCPUCores range.
type ErrCPUCoresViolation struct {
	// Got is the number of CPU cores in violation.
	Got int32

	// Range is the policy's supported numCPUCores range.
	Range vimv1.IntRange

	// Above is true if Got exceeds Range.Max; false if Got is below
	// Range.Min.
	Above bool

	// Err, if non-nil, is included in Error and returned by Unwrap.
	Err error
}

func (e *ErrCPUCoresViolation) Error() string {
	if e.Above {
		return withCause(fmt.Sprintf(
			"number of CPU cores %d exceeds the maximum %d supported "+
				"by the namespace's VirtualMachineConfigPolicy",
			e.Got, e.Range.Max), e.Err)
	}
	return withCause(fmt.Sprintf(
		"number of CPU cores %d is below the minimum %d supported "+
			"by the namespace's VirtualMachineConfigPolicy",
		e.Got, e.Range.Min), e.Err)
}

func (e *ErrCPUCoresViolation) Unwrap() error { return e.Err }

// ErrNUMANodesViolation indicates that a VM's number of vNUMA nodes falls
// outside a VirtualMachineConfigPolicy's supported numNUMANodes range.
type ErrNUMANodesViolation struct {
	// Got is the number of vNUMA nodes in violation.
	Got int32

	// Range is the policy's supported numNUMANodes range.
	Range vimv1.IntRange

	// Above is true if Got exceeds Range.Max; false if Got is below
	// Range.Min.
	Above bool

	// Err, if non-nil, is included in Error and returned by Unwrap.
	Err error
}

func (e *ErrNUMANodesViolation) Error() string {
	if e.Above {
		return withCause(fmt.Sprintf(
			"number of vNUMA nodes %d exceeds the maximum %d supported "+
				"by the namespace's VirtualMachineConfigPolicy",
			e.Got, e.Range.Max), e.Err)
	}
	return withCause(fmt.Sprintf(
		"number of vNUMA nodes %d is below the minimum %d supported "+
			"by the namespace's VirtualMachineConfigPolicy",
		e.Got, e.Range.Min), e.Err)
}

func (e *ErrNUMANodesViolation) Unwrap() error { return e.Err }

// ErrMemoryViolation indicates that a VM's memory falls outside a
// VirtualMachineConfigPolicy's supported memory range.
type ErrMemoryViolation struct {
	// Got is the memory quantity in violation.
	Got resource.Quantity

	// Range is the policy's supported memory range.
	Range vimv1.ResourceQuantityRange

	// Above is true if Got exceeds Range.Max; false if Got is below
	// Range.Min.
	Above bool

	// Err, if non-nil, is included in Error and returned by Unwrap.
	Err error
}

func (e *ErrMemoryViolation) Error() string {
	if e.Above {
		return withCause(fmt.Sprintf(
			"memory %s exceeds the maximum %s supported by the "+
				"namespace's VirtualMachineConfigPolicy",
			e.Got.String(), e.Range.Max.String()), e.Err)
	}
	return withCause(fmt.Sprintf(
		"memory %s is below the minimum %s supported by the "+
			"namespace's VirtualMachineConfigPolicy",
		e.Got.String(), e.Range.Min.String()), e.Err)
}

func (e *ErrMemoryViolation) Unwrap() error { return e.Err }

// ErrIOMMUViolation indicates that a VM has IOMMU enabled but the
// VirtualMachineConfigPolicy does not support it.
type ErrIOMMUViolation struct {
	// Err, if non-nil, is included in Error and returned by Unwrap.
	Err error
}

func (e *ErrIOMMUViolation) Error() string {
	return withCause(
		"IOMMU is not supported by the namespace's VirtualMachineConfigPolicy",
		e.Err)
}

func (e *ErrIOMMUViolation) Unwrap() error { return e.Err }

// ErrMemoryLockedToMaxViolation indicates that a VM has its memory
// reservation locked to its memory limit but the VirtualMachineConfigPolicy
// does not support it.
type ErrMemoryLockedToMaxViolation struct {
	// Err, if non-nil, is included in Error and returned by Unwrap.
	Err error
}

func (e *ErrMemoryLockedToMaxViolation) Error() string {
	return withCause(
		"memory reservation locked to max is not supported by the "+
			"namespace's VirtualMachineConfigPolicy",
		e.Err)
}

func (e *ErrMemoryLockedToMaxViolation) Unwrap() error { return e.Err }

// ErrHugePagesViolation indicates that a VM has 1 GB huge page backing
// enabled but the VirtualMachineConfigPolicy does not support it.
type ErrHugePagesViolation struct {
	// Err, if non-nil, is included in Error and returned by Unwrap.
	Err error
}

func (e *ErrHugePagesViolation) Error() string {
	return withCause(
		"1 GB huge pages is not supported by the namespace's "+
			"VirtualMachineConfigPolicy",
		e.Err)
}

func (e *ErrHugePagesViolation) Unwrap() error { return e.Err }
