/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package lib

import (
	"fmt"
	"os"
)

const (
	VmopNamespaceEnv = "POD_NAMESPACE"
)

// SetVmOpNamespaceEnv sets the VM Operator pod's namespace in the environment
func SetVmOpNamespaceEnv(namespace string) error {
	err := os.Setenv(VmopNamespaceEnv, namespace)
	if err != nil {
		return fmt.Errorf("failed to set env var: %v", err)
	}
	return nil
}

// GetVmOpNamespaceFromEnv resolves the VM Operator pod's namespace from the environment
func GetVmOpNamespaceFromEnv() (string, error) {
	vmopNamespace, vmopNamespaceExists := os.LookupEnv(VmopNamespaceEnv)
	if !vmopNamespaceExists {
		return "", fmt.Errorf("VM Operator namespace envvar %s is not set", VmopNamespaceEnv)
	}
	return vmopNamespace, nil
}
