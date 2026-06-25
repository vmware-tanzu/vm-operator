// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package configpolicy_test

import (
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("VirtualMachineConfigOptions", func() {
	// TODO(vmop-3763): Implement once the full pipeline
	// (Zone → ConfigTarget → VirtualMachineConfigOptions) is in place.

	PIt("status is populated after ConfigTarget is reconciled", func() {
	})

	PIt("VirtualMachineGuestOptions are created for each guest OS", func() {
	})
})
