// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmoperator_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestVMOperator(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "VM Operator Suite")
}
