// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// Package install installs the API group, making it available as
// an option to all of the API encoding/decoding machinery.
package install

import (
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	vmopv1a1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
)

// Install registers the API group and adds types to a scheme.
func Install(scheme *runtime.Scheme) {
	utilruntime.Must(vmopv1a1.AddToScheme(scheme))
}
