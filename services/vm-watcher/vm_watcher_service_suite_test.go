// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmwatcher_test

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestVMWatcherService(t *testing.T) {
	RegisterFailHandler(Fail)
	// Match the integration test defaults in builder.TestSuite.Register.
	// The service waits five seconds before restarting after an error.
	SetDefaultEventuallyTimeout(10 * time.Second)
	SetDefaultEventuallyPollingInterval(100 * time.Millisecond)
	RunSpecs(t, "VM Watcher Service Test Suite")
}
