// Copyright (c) 2026 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package redact_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestRedact(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Redact Suite")
}
