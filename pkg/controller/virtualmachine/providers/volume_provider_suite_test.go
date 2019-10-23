// +build !integration

/* **********************************************************
 * Copyright 2019-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package providers

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestVolumeProvider(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Volume Provider Suite")
}
