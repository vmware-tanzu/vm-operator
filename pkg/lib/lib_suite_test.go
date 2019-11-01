// *******************************************************************************
// Copyright 2018 - 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
// *******************************************************************************

package lib_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestLib(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Lib Suite")
}
