// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package errors_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
)

var _ = Describe("RequeueError", func() {

	DescribeTable("Error",
		func(e error, expErr string) {
			Expect(e).To(MatchError(expErr))
		},

		Entry(
			"after is 0",
			pkgerr.RequeueError{},
			"requeue immediately",
		),

		Entry(
			"after is 1s",
			pkgerr.RequeueError{After: time.Second * 1},
			"requeue after 1s",
		),
	)
})
