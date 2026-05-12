// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/go-logr/logr"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
)

// EthernetDeviceKeyBase is the vSphere device key base offset for ethernet
// (VirtualEthernetCard) devices. The ethernetX index used in VMX ExtraConfig
// keys (e.g. "ethernet1.ctxPerDev") equals deviceKey - EthernetDeviceKeyBase.
const EthernetDeviceKeyBase int32 = 4000

// EthernetExtraConfigPrefix returns the "ethernetX." prefix for VMX ExtraConfig
// keys corresponding to the ethernet device with the given device key.
func EthernetExtraConfigPrefix(deviceKey int32) string {
	return fmt.Sprintf("ethernet%d.", deviceKey-EthernetDeviceKeyBase)
}

var (
	cachedAdvancedVMXKeyMap = sync.OnceValue(func() map[string]int {
		return BuildVMXKeyMap(reflect.TypeOf(vmopv1.VirtualMachineAdvancedSpec{}))
	})

	cachedNICVMXKeyMap = sync.OnceValue(func() map[string]int {
		return BuildVMXNICKeyMap(
			reflect.TypeOf(vmopv1.VirtualMachineNetworkInterfaceVMXNet3Spec{}))
	})
)

// AdvancedVMXKeyMap returns the shared, lazily-built map of vmx struct tags
// to field indices for VirtualMachineAdvancedSpec.
func AdvancedVMXKeyMap() map[string]int {
	return cachedAdvancedVMXKeyMap()
}

// NICVMXKeyMap returns the shared, lazily-built map of bare vmxnet3 property
// names to field indices for VirtualMachineNetworkInterfaceVMXNet3Spec.
func NICVMXKeyMap() map[string]int {
	return cachedNICVMXKeyMap()
}

// BuildVMXKeyMap returns a map from vmx struct tag to field index for the given
// struct type. Fields whose vmx tag contains "%d" (NIC-indexed) are excluded;
// use BuildVMXNICKeyMap for those.
func BuildVMXKeyMap(t reflect.Type) map[string]int {
	m := make(map[string]int)
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("vmx")
		if tag == "" || strings.Contains(tag, "%d") {
			continue
		}
		m[tag] = i
	}
	return m
}

// BuildVMXNICKeyMap returns a map from the bare property name to field index
// for a NIC-tuning struct type. Only fields with vmx tags of the form
// "ethernet%d.<key>" are included; the returned map keys are the bare "<key>"
// portion (e.g. "ctxPerDev").
func BuildVMXNICKeyMap(t reflect.Type) map[string]int {
	m := make(map[string]int)
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("vmx")
		if tag == "" || !strings.Contains(tag, "%d") {
			continue
		}
		if _, rest, ok := strings.Cut(tag, "."); ok {
			m[rest] = i
		}
	}
	return m
}

// vmxStringDecoder translates a raw vSphere string value to the canonical spec
// value for a named string type (e.g. TxContextThreadingMode).
//
// Returns the canonical value to store, or "" to skip (invalid per the type's
// kubebuilder validation rule). A decoder should never return "" for a valid
// passthrough value since empty string is not a legal enum constant.
type vmxStringDecoder func(raw string) string

// vmxSliceDecoder decodes a raw vSphere string value into a slice of strings
// for a named string slice element type (e.g. PNICQueueFeature).
//
// Returns the decoded elements, or nil to skip (value is invalid or produces
// no elements).
type vmxSliceDecoder func(raw string) []string

// vmxBoolDecoder decodes a raw vSphere string value into a *bool for a named
// bool type (e.g. UDPRSSMode) that uses a non-standard wire encoding.
//
// Returns:
//   - &true / &false to set the pointer value.
//   - nil to leave the pointer nil (auto sentinel or invalid input).
type vmxBoolDecoder func(raw string) *bool

// vmxStringDecoders maps a named string element type to its decoder.
// Populated once by init().
var vmxStringDecoders map[reflect.Type]vmxStringDecoder

// vmxSliceDecoders maps a named string slice element type to its decoder.
// Populated once by init().
var vmxSliceDecoders map[reflect.Type]vmxSliceDecoder

// vmxBoolDecoders maps a named bool element type to its decoder.
// Populated once by init().
var vmxBoolDecoders map[reflect.Type]vmxBoolDecoder

func init() {
	vmxStringDecoders = map[reflect.Type]vmxStringDecoder{
		reflect.TypeOf(vmopv1.TxContextThreadingMode("")): decodeTxContextThreadingMode,
		reflect.TypeOf(vmopv1.CoalescingScheme("")):       decodeCoalescingScheme,
	}
	vmxSliceDecoders = map[reflect.Type]vmxSliceDecoder{
		reflect.TypeOf(vmopv1.PNICQueueFeature("")): decodePNICQueueFeatures,
	}
	vmxBoolDecoders = map[reflect.Type]vmxBoolDecoder{
		reflect.TypeOf(vmopv1.UDPRSSMode(false)): decodeUDPRSSMode,
	}
}

