// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package zone_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator/controllers/infra/zone"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

func TestZoneController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Zone Controller Test Suite")
}

var _ = BeforeSuite(func() {
	zone.SkipNameValidation = ptr.To(true)
})
