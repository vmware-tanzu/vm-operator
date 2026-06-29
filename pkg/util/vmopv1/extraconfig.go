// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/go-logr/logr"

	"k8s.io/apimachinery/pkg/util/sets"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	vsphereconst "github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
)

// EthernetDeviceKeyBase is the vSphere device key base offset for ethernet
// (VirtualEthernetCard) devices. The ethernetX index used in VMX ExtraConfig
// keys (e.g. "ethernet1.ctxPerDev") equals deviceKey - EthernetDeviceKeyBase.
const EthernetDeviceKeyBase int32 = 4000

// ethernetVMXKeyPrefix is the printf-style format string used in vmx struct tags
// for per-NIC tuning fields (e.g. `vmx:"ethernet%d.ctxPerDev"`).
const ethernetVMXKeyPrefix = "ethernet%d"

// EthernetExtraConfigPrefix returns the "ethernetX." prefix for VMX ExtraConfig
// keys corresponding to the ethernet device with the given device key.
func EthernetExtraConfigPrefix(deviceKey int32) string {
	return fmt.Sprintf(ethernetVMXKeyPrefix+".", deviceKey-EthernetDeviceKeyBase)
}

// ethernetDeviceVMXRE matches a vmx struct tag of the form "ethernet%d.<key>"
// and captures the bare key portion in submatch[1]. Used only for struct-tag
// reflection (buildDeviceTagMaps). Does NOT match live ExtraConfig keys.
var ethernetDeviceVMXRE = regexp.MustCompile(`^` + ethernetVMXKeyPrefix + `\.(.*)$`)

// ethernetLiveKeyRE matches a live VMX ExtraConfig key of the form "ethernetN.<key>"
// (where N is a decimal device index) and captures the bare key in submatch[1].
var ethernetLiveKeyRE = regexp.MustCompile(`^ethernet\d+\.(.*)$`)

// VMXMode describes how a change to a first-class VMX key is applied.
// It is declared by the vmxmode struct tag on VirtualMachineAdvancedSpec fields.
type VMXMode int

const (
	// VMXModePowerCycle is the zero value and the default when no vmxmode tag
	// is present. ReconfigVM_Task succeeds on a powered-on VM, but the change
	// takes effect only after the guest power-cycles.
	VMXModePowerCycle VMXMode = iota
	// VMXModeLive means ReconfigVM_Task applies the change immediately,
	// regardless of power state (ESXi hot-add capable).
	VMXModeLive
	// VMXModePowerOff means ReconfigVM_Task fails on a powered-on VM.
	// The change must be deferred until the VM is powered off.
	VMXModePowerOff
)

// vmxTagMaps holds both key→fieldIdx and key→VMXMode maps for any VMX spec
// struct. Used for global VM fields (VirtualMachineAdvancedSpec) and
// device-scoped fields (e.g. ethernet%d, passthru%d).
type vmxTagMaps struct {
	keys  map[string]int
	modes map[string]VMXMode
}

var (
	cachedAdvancedVMXMaps = sync.OnceValue(buildAdvancedVMXMaps[vmopv1.VirtualMachineAdvancedSpec])
	cachedVMXNet3NICMaps  = sync.OnceValue(buildVMXNet3NICMaps[vmopv1.VirtualMachineNetworkInterfaceVMXNet3Spec])

	// cachedSortedAdvancedVMXKeys is the sorted list of VMX keys mapped to
	// first-class VirtualMachineAdvancedSpec fields. Precomputed once since
	// callers (e.g. status reconciliation) rebuild it on every call otherwise.
	cachedSortedAdvancedVMXKeys = sync.OnceValue(func() []string {
		return sets.List(sets.KeySet(AdvancedVMXKeyMap()))
	})
)

// AdvancedVMXKeyMap returns the shared, lazily-built map of vmx struct tags
// to field indices for VirtualMachineAdvancedSpec.
func AdvancedVMXKeyMap() map[string]int {
	return cachedAdvancedVMXMaps().keys
}

