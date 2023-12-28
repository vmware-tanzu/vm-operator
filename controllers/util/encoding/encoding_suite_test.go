// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package encoding_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestUtilities(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Encoding Utils Suite")
}
