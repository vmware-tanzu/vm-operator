// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
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

// DecodeVMXFieldValue decodes a raw vSphere ExtraConfig string value into rv.
// rv must be settable and must be of kind Ptr or Slice.
//
// Supported pointer element types: bool, string (and string-based named types).
// Supported slice element types: int32, string (and string-based named types).
//
// Bool decoding accepts "TRUE"/"FALSE" and "1"/"0" (case-insensitive).
// []int32 is decoded from a comma-separated list of decimal integers.
// []string-based slices are stored as a single-element slice containing raw.
func DecodeVMXFieldValue(rv reflect.Value, raw string) error {
	switch rv.Kind() {
	case reflect.Ptr:
		return decodePtrVMXField(rv, raw)
	case reflect.Slice:
		return decodeSliceVMXField(rv, raw)
	}
	return fmt.Errorf("unsupported field kind %v for vmx decode", rv.Kind())
}

func decodePtrVMXField(rv reflect.Value, raw string) error {
	elemType := rv.Type().Elem()
	newPtr := reflect.New(elemType)
	elem := newPtr.Elem()

	switch elemType.Kind() {
	case reflect.Bool:
		b, err := decodeBoolVMX(raw)
		if err != nil {
			return err
		}
		elem.SetBool(b)
	case reflect.String:
		elem.SetString(raw)
	default:
		return fmt.Errorf("unsupported pointer element kind %v for vmx decode", elemType.Kind())
	}

	rv.Set(newPtr)
	return nil
}

func decodeSliceVMXField(rv reflect.Value, raw string) error {
	elemType := rv.Type().Elem()

	switch elemType.Kind() {
	case reflect.Int32:
		parts := strings.Split(raw, ",")
		slice := reflect.MakeSlice(rv.Type(), len(parts), len(parts))
		for i, p := range parts {
			n, err := strconv.ParseInt(strings.TrimSpace(p), 10, 32)
			if err != nil {
				return fmt.Errorf("cannot parse %q as int32: %w", p, err)
			}
			slice.Index(i).SetInt(n)
		}
		rv.Set(slice)
	case reflect.String:
		// Store the raw value as a single-element slice.
		slice := reflect.MakeSlice(rv.Type(), 1, 1)
		slice.Index(0).SetString(raw)
		rv.Set(slice)
	default:
		return fmt.Errorf("unsupported slice element kind %v for vmx decode", elemType.Kind())
	}
	return nil
}

// decodeBoolVMX parses vSphere boolean ExtraConfig strings.
// Accepts "TRUE"/"FALSE" (vSphere convention) and "1"/"0" (case-insensitive).
func decodeBoolVMX(raw string) (bool, error) {
	switch strings.ToUpper(raw) {
	case "TRUE", "1":
		return true, nil
	case "FALSE", "0":
		return false, nil
	}
	return false, fmt.Errorf("cannot decode %q as vmx bool", raw)
}