// SortedAdvancedVMXKeys returns the shared, lazily-built, sorted list of vmx
// struct tags for VirtualMachineAdvancedSpec (the keys of AdvancedVMXKeyMap).
func SortedAdvancedVMXKeys() []string {
	return cachedSortedAdvancedVMXKeys()
}

// AdvancedVMXModeMap returns the shared, lazily-built map of vmx tag value →
// VMXMode for VirtualMachineAdvancedSpec.
func AdvancedVMXModeMap() map[string]VMXMode {
	return cachedAdvancedVMXMaps().modes
}

// VMXNet3NICKeyMap returns the shared, lazily-built map of template VMX keys
// (e.g. "ethernet%d.ctxPerDev") to field indices for
// VirtualMachineNetworkInterfaceVMXNet3Spec.
func VMXNet3NICKeyMap() map[string]int {
	return cachedVMXNet3NICMaps().keys
}

// VMXNet3NICModeMap returns the shared, lazily-built map of template VMX keys
// (e.g. "ethernet%d.ctxPerDev") to VMXMode for
// VirtualMachineNetworkInterfaceVMXNet3Spec.
func VMXNet3NICModeMap() map[string]VMXMode {
	return cachedVMXNet3NICMaps().modes
}

// isDeviceProperty reports whether tag matches the device-namespace VMX pattern
// described by re (e.g. ethernetDeviceVMXRE for "ethernet%d.<key>").
func isDeviceProperty(tag string, re *regexp.Regexp) bool {
	return re.MatchString(tag)
}

// extractDeviceProperty strips the device-namespace prefix from a full VMX key
// using re, returning the bare property name and true.
// Returns ("", false) when the key does not match the pattern.
func extractDeviceProperty(key string, re *regexp.Regexp) (string, bool) {
	sub := re.FindStringSubmatch(key)
	if len(sub) != 2 {
		return "", false
	}
	return sub[1], true
}

// ExtractEthernetDeviceProperty strips the "ethernetN." namespace prefix from
// a live VMX ExtraConfig key (e.g. "ethernet0.ctxPerDev"), returning the bare
// property name and true. Returns ("", false) when the key does not match.
func ExtractEthernetDeviceProperty(key string) (string, bool) {
	return extractDeviceProperty(key, ethernetLiveKeyRE)
}

// IsEthernetDeviceKey reports whether key is a live VMX ExtraConfig key in the
// ethernetN namespace (e.g. "ethernet0.ctxPerDev").
func IsEthernetDeviceKey(key string) bool {
	return ethernetLiveKeyRE.MatchString(key)
}

// EthernetTemplateKey returns the template-form VMX key for a bare NIC property
// name (e.g. "ctxPerDev" → "ethernet%d.ctxPerDev"). This is the canonical form
// used as map keys by VMXNet3NICKeyMap and VMXNet3NICModeMap.
func EthernetTemplateKey(bareKey string) string {
	return ethernetVMXKeyPrefix + "." + bareKey
}

// NormalizeEthernetDeviceKey converts a live VMX ExtraConfig key to its
// template form for map lookup. For example "ethernet0.ctxPerDev" →
// "ethernet%d.ctxPerDev". Returns the key unchanged when it does not match
// the live ethernet-device pattern.
func NormalizeEthernetDeviceKey(key string) string {
	if bare, ok := extractDeviceProperty(key, ethernetLiveKeyRE); ok {
		return ethernetVMXKeyPrefix + "." + bare
	}
	return key
}

// IsFirstClassVMXnet3NICKey reports whether key refers to a first-class VMXNet3 NIC
// property managed via spec.network.interfaces[].vmxnet3. It accepts both the
// bare key form ("ctxPerDev") and the live prefixed form ("ethernet0.ctxPerDev").
func IsFirstClassVMXnet3NICKey(key string) bool {
	m := VMXNet3NICKeyMap()
	// Strip "ethernetN." prefix if present; otherwise treat key as bare.
	bare, ok := extractDeviceProperty(key, ethernetLiveKeyRE)
	if !ok {
		bare = key
	}
	_, found := m[EthernetTemplateKey(bare)]
	return found
}

