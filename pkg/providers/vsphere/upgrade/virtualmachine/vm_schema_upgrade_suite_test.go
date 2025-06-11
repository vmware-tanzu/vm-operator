// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/klog/v2"
)

func init() {
	klog.InitFlags(nil)
	klog.SetOutput(GinkgoWriter)
}

func TestVMSchemaUpgrade(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "VM Schema Upgrade Suite")
}
