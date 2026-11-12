// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmtags_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	_ "github.com/vmware-tanzu/vm-operator/test/builder/log"
)

func TestVMTags(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "VMTags Reconciler Test Suite")
}
