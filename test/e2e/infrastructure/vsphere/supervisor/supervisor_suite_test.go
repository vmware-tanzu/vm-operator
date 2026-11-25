// Copyright (c) 2026 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package supervisor

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSupervisor(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Supervisor Suite")
}
