// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package spq_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	spqutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube/spq"
)

var _ = Describe("FromContext", func() {
	Specify("two calls should return the same object", func() {
		ctx := cource.NewContext()
		obj1 := spqutil.FromContext(ctx)
		obj2 := spqutil.FromContext(ctx)
		Expect(obj1).To(Equal(obj2))
	})
})
