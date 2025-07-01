// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package concepts

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestConcepts(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Docs Concepts Test Suite")
}
