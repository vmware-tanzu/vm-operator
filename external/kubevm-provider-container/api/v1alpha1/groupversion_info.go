// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// Package v1alpha1 contains the container provider's API types for KubeVM.
//
// This provider joins the shared infrastructure.kube-vm.io group used by
// every KubeVM provider (see external/kubevm-provider-aws), rather than
// minting infrastructure.container.kube-vm.io: the group names the contract
// role, and the Kind (ContainerMachine) names the provider.
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
