/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package controller

import (
	"github.com/vmware-tanzu/vm-operator/pkg/controller/virtualmachineservice"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, virtualmachineservice.Add)
}
