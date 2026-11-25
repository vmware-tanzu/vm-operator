// Copyright (c) 2025 Broadcom. All Rights Reserved.

// Package v1alpha2 contains API Schema definitions for the mobility-operator v1alpha2 API group
// +kubebuilder:object:generate=true
// +groupName=mobility-operator.vmware.com
package v1alpha2

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

const (
	// Version is the API Version.
	Version = "v1alpha2"

	// GroupName is the name of the API group.
	GroupName = "mobility-operator.vmware.com"
)

var (
	// GroupVersion is group version used to register these objects.
	GroupVersion = schema.GroupVersion{Group: GroupName, Version: Version}

	// SchemeBuilder is used to add go types to the GroupVersionKind scheme.
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)
