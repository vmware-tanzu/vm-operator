// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package extraconfig

import (
	"fmt"
	"reflect"
	"strings"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
)

// TranslateFirstClass returns VMX OptionValues for every first-class field of
// spec.advanced, including entries with empty-string values for nil/zero fields.
// An empty-string value signals that the key should be cleared on the VM if
// currently present. Returns nil if advanced is nil.
//
// Each non-zero field is converted to vSphere wire format via EncodeVMX*Field,
// which applies a registered custom encoder for the field's type when available,
// or falls back to the default encoding (*bool → "TRUE"/"FALSE", []int32 →
// comma-separated decimals, []string → comma-joined). nil/zero fields emit ""
// to signal clear-if-present to the caller.
func TranslateFirstClass(advanced *vmopv1.VirtualMachineAdvancedSpec) pkgutil.OptionValues {
	if advanced == nil {
		return nil
	}

	keyMap := vmopv1util.AdvancedVMXKeyMap()
	rv := reflect.ValueOf(advanced).Elem()
	out := make(pkgutil.OptionValues, 0, len(keyMap))

	for vmxKey, fieldIdx := range keyMap {
		fv := rv.Field(fieldIdx)
		val := translateFieldValue(fv)
		out = append(out, &vimtypes.OptionValue{Key: vmxKey, Value: val})
	}
	return out
}

// translateFieldValue converts a struct field value to its canonical VMX
// string representation, or "" to indicate the field should be omitted.
func translateFieldValue(fv reflect.Value) string {
	switch fv.Kind() {
	case reflect.Ptr:
		if fv.IsNil() {
			return ""
		}
		elem := fv.Elem()
		switch elem.Kind() {
		case reflect.Bool:
			return vmopv1util.EncodeVMXBoolField(elem.Type(), elem.Bool())
		case reflect.String:
			return vmopv1util.EncodeVMXStringField(elem.Type(), elem.String())
		}
	case reflect.Slice:
		if fv.IsNil() || fv.Len() == 0 {
			return ""
		}
		elem := fv.Type().Elem()
		switch elem.Kind() {
		case reflect.Int32:
			parts := make([]string, fv.Len())
			for i := range parts {
				parts[i] = fmt.Sprintf("%d", fv.Index(i).Int())
			}
			return strings.Join(parts, ",")
		case reflect.String:
			strs := make([]string, fv.Len())
			for i := range strs {
				strs[i] = fv.Index(i).String()
			}
			return vmopv1util.EncodeVMXSliceStringField(elem, strs)
		}
	}
	return ""
}
