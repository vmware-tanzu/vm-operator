// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// Package v1alpha1 contains the AWS provider's API types for KubeVM.
//
// The group mirrors Cluster API's shape: the core is kube-vm.io, providers are
// infrastructure.kube-vm.io, just as cluster.x-k8s.io pairs with
// infrastructure.cluster.x-k8s.io. The group names the contract role; the Kind
// names the provider, so a future AzureMachine joins this group rather than
// starting another.
//
// +kubebuilder:object:generate=true
// +groupName=infrastructure.kube-vm.io
package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

// GroupName is this provider's API group.
const GroupName = "infrastructure.kube-vm.io"

var (
	// GroupVersion is the group and version for these types.
	GroupVersion = schema.GroupVersion{
		Group:   GroupName,
		Version: "v1alpha1",
	}

	// SchemeBuilder registers these types with a scheme.
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

	// AddToScheme adds these types to a scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)
