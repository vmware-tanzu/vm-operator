// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package pkg

var (
	BuildCommit string
	BuildNumber string
	BuildType   string

	// BuildVersion is injected at build-time, but this default value is
	// assigned to ensure tests that rely on IsVirtualMachineSchemaUpgraded
	// function correctly.
	BuildVersion = "v0.0.0"
)
