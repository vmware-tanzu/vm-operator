package util

import "reflect"

// IsNil returns true if v is nil, including a nil check against possible
// interface data.
func IsNil(v any) bool {
	if v == nil {
		return true
	}
	if vv := reflect.ValueOf(v); vv.Kind() == reflect.Ptr {
		return vv.IsNil()
	}
	return false
}
