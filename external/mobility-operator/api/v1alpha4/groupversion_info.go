// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// Package v1alpha4 contains API Schema definitions for the mobility-operator v1alpha4 API group
// +kubebuilder:object:generate=true
// +groupName=mobility-operator.vmware.com
package v1alpha4

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	// Version is the API Version.
	Version = "v1alpha4"

	// GroupName is the name of the API group.
	GroupName = "mobility-operator.vmware.com"
)

var (
	// GroupVersion is group version used to register these objects.
	GroupVersion = schema.GroupVersion{Group: GroupName, Version: Version}

	// schemeBuilder is used to add go types to the GroupVersionKind scheme.
	schemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = schemeBuilder.AddToScheme

	objectTypes = []runtime.Object{}
)

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(GroupVersion, objectTypes...)
	metav1.AddToGroupVersion(scheme, GroupVersion)

	return nil
}
