// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package isohttpserver_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestISOHTTPServer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ISO HTTP Server Suite")
}
