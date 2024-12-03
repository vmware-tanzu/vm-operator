// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package ovfcache_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestOVFCache(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "OVF Cache Test Suite")
}