// loggerFromCtx returns a logger with a fixed component label for vmx decode
// operations. Falls back to the default controller-runtime logger when ctx has
// no logger attached.
func loggerFromCtx(ctx context.Context) logr.Logger {
	return pkglog.FromContextOrDefault(ctx).WithName("vmx-decode")
}

// DecodeVMXFieldValue decodes a raw vSphere ExtraConfig string value into rv.
// rv must be settable and must be of kind Ptr or Slice.
//
// Supported pointer element types: bool, string (and string-based named types).
// Supported slice element types: int32, string (and string-based named types).
//
// Bool pointer fields (*bool): true-ish values ("TRUE", "YES", "ON", "T", "Y",
// "1") set the pointer to true; false-ish and unrecognised values set it to
// false; host-default sentinels ("AUTO", "DEFAULT", "DONTCARE") leave it nil.
// Named bool types (e.g. UDPRSSMode) with a registered decoder use that
// decoder instead; unknown values leave the pointer nil.
//
// Named string pointer types with a registered decoder are translated to their
// canonical spec constant. Unknown values are stored raw if they pass the
// type's kubebuilder validation rule; otherwise the field is left nil/empty.
//
// []int32 is decoded from a comma-separated list of decimal integers.
// []string-based slices with a registered decoder use that decoder (e.g.
// bitmask for PNICQueueFeature). Unregistered slices store raw as a
// single-element slice.
func DecodeVMXFieldValue(ctx context.Context, rv reflect.Value, raw string) error {
	switch rv.Kind() {
	case reflect.Ptr:
		return decodePtrVMXField(ctx, rv, raw)
	case reflect.Slice:
		return decodeSliceVMXField(ctx, rv, raw)
	}
	return fmt.Errorf("unsupported field kind %v for vmx decode", rv.Kind())
}

func decodePtrVMXField(ctx context.Context, rv reflect.Value, raw string) error {
	logger := loggerFromCtx(ctx)
	elemType := rv.Type().Elem()

	switch elemType.Kind() {
	case reflect.Bool:
		// Check for a type-specific bool decoder first (e.g. UDPRSSMode uses
		// a non-standard integer wire encoding: 1=enabled, 2=disabled).
		if dec, ok := vmxBoolDecoders[elemType]; ok {
			b := dec(raw)
			if b == nil {
				logger.V(4).Info("vmx decode: bool value left nil", "raw", raw, "type", elemType)
				return nil
			}
			newPtr := reflect.New(elemType)
			newPtr.Elem().SetBool(*b)
			rv.Set(newPtr)
			return nil
		}
		b := decodeTristateBoolVMX(raw)
		if b == nil {
			logger.V(4).Info("vmx decode: host-default sentinel, leaving *bool nil", "raw", raw)
			return nil
		}
		newPtr := reflect.New(elemType)
		newPtr.Elem().SetBool(*b)
		rv.Set(newPtr)
		return nil
	case reflect.String:
		val := raw
		if dec, ok := vmxStringDecoders[elemType]; ok {
			canonical := dec(raw)
			if canonical == "" {
				logger.V(4).Info("vmx decode: string value skipped (invalid per type rule)",
					"raw", raw, "type", elemType)
				return nil
			}
			val = canonical
		}
		newPtr := reflect.New(elemType)
		newPtr.Elem().SetString(val)
		rv.Set(newPtr)
		return nil
	default:
		return fmt.Errorf("unsupported pointer element kind %v for vmx decode", elemType.Kind())
	}
}

func decodeSliceVMXField(ctx context.Context, rv reflect.Value, raw string) error {
	logger := loggerFromCtx(ctx)
	elemType := rv.Type().Elem()

	switch elemType.Kind() {
	case reflect.Int32:
		parts := strings.Split(raw, ",")
		slice := reflect.MakeSlice(rv.Type(), 0, len(parts))
		for _, p := range parts {
			n, err := strconv.ParseInt(strings.TrimSpace(p), 10, 32)
			if err != nil {
				logger.V(1).Info("vmx decode: cannot parse int32 in value, skipping field",
					"part", p, "raw", raw)
				return nil //nolint:nilerr // intentional: log and skip rather than propagate
			}
			elem := reflect.New(elemType).Elem()
			elem.SetInt(n)
			slice = reflect.Append(slice, elem)
		}
		rv.Set(slice)
	case reflect.String:
		if dec, ok := vmxSliceDecoders[elemType]; ok {
			elems := dec(raw)
			if len(elems) == 0 {
				logger.V(4).Info("vmx decode: slice decoder returned no elements, leaving field nil",
					"raw", raw, "type", elemType)
				return nil
			}
			slice := reflect.MakeSlice(rv.Type(), len(elems), len(elems))
			for i, e := range elems {
				slice.Index(i).SetString(e)
			}
			rv.Set(slice)
			return nil
		}
		// No registered decoder: store raw as a single-element slice.
		slice := reflect.MakeSlice(rv.Type(), 1, 1)
		slice.Index(0).SetString(raw)
		rv.Set(slice)
	default:
		return fmt.Errorf("unsupported slice element kind %v for vmx decode", elemType.Kind())
	}
	return nil
}

