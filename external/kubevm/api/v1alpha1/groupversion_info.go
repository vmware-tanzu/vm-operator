// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// +kubebuilder:object:generate=true
// +groupName=kube-vm.io

// Package v1alpha1 contains the KubeVM generic, provider-agnostic virtual
// machine API.
//
// The API is deliberately portable: it carries the concepts that are common
// across hypervisors and cloud VM services, and binds to a specific backend
// through spec.infrastructureRef, which points at a provider-owned object.
// The generic layer observes that backend only through a duck-typed status
// contract (well-known field paths), so nothing here imports provider code.
package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	// GroupName is the API group for the generic VM API.
	//
	// Provider groups are expected to derive from this one, for example
	// infrastructure.vsphere.kube-vm.io.
	GroupName = "kube-vm.io"

	// GroupVersion is group version used to register these objects.
	GroupVersion = schema.GroupVersion{Group: GroupName, Version: "v1alpha1"}

	// SchemeBuilder is used to add go types to the GroupVersionKind scheme.
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)

// Resource takes an unqualified resource and returns a Group-qualified
// GroupResource.
func Resource(resource string) schema.GroupResource {
	return GroupVersion.WithResource(resource).GroupResource()
}