// buildVMXTagMaps scans struct type T in a single reflection pass and builds
// both tag→fieldIdx and tag→VMXMode maps. include decides whether a vmx tag
// should be indexed; the tag itself is used as the map key. This shared builder
// is used for both global VM fields and device-scoped field sets.
func buildVMXTagMaps[T any](include func(tag string) bool) vmxTagMaps {
	t := reflect.TypeOf(*new(T))
	keys := make(map[string]int, t.NumField())
	modes := make(map[string]VMXMode, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("vmx")
		if !include(tag) {
			continue
		}
		keys[tag] = i
		switch f.Tag.Get("vmxmode") {
		case "live":
			modes[tag] = VMXModeLive
		case "poweroff":
			modes[tag] = VMXModePowerOff
		default:
			modes[tag] = VMXModePowerCycle
		}
	}
	return vmxTagMaps{keys: keys, modes: modes}
}

// buildAdvancedVMXMaps builds maps for global VM fields in T.
// Fields without a vmx tag, or with a device-scoped tag (e.g. ethernet%d.*), are skipped.
func buildAdvancedVMXMaps[T any]() vmxTagMaps {
	return buildVMXTagMaps[T](func(tag string) bool {
		return tag != "" && !isDeviceProperty(tag, ethernetDeviceVMXRE)
	})
}

// buildDeviceTagMaps builds maps for a device-scoped struct type T. re selects
// which fields belong to this device namespace (e.g. ethernetDeviceVMXRE for
// ethernet%d fields). The full vmx tag (e.g. "ethernet%d.ctxPerDev") is used
// as the map key, preserving the template placeholder for runtime formatting.
func buildDeviceTagMaps[T any](re *regexp.Regexp) vmxTagMaps {
	return buildVMXTagMaps[T](func(tag string) bool {
		return isDeviceProperty(tag, re)
	})
}

// buildVMXNet3NICMaps returns both maps for T using the ethernet%d namespace.
func buildVMXNet3NICMaps[T any]() vmxTagMaps {
	return buildDeviceTagMaps[T](ethernetDeviceVMXRE)
}

// vmxStringDecoder translates a raw vSphere string value to the canonical spec
// value for a named string type (e.g. TxContextThreadingMode).
//
// Returns the canonical value to store, or "" to skip (invalid per the type's
// kubebuilder validation rule). A decoder should never return "" for a valid
// passthrough value since empty string is not a legal enum constant.
type vmxStringDecoder func(raw string) string

// vmxStringEncoder translates a spec string value to its vSphere wire format
// for a named string type that uses a non-identity wire encoding
// (e.g. TxContextThreadingMode whose wire format is "1"/"2"/"3").
//
// Returns the wire-format value to send to vSphere. Unknown / weak-enum
// passthrough values should be returned as-is.
// Every registered vmxStringDecoder must have a corresponding vmxStringEncoder.
type vmxStringEncoder func(val string) string

// vmxSliceDecoder decodes a raw vSphere string value into a slice of strings
// for a named string slice element type (e.g. PNICQueueFeature).
//
// Returns the decoded elements, or nil to skip (value is invalid or produces
// no elements).
type vmxSliceDecoder func(raw string) []string

// vmxSliceEncoder encodes a slice of spec string values to its vSphere wire
// format for a named string slice element type that uses a non-identity
// wire encoding (e.g. PNICQueueFeature whose wire format is a bitmask integer).
//
// Every registered vmxSliceDecoder must have a corresponding vmxSliceEncoder.
type vmxSliceEncoder func(vals []string) string

// vmxBoolDecoder decodes a raw vSphere string value into a *bool for a named
// bool type (e.g. UDPRSSMode) that uses a non-standard wire encoding.
//
// Returns:
//   - &true / &false to set the pointer value.
//   - nil to leave the pointer nil (auto sentinel or invalid input).
type vmxBoolDecoder func(raw string) *bool

// vmxBoolEncoder encodes a spec bool value to its vSphere wire format for a
// named bool type that uses a non-standard wire encoding
// (e.g. UDPRSSMode whose wire format is "1"=enabled / "2"=disabled).
//
// Every registered vmxBoolDecoder must have a corresponding vmxBoolEncoder.
type vmxBoolEncoder func(val bool) string

