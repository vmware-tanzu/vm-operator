// Copyright (c) 2026 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package ssh_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSSH(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SSH Suite")
}
