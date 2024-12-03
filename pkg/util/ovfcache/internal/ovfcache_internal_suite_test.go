// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package internal_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestOVFCacheInternal(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "OVF Cache Internal Test Suite")
}