// vmxStringDecoders maps a named string element type to its decoder.
// Populated once by init().
var vmxStringDecoders map[reflect.Type]vmxStringDecoder

// vmxStringEncoders maps a named string element type to its encoder (inverse
// of vmxStringDecoders). Populated once by init().
var vmxStringEncoders map[reflect.Type]vmxStringEncoder

// vmxSliceDecoders maps a named string slice element type to its decoder.
// Populated once by init().
var vmxSliceDecoders map[reflect.Type]vmxSliceDecoder

// vmxSliceEncoders maps a named string slice element type to its encoder
// (inverse of vmxSliceDecoders). Populated once by init().
var vmxSliceEncoders map[reflect.Type]vmxSliceEncoder

// vmxBoolDecoders maps a named bool element type to its decoder.
// Populated once by init().
var vmxBoolDecoders map[reflect.Type]vmxBoolDecoder

// vmxBoolEncoders maps a named bool element type to its encoder (inverse
// of vmxBoolDecoders). Populated once by init().
var vmxBoolEncoders map[reflect.Type]vmxBoolEncoder

func init() {
	vmxStringDecoders = map[reflect.Type]vmxStringDecoder{
		reflect.TypeOf(vmopv1.TxContextThreadingMode("")): decodeTxContextThreadingMode,
		reflect.TypeOf(vmopv1.CoalescingScheme("")):       decodeCoalescingScheme,
	}
	vmxStringEncoders = map[reflect.Type]vmxStringEncoder{
		reflect.TypeOf(vmopv1.TxContextThreadingMode("")): encodeTxContextThreadingMode,
		reflect.TypeOf(vmopv1.CoalescingScheme("")):       encodeCoalescingScheme,
	}
	vmxSliceDecoders = map[reflect.Type]vmxSliceDecoder{
		reflect.TypeOf(vmopv1.PNICQueueFeature("")): decodePNICQueueFeatures,
	}
	vmxSliceEncoders = map[reflect.Type]vmxSliceEncoder{
		reflect.TypeOf(vmopv1.PNICQueueFeature("")): encodePNICQueueFeatures,
	}
	vmxBoolDecoders = map[reflect.Type]vmxBoolDecoder{
		reflect.TypeOf(vmopv1.UDPRSSMode(false)): decodeUDPRSSMode,
	}
	vmxBoolEncoders = map[reflect.Type]vmxBoolEncoder{
		reflect.TypeOf(vmopv1.UDPRSSMode(false)): encodeUDPRSSMode,
	}
}

// EncodeVMXStringField converts a spec string value to its vSphere wire format.
// For types with a registered encoder the encoded value is returned; for all
// other types the value is returned unchanged.
func EncodeVMXStringField(elemType reflect.Type, val string) string {
	if enc, ok := vmxStringEncoders[elemType]; ok {
		return enc(val)
	}
	return val
}

// EncodeVMXBoolField converts a spec bool value to its vSphere wire format.
// For types with a registered encoder (e.g. UDPRSSMode uses "1"/"2") the
// encoded value is returned; for plain bool types "TRUE"/"FALSE" is returned.
func EncodeVMXBoolField(elemType reflect.Type, val bool) string {
	if enc, ok := vmxBoolEncoders[elemType]; ok {
		return enc(val)
	}
	if val {
		return "TRUE"
	}
	return "FALSE"
}

