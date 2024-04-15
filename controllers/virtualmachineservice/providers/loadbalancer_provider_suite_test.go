// Copyright (c) 2019-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package providers_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestLoadBalancerProvider(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "LoadBalancer Provider Suite")
}
