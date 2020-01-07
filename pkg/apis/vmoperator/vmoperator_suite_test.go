/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

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
