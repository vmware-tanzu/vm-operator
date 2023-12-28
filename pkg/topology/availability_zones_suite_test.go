// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package topology_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestTopology(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Topology Suite")
}
