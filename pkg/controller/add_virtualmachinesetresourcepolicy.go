// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package controller

import virtualmachinesetresourcepolicy "github.com/vmware-tanzu/vm-operator/pkg/controller/virtualmachinesetresourcepolicy"

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, virtualmachinesetresourcepolicy.Add)
}
