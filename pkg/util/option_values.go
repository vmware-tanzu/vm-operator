// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"fmt"
	"reflect"
	"slices"

	vimtypes "github.com/vmware/govmomi/vim25/types"
)

// OptionValues simplifies manipulation of properties that are arrays of
// vimtypes.BaseOptionValue, such as ExtraConfig.
type OptionValues []vimtypes.BaseOptionValue

// OptionValuesFromMap returns a new OptionValues object from the provided map.
func OptionValuesFromMap[T any](in map[string]T) OptionValues {
	if len(in) == 0 {
		return nil
	}
	var (
		i   int
		out = make(OptionValues, len(in))
	)
	for k, v := range in {
		out[i] = &vimtypes.OptionValue{Key: k, Value: v}
		i++
	}
	return out
}

// Delete removes elements with the given key, returning the new list.
func (ov OptionValues) Delete(key string) OptionValues {
	return slices.DeleteFunc(ov, func(optVal vimtypes.BaseOptionValue) bool {
		return optVal.GetOptionValue() != nil && optVal.GetOptionValue().Key == key
	})
}

// Get returns the value if exists, otherwise nil is returned. The second return
// value is a flag indicating whether the value exists or nil was the actual
// value.
func (ov OptionValues) Get(key string) (any, bool) {
	if ov == nil {
		return nil, false
	}
	for i := range ov {
		if optVal := ov[i].GetOptionValue(); optVal != nil {
			if optVal.Key == key {
				return optVal.Value, true
			}
		}
	}
	return nil, false
}

// GetString returns the value as a string if the value exists.
func (ov OptionValues) GetString(key string) (string, bool) {
	if ov == nil {
		return "", false
	}
	for i := range ov {
		if optVal := ov[i].GetOptionValue(); optVal != nil {
			if optVal.Key == key {
				return getOptionValueAsString(optVal.Value), true
			}
		}
	}
	return "", false
}

// Additions returns a diff that includes only the elements from the provided
// list that do not already exist.
func (ov OptionValues) Additions(in ...vimtypes.BaseOptionValue) OptionValues {
	return ov.diff(in, true)
}

// Diff returns a diff that includes the elements from the provided list that do
// not already exist or have different values.
func (ov OptionValues) Diff(in ...vimtypes.BaseOptionValue) OptionValues {
	return ov.diff(in, false)
}

func (ov OptionValues) diff(in OptionValues, addOnly bool) OptionValues {
	if ov == nil && in == nil {
		return nil
	}
	var (
		out         OptionValues
		leftOptVals = ov.Map()
	)
	for i := range in {
		if rightOptVal := in[i].GetOptionValue(); rightOptVal != nil {
			k, v := rightOptVal.Key, rightOptVal.Value
			if ov == nil {
				out = append(out, &vimtypes.OptionValue{Key: k, Value: v})
			} else if leftOptVal, ok := leftOptVals[k]; !ok {
				out = append(out, &vimtypes.OptionValue{Key: k, Value: v})
			} else if !addOnly && v != leftOptVal {
				out = append(out, &vimtypes.OptionValue{Key: k, Value: v})
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// Merge adds the provided item(s), overwriting any existing values.
func (ov OptionValues) Merge(in ...vimtypes.BaseOptionValue) OptionValues {
	return ov.merge(in, true)
}

// Append adds the provided item(s) as long as a key does not conflict with an
// existing item.
func (ov OptionValues) Append(in ...vimtypes.BaseOptionValue) OptionValues {
	return ov.merge(in, false)
}

func (ov OptionValues) merge(in OptionValues, overwrite bool) OptionValues {

	var (
		out        OptionValues
		outOptVals = map[string]*vimtypes.OptionValue{}
	)

	if len(in) == 0 {
		return ov
	}

	// Init the out slice from the left side.
	if len(ov) > 0 {
		for i := range ov {
			if optVal := ov[i].GetOptionValue(); optVal != nil {
				kv := &vimtypes.OptionValue{Key: optVal.Key, Value: optVal.Value}
				out = append(out, kv)
				outOptVals[optVal.Key] = kv
			}
		}
	}

	// Merge or append the right side.
	for i := range in {
		if rightOptVal := in[i].GetOptionValue(); rightOptVal != nil {
			k, v := rightOptVal.Key, rightOptVal.Value
			if outOptVal, ok := outOptVals[k]; !ok {
				out = append(out, &vimtypes.OptionValue{Key: k, Value: v})
			} else if overwrite {
				outOptVal.Value = v
			}
		}
	}

	if len(out) == 0 {
		return nil
	}

	return out
}

// Map returns the list of option values as a map.
func (ov OptionValues) Map() map[string]any {
	if len(ov) == 0 {
		return nil
	}
	out := map[string]any{}
	for i := range ov {
		if optVal := ov[i].GetOptionValue(); optVal != nil {
			out[optVal.Key] = optVal.Value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// StringMap returns the list of option values as a map where the values are
// strings.
func (ov OptionValues) StringMap() map[string]string {
	if len(ov) == 0 {
		return nil
	}
	out := map[string]string{}
	for i := range ov {
		if optVal := ov[i].GetOptionValue(); optVal != nil {
			out[optVal.Key] = getOptionValueAsString(optVal.Value)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func getOptionValueAsString(val any) string {
	switch tval := val.(type) {
	case string:
		return tval
	default:
		if rv := reflect.ValueOf(val); rv.Kind() == reflect.Pointer {
			if rv.IsNil() {
				return ""
			}
			return fmt.Sprintf("%v", rv.Elem().Interface())
		}
		return fmt.Sprintf("%v", tval)
	}
}
