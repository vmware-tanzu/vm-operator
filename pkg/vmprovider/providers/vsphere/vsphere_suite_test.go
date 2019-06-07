/* **********************************************************
 * Copyright 2019-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/
package vsphere_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"testing"
)

func TestProviderCredentials(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "vSphere Provider Suite")
}
