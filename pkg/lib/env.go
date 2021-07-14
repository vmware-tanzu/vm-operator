// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package lib

import (
	"fmt"
	"os"
	"strconv"
)

const (
	trueString                    = "true"
	TrueString                    = trueString
	VmopNamespaceEnv              = "POD_NAMESPACE"
	WcpFaultDomainsFSS            = "FSS_WCP_FAULTDOMAINS"
	VMServiceFSS                  = "FSS_WCP_VMSERVICE"
	VMServiceV1Alpha2FSS          = "FSS_WCP_VMSERVICE_V1ALPHA2"
	ThunderPciDevicesFSS          = "FSS_THUNDERPCIDEVICES"
	MaxCreateVMsOnProviderEnv     = "MAX_CREATE_VMS_ON_PROVIDER"
	DefaultMaxCreateVMsOnProvider = 80
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

var IsWcpFaultDomainsFSSEnabled = func() bool {
	return os.Getenv(WcpFaultDomainsFSS) == trueString
}

var IsVMServiceFSSEnabled = func() bool {
	return os.Getenv(VMServiceFSS) == trueString
}

var IsVMServiceV1Alpha2FSSEnabled = func() bool {
	return os.Getenv(VMServiceV1Alpha2FSS) == trueString
}

var IsThunderPciDevicesFSSEnabled = func() bool {
	return os.Getenv(ThunderPciDevicesFSS) == trueString
}

// MaxAllowedCreateVMsOnProvider returns the percentage of reconciler threads that can be used to create VMs on the provider
// concurrently. The default is 80.
// TODO: Remove the env lookup once we have tuned this value from system tests.
var MaxConcurrentCreateVMsOnProvider = func() int {
	v := os.Getenv(MaxCreateVMsOnProviderEnv)
	if v == "" {
		return DefaultMaxCreateVMsOnProvider
	}

	// Return default in case of an invalid value.
	val, err := strconv.Atoi(v)
	if err != nil {
		return DefaultMaxCreateVMsOnProvider
	}

	return val
}