// EncodeVMXSliceStringField converts a spec slice of string values to its
// vSphere wire format. For types with a registered encoder (e.g. PNICQueueFeature
// uses a bitmask integer) the encoded value is returned; for unregistered types
// the values are joined with commas.
func EncodeVMXSliceStringField(elemType reflect.Type, vals []string) string {
	if enc, ok := vmxSliceEncoders[elemType]; ok {
		return enc(vals)
	}
	return strings.Join(vals, ",")
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
		// No registered decoder: split by comma to match the comma-join encoding.
		parts := strings.Split(raw, ",")
		slice := reflect.MakeSlice(rv.Type(), len(parts), len(parts))
		for i, p := range parts {
			slice.Index(i).SetString(strings.TrimSpace(p))
		}
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

// encodeTxContextThreadingMode translates the spec constant to vSphere's
// integer wire format. Weak-enum passthrough values (single digits) are
// returned unchanged.
func encodeTxContextThreadingMode(val string) string {
	switch val {
	case string(vmopv1.TxContextThreadingModePerDevice):
		return "1"
	case string(vmopv1.TxContextThreadingModePerVM):
		return "2"
	case string(vmopv1.TxContextThreadingModePerQueue):
		return "3"
	default:
		return val
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

// encodeCoalescingScheme translates the spec constant to vSphere's lowercase
// wire format. Weak-enum passthrough values are returned unchanged.
func encodeCoalescingScheme(val string) string {
	switch val {
	case string(vmopv1.CoalescingSchemeDisabled):
		return "disabled"
	case string(vmopv1.CoalescingSchemeAdapt):
		return "adapt"
	case string(vmopv1.CoalescingSchemeStatic):
		return "static"
	case string(vmopv1.CoalescingSchemeRateBasedCoalescing):
		return "rbc"
	default:
		return val
	}
}

// decodePNICQueueFeatures decodes a vSphere pnicFeatures bitmask decimal string
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

// encodePNICQueueFeatures encodes a slice of PNICQueueFeature spec values to
// vSphere's bitmask decimal integer wire format.
//
// Known features:
//   - PNICQueueFeatureLargeReceiveOffload → bit 0 (value 1)
//   - PNICQueueFeatureReceiveSideScaling  → bit 2 (value 4)
//
// Weak-enum passthrough values (decimal strings of powers of 2) are parsed and
// added to the bitmask. Returns "" if the resulting bitmask is zero.
func encodePNICQueueFeatures(vals []string) string {
	var n uint64
	for _, v := range vals {
		switch v {
		case string(vmopv1.PNICQueueFeatureLargeReceiveOffload):
			n |= 1
		case string(vmopv1.PNICQueueFeatureReceiveSideScaling):
			n |= 4
		default:
			// Weak enum: decimal string of a power of 2.
			power, err := strconv.ParseUint(v, 10, 64)
			if err == nil {
				n |= power
			}
		}
	}
	n &= (1 << vmopv1.PNICFeaturesMaxItems) - 1
	if n == 0 {
		return ""
	}
	return strconv.FormatUint(n, 10)
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

// encodeUDPRSSMode encodes a spec bool to vSphere's integer wire format:
// true (enabled) → "1", false (disabled) → "2".
func encodeUDPRSSMode(val bool) string {
	if val {
		return "1"
	}
	return "2"
}

// systemReservedExtraConfigKeys is the set of exact ExtraConfig keys managed
// internally by vm-operator and not settable by users via spec.advanced.extraConfig.
var systemReservedExtraConfigKeys = map[string]bool{
	vsphereconst.EnableDiskUUIDExtraConfigKey:              true,
	vsphereconst.ExtraConfigReservedKeyVMXRebootPowerCycle: true,
	vsphereconst.ExtraConfigReservedProfileID:              true,
	vsphereconst.ExtraConfigRunContainerKey:                true,
	vsphereconst.ExtraConfigVMServiceNamespacedName:        true,
	vsphereconst.GOSCPendingExtraConfigKey:                 true,
	vsphereconst.GOSCIgnoreToolsCheckExtraConfigKey:        true,
	vsphereconst.MMPowerOffVMExtraConfigKey:                true,
	vsphereconst.PCIPassthruMMIOExtraConfigKey:             true,
	vsphereconst.PCIPassthruMMIOSizeExtraConfigKey:         true,
}

// IsSystemReservedExtraConfigKey reports whether key is reserved for internal
// vm-operator use and must not be set by users via spec.advanced.extraConfig.
// Keys matching the "vmservice." or "guestinfo." prefixes are also reserved.
func IsSystemReservedExtraConfigKey(key string) bool {
	if systemReservedExtraConfigKeys[key] {
		return true
	}
	return strings.HasPrefix(key, vsphereconst.ExtraConfigReservedPrefixVMService) ||
		strings.HasPrefix(key, vsphereconst.ExtraConfigGuestInfoPrefix)
}
