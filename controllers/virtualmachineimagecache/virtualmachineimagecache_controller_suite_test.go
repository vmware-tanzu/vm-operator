// Copyright (c) 2024 Broadcom. All Rights Reserved.
// Broadcom Confidential. The term "Broadcom" refers to Broadcom Inc.
// and/or its subsidiaries.

package virtualmachineimagecache_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachineimagecache"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

func TestVirtualMachineImageCacheController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "VirtualMachineImageCache Controller Test Suite")
}

var _ = BeforeSuite(func() {
	virtualmachineimagecache.SkipNameValidation = ptr.To(true)
})
