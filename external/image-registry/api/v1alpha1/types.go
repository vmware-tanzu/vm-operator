// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

// NameAndKindRef describes a reference to another object in the same
// namespace as the referrer. The reference can be just a name but may also
// include the referred resource's Kind.
type NameAndKindRef struct {
	// Kind is a string value representing the kind of resource to which this
	// object refers.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
	// +optional
	Kind string `json:"kind,omitempty"`

	// Name refers to a unique resource in the current namespace.
	// More info: http://kubernetes.io/docs/user-guide/identifiers#names
	Name string `json:"name"`
}
