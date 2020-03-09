// +build !integration

// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package record_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestServices(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Event Suite")
}