// decodeTristateBoolVMX parses a VMX boolean string including the auto,
// default, and dontcare host-default sentinels (all comparisons are
// case-insensitive).
//
// Returns:
//   - &true  for true-ish values (TRUE, YES, ON, T, Y, 1).
//   - &false for false-ish or unrecognised values.
//   - nil    for host-default sentinels (AUTO, DEFAULT, DONTCARE); callers
//     should leave the *bool field unset (nil), which is the nil=auto
//     convention: nil means "let the hypervisor decide."
func decodeTristateBoolVMX(raw string) *bool {
	switch strings.ToUpper(raw) {
	case "TRUE", "1", "YES", "ON", "T", "Y":
		v := true
		return &v
	case "AUTO", "DEFAULT", "DONTCARE":
		return nil
	default:
		v := false
		return &v
	}
}

// decodeTxContextThreadingMode translates vSphere's integer encoding of the TX
// context threading mode to the spec constant.
//
// vSphere stores: "1"=PerDevice, "2"=PerVM, "3"=PerQueue.
// Unknown single-digit values (4-9) are passed through as weak-enum raw values
// since the validation rule accepts [1-9]. Multi-digit values, zero, and
// non-numeric strings return "" (skip).
//
// See vmopv1.TxContextThreadingMode XValidation rule for the accepted value set.
func decodeTxContextThreadingMode(raw string) string {
	switch raw {
	case "1":
		return string(vmopv1.TxContextThreadingModePerDevice)
	case "2":
		return string(vmopv1.TxContextThreadingModePerVM)
	case "3":
		return string(vmopv1.TxContextThreadingModePerQueue)
	default:
		// Weak enum: single digits 1-9 are accepted by the validation rule.
		if len(raw) == 1 && raw[0] >= '1' && raw[0] <= '9' {
			return raw
		}
		return ""
	}
}

// decodeCoalescingScheme translates vSphere's lowercase coalescing scheme
// strings to the spec constant.
//
// vSphere stores: "disabled", "adapt", "static", "rbc".
// Unknown strings shorter than CoalescingSchemeMaxLen characters are passed
// through as weak-enum raw values. Longer strings return "" (skip).
//
// See vmopv1.CoalescingScheme XValidation rule for the accepted value set.
func decodeCoalescingScheme(raw string) string {
	switch strings.ToLower(raw) {
	case "disabled":
		return string(vmopv1.CoalescingSchemeDisabled)
	case "adapt":
		return string(vmopv1.CoalescingSchemeAdapt)
	case "static":
		return string(vmopv1.CoalescingSchemeStatic)
	case "rbc":
		return string(vmopv1.CoalescingSchemeRateBasedCoalescing)
	default:
		// Weak enum: any string < vmopv1.CoalescingSchemeMaxLen chars is
		// valid per the XValidation rule.
		if len(raw) < vmopv1.CoalescingSchemeMaxLen {
			return raw
		}
		return ""
	}
}

// decodePNICQueueFeatures decodes a vSphere pnicfeatures bitmask decimal string
// into a slice of PNICQueueFeature elements.
//
// Each set bit in the integer maps to one element:
//   - Bit 0 (1)  → PNICQueueFeatureLargeReceiveOffload
//   - Bit 2 (4)  → PNICQueueFeatureReceiveSideScaling
//   - Other bits → decimal string of the power of 2 (weak enum passthrough)
//
// Only bits 0-(PNICFeaturesMaxItems-1) are checked (at most PNICFeaturesMaxItems
// elements, matching the MaxItems constraint on []PNICQueueFeature fields).
// Bits beyond bit 15 are silently dropped. An input of "0" or any non-integer
// returns nil so the slice is left unset.
//
// See vmopv1.PNICQueueFeature XValidation rule for the accepted element value set.
func decodePNICQueueFeatures(raw string) []string {
	n, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || n == 0 {
		return nil
	}

	// Only consider bits 0-(PNICFeaturesMaxItems-1).
	n &= (1 << vmopv1.PNICFeaturesMaxItems) - 1

	if n == 0 {
		return nil
	}

	var elems []string
	for bit := uint(0); bit < vmopv1.PNICFeaturesMaxItems; bit++ {
		if n&(1<<bit) == 0 {
			continue
		}
		power := uint64(1) << bit
		switch power {
		case 1:
			elems = append(elems, string(vmopv1.PNICQueueFeatureLargeReceiveOffload))
		case 4:
			elems = append(elems, string(vmopv1.PNICQueueFeatureReceiveSideScaling))
		default:
			elems = append(elems, strconv.FormatUint(power, 10))
		}
	}

	return elems
}

// decodeUDPRSSMode decodes the vSphere integer encoding of UDP RSS mode.
//
// vSphere stores "1" for enabled and "2" for disabled. This differs from the
// standard VMX boolean format (TRUE/FALSE/1/0) used by other fields. Any value
// other than "1" or "2" — including host-default sentinels — leaves the field
// nil (hypervisor decides).
//
// See vmopv1.UDPRSSMode for the type declaration and wire-format note.
func decodeUDPRSSMode(raw string) *bool {
	switch raw {
	case "1":
		v := true
		return &v
	case "2":
		v := false
		return &v
	default:
		return nil
	}
}
