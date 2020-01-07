// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmoperator

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("REST provider", func() {
	Context("During initialization", func() {
		It("can register only provider", func() {
			restProvider := RestProvider{}
			Expect(RegisterRestProvider(restProvider)).To(Succeed())
			Expect(*GetRestProvider()).To(Equal(restProvider))
			err := RegisterRestProvider(restProvider)
			Expect(err).Should(HaveOccurred())
			Expect(err).Should(MatchError("REST provider already registered"))
		})
	})
})
