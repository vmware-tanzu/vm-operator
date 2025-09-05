// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha5

import (
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NamespacedName returns an object's namespaced name in the following format:
//
//	(NAMESPACE)?/?(NAME)?
//
// Both the namespace and name components are optional. Please refer to the
// examples for the various output formats.
func NamespacedName(obj metav1.Object) string {
	if obj == nil {
		return ""
	}
	if v := reflect.ValueOf(obj); v.Kind() == reflect.Ptr && v.IsNil() {
		return ""
	}

	var (
		namespace = obj.GetNamespace()
		name      = obj.GetName()
	)

	switch {
	case namespace != "" && name != "":
		return namespace + "/" + name

	case namespace != "":
		// Distinguish a namespace sans name with a trailing slash.
		return namespace + "/"

	case name != "":
		// Distinguish a name sans namespace with a preceding slash.
		return "/" + name

	default:
		return ""
	}
}
