// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package utils_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator/controllers/contentlibrary/utils"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

func TestContentLibraryItemControllerUtils(t *testing.T) {
	utils.SkipNameValidation = ptr.To(true)
	RegisterFailHandler(Fail)
	RunSpecs(t, "ContentLibraryItem Controller Utils Test Suite")
}
