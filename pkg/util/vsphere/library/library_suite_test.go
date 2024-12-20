// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package library_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestContentLibrary(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ContentLibrary Test Suite")
}
