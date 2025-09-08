// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// Package main is a stub module whose imports determine which API packages are
// vendored and therefore included in the generated CRD reference documentation.
package main

import (
	_ "github.com/vmware-tanzu/vm-operator/api/v1alpha1"

	_ "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	_ "github.com/vmware-tanzu/vm-operator/api/v1alpha2/cloudinit"
	_ "github.com/vmware-tanzu/vm-operator/api/v1alpha2/common"
	_ "github.com/vmware-tanzu/vm-operator/api/v1alpha2/sysprep"

	_ "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	_ "github.com/vmware-tanzu/vm-operator/api/v1alpha3/cloudinit"
	_ "github.com/vmware-tanzu/vm-operator/api/v1alpha3/common"
	_ "github.com/vmware-tanzu/vm-operator/api/v1alpha3/sysprep"

	_ "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	_ "github.com/vmware-tanzu/vm-operator/api/v1alpha4/cloudinit"
	_ "github.com/vmware-tanzu/vm-operator/api/v1alpha4/common"
	_ "github.com/vmware-tanzu/vm-operator/api/v1alpha4/sysprep"

	_ "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	_ "github.com/vmware-tanzu/vm-operator/api/v1alpha5/cloudinit"
	_ "github.com/vmware-tanzu/vm-operator/api/v1alpha5/common"
	_ "github.com/vmware-tanzu/vm-operator/api/v1alpha5/sysprep"

	_ "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	_ "github.com/vmware-tanzu/vm-operator/api/v1alpha6/cloudinit"
	_ "github.com/vmware-tanzu/vm-operator/api/v1alpha6/common"
	_ "github.com/vmware-tanzu/vm-operator/api/v1alpha6/sysprep"
)

func main() {}
