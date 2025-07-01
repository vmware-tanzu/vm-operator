// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package errors_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
)

var _ = Describe("NoRequeueError", func() {

	DescribeTable("Error",
		func(e error, expErr string) {
			Expect(e).To(MatchError(expErr))
		},

		Entry(
			"no message",
			pkgerr.NoRequeueError{},
			"no requeue",
		),

		Entry(
			"with message",
			pkgerr.NoRequeueError{Message: "hi"},
			"hi",
		),
	)
})
