// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator/pkg/util"
)

var _ = Describe("IsNotFoundError", func() {
	Context("when a NotFound error is passed", func() {
		It("should return true", func() {
			err := fmt.Errorf("404 Not Found")
			Expect(util.IsNotFoundError(err)).To(BeTrue())
		})
	})

	Context("when an error other than NotFound is passed", func() {
		It("should return false", func() {
			err := fmt.Errorf("some-other-error")
			Expect(util.IsNotFoundError(err)).To(BeFalse())
		})
	})
})
