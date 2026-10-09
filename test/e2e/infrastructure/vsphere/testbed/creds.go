// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package testbed

import (
	"os"
)

// Hopefully it's okay to put standard constants in the test framework.
// In the future, we may want the ability to override these constants for different environments.
// Of course, that begs the question of whether 'testbed' is even the right package for these constants.
var (
	GatewayUsername  = getFromEnvWithDefault("GATEWAY_VM_USERNAME", "root")
	GatewayPassword  = getFromEnvWithDefault("GATEWAY_VM_PASSWORD", "vmware")
	RootUsername     = getFromEnvWithDefault("SSH_USERNAME", "root")
	RootPassword     = getFromEnvWithDefault("SSH_PASSWORD", "vmware")
	AdminUsername    = getFromEnvWithDefault("VCSA_USERNAME", "Administrator@vsphere.local")
	AdminPassword    = getFromEnvWithDefault("VCSA_PASSWORD", "Admin!23")
	NSXAdminUsername = getFromEnvWithDefault("NSX_MANAGER_USERNAME", "admin")
	NSXAdminPassword = getFromEnvWithDefault("NSX_MANAGER_PASSWORD", "Admin!23")
)

func getFromEnvWithDefault(key, defaultVal string) string {
	val, ok := os.LookupEnv(key)
	if !ok {
		return defaultVal
	}

	return val
}
