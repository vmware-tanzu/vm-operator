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
	VMServiceFSS     = "FSS_WCP_VMSERVICE"
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

var IsVMServiceFSSEnabled = func() bool {
	return os.Getenv("FSS_WCP_VMSERVICE") == "true"
}

var IsT1PerNamespaceEnabled = func() bool {
	return os.Getenv("FSS_WCP_T1_PERNAMESPACE") == "true"
}

// EnableVMServiceFSS enables the VM service FSS. Currently, only used in tests.
func EnableVMServiceFSS() {
	os.Setenv(VMServiceFSS, "true")
}

// EnableVMServiceFSS disables the VM service FSS. Currently, only used in tests.
func DisableVMServiceFSS() {
	os.Unsetenv(VMServiceFSS)
}
