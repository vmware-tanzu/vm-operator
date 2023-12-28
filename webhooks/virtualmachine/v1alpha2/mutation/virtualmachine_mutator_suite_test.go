// Copyright (c) 2019-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package mutation_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"

	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/test/builder"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachine/v1alpha2/mutation"
)

// suite is used for unit and integration testing this webhook.
var suite = builder.NewTestSuiteForMutatingWebhookwithFSS(
	mutation.AddToManager,
	mutation.NewMutator,
	"default.mutating.virtualmachine.v1alpha2.vmoperator.vmware.com",
	map[string]bool{lib.VMServiceV1Alpha2FSS: true})

func TestWebhook(t *testing.T) {
	suite.Register(t, "Mutating webhook suite", intgTests, uniTests)
}

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)

func newInvalidNextRestartTimeTableEntries(description string) []TableEntry {
	return []TableEntry{
		Entry(description, "5m"),
		Entry(description, "1s"),
		Entry(description, "2h45m"),
		Entry(description, "1.5h"),
		Entry(description, "2023-06-01T13:00:00Z"),
		Entry(description, "2023-06-01T13:00:00-06:00"),
		Entry(description, "2023-06-01T13:00:00+05:30"),
		Entry(description, "hello"),
		Entry(description, "world"),
	}
}
